package excelddl

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Column Excel 列定义（与教程约定一致）。
type Column struct {
	Comment string
	Name    string
	Type    string // VARCHAR / INT / TEXT / BOOLEAN / TIMESTAMP / DECIMAL / JSON …
	Length  string
	NotNull bool
	Unique  bool
	Primary bool
	Default string
}

// Table 一张表 = Excel 一个工作表。
type Table struct {
	Name    string
	Columns []Column
}

// ReadFile 读取 Excel：工作表名=表名；从第 3 行起读字段。
// 列顺序：注释 | 字段名 | 类型 | 长度 | 非空 | 唯一 | 主键 | 默认值(可选)
func ReadFile(path string) ([]Table, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var tables []Table
	for _, sheet := range f.GetSheetList() {
		if isMetaSheet(sheet) {
			continue
		}
		rows, err := f.GetRows(sheet)
		if err != nil {
			return nil, fmt.Errorf("工作表 %s: %w", sheet, err)
		}
		if len(rows) < 3 {
			continue
		}
		t := Table{Name: strings.TrimSpace(sheet)}
		t.Columns = append(t.Columns, defaultAuditColumns()...)
		for i, row := range rows[2:] {
			// 至少要有：注释、字段名、类型（空单元格在 Excel 末尾可能被裁掉）
			if len(row) < 3 {
				continue
			}
			name := strings.TrimSpace(cell(row, 1))
			if name == "" {
				continue
			}
			if isReservedAuditName(name) {
				continue
			}
			col := Column{
				Comment: strings.TrimSpace(cell(row, 0)),
				Name:    name,
				Type:    strings.ToUpper(strings.TrimSpace(cell(row, 2))),
				Length:  strings.TrimSpace(cell(row, 3)),
				NotNull: parseBool(cell(row, 4)),
				Unique:  parseBool(cell(row, 5)),
				Primary: parseBool(cell(row, 6)),
				Default: strings.TrimSpace(cell(row, 7)),
			}
			if col.Type == "" {
				return nil, fmt.Errorf("表 %s 第 %d 行缺少类型", t.Name, i+3)
			}
			t.Columns = append(t.Columns, col)
		}
		if t.Name != "" {
			tables = append(tables, t)
		}
	}
	return tables, nil
}

func defaultAuditColumns() []Column {
	return []Column{
		{Name: "id", Type: "INT", Primary: true, Comment: "主键"},
		{Name: "create_time", Type: "TIMESTAMP", NotNull: true, Comment: "创建时间"},
		{Name: "create_user", Type: "NVARCHAR", Length: "64", NotNull: true, Default: "''", Comment: "创建人"},
		{Name: "update_time", Type: "TIMESTAMP", NotNull: true, Comment: "更新时间"},
		{Name: "update_user", Type: "NVARCHAR", Length: "64", NotNull: true, Default: "''", Comment: "更新人"},
		{Name: "is_delete", Type: "BIT", NotNull: true, Default: "0", Comment: "软删"},
	}
}

func isReservedAuditName(name string) bool {
	switch strings.ToLower(name) {
	case "id", "create_time", "create_user", "update_time", "update_user", "is_delete":
		return true
	default:
		return false
	}
}

func cell(row []string, i int) string {
	if i >= len(row) {
		return ""
	}
	return row[i]
}

func parseBool(s string) bool {
	s = strings.TrimSpace(strings.ToLower(s))
	return s == "1" || s == "true" || s == "yes" || s == "y" || s == "是"
}

// BuildMSSQLDDL 生成 T-SQL CREATE TABLE（再交给 dialect.ConvertDDL）。
func BuildMSSQLDDL(t Table) (string, error) {
	if strings.TrimSpace(t.Name) == "" {
		return "", fmt.Errorf("表名为空")
	}
	var b strings.Builder
	b.WriteString("CREATE TABLE ")
	b.WriteString(t.Name)
	b.WriteString(" (\n")
	for i, c := range t.Columns {
		line, err := columnDDL(c)
		if err != nil {
			return "", err
		}
		b.WriteString("  ")
		b.WriteString(line)
		if i < len(t.Columns)-1 {
			b.WriteString(",")
		}
		b.WriteString("\n")
	}
	b.WriteString(")")
	return b.String(), nil
}

func columnDDL(c Column) (string, error) {
	name := strings.TrimSpace(c.Name)
	if name == "" {
		return "", fmt.Errorf("字段名为空")
	}
	typ := strings.ToUpper(strings.TrimSpace(c.Type))
	var b strings.Builder
	b.WriteString(name)
	b.WriteString(" ")

	switch typ {
	case "INT", "INTEGER":
		if c.Primary {
			b.WriteString("INT IDENTITY(1,1) PRIMARY KEY")
			return b.String(), nil
		}
		b.WriteString("INT")
	case "BIGINT":
		b.WriteString("BIGINT")
	case "SERIAL": // 兼容旧 Excel：按自增主键处理
		b.WriteString("INT IDENTITY(1,1) PRIMARY KEY")
		return b.String(), nil
	case "VARCHAR", "NVARCHAR":
		lenStr := c.Length
		if lenStr == "" {
			lenStr = "255"
		}
		if strings.EqualFold(lenStr, "MAX") {
			b.WriteString("NVARCHAR(MAX)")
		} else {
			b.WriteString("NVARCHAR(")
			b.WriteString(lenStr)
			b.WriteString(")")
		}
	case "TEXT":
		b.WriteString("NVARCHAR(MAX)")
	case "BOOLEAN", "BOOL", "BIT":
		b.WriteString("BIT")
	case "TIMESTAMP", "DATETIME", "DATETIME2":
		b.WriteString("DATETIME2")
	case "DECIMAL", "NUMERIC":
		lenStr := strings.ReplaceAll(c.Length, ".", ",")
		if lenStr != "" {
			b.WriteString("DECIMAL(")
			b.WriteString(lenStr)
			b.WriteString(")")
		} else {
			b.WriteString("DECIMAL(18,2)")
		}
	case "FLOAT":
		b.WriteString("FLOAT")
	case "JSON", "JSONB":
		b.WriteString("NVARCHAR(MAX)")
	default:
		return "", fmt.Errorf("不支持的类型 %s（字段 %s）", typ, name)
	}

	if c.Primary && typ != "INT" && typ != "INTEGER" && typ != "SERIAL" {
		b.WriteString(" PRIMARY KEY")
	}
	if c.NotNull {
		b.WriteString(" NOT NULL")
	}
	if c.Unique {
		b.WriteString(" UNIQUE")
	}
	if c.Default != "" {
		b.WriteString(" DEFAULT ")
		b.WriteString(normalizeDefault(typ, c.Default))
	}
	return b.String(), nil
}

func normalizeDefault(typ, def string) string {
	def = strings.TrimSpace(def)
	u := strings.ToUpper(typ)
	if u == "BIT" || u == "BOOLEAN" || u == "BOOL" {
		if def == "true" || def == "TRUE" || def == "1" {
			return "1"
		}
		return "0"
	}
	if (u == "NVARCHAR" || u == "VARCHAR" || u == "TEXT") && !strings.HasPrefix(def, "N'") && !strings.HasPrefix(def, "'") {
		if _, err := strconv.ParseFloat(def, 64); err != nil {
			return "N'" + strings.ReplaceAll(def, "'", "''") + "'"
		}
	}
	return def
}
