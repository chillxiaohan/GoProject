package excelddl

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// FieldTypes 建表 Excel「类型」列下拉（与 BuildMSSQLDDL 支持的类型对齐）。
var FieldTypes = []string{
	"INT",
	"BIGINT",
	"NVARCHAR",
	"VARCHAR",
	"TEXT",
	"BIT",
	"BOOLEAN",
	"TIMESTAMP",
	"DATETIME2",
	"DECIMAL",
	"FLOAT",
	"JSON",
}

// LengthHints 「长度」列常用值。
// 注意：内联下拉用逗号分隔项，DECIMAL 精度写成 18.2（建表时会转成 18,2）。
var LengthHints = []string{
	"32",
	"50",
	"64",
	"100",
	"128",
	"200",
	"255",
	"500",
	"MAX",
	"18.2",
}

// YesNoOptions 「非空 / 唯一 / 主键」列下拉。留空视为否。
var YesNoOptions = []string{"是", "否"}

const dropdownLastRow = 200

func isMetaSheet(name string) bool {
	n := strings.TrimSpace(name)
	return n == "" || strings.HasPrefix(n, "_")
}

func applyDropdowns(f *excelize.File, dataSheet string) error {
	// 使用单元格内联列表（不用隐藏表引用），Excel / WPS 均可点选。
	// 黄色浮层若出现，那是「输入提示」不是下拉；请点单元格右侧▼箭头。
	if err := addInlineList(f, dataSheet, fmt.Sprintf("C3:C%d", dropdownLastRow), FieldTypes); err != nil {
		return err
	}
	if err := addInlineList(f, dataSheet, fmt.Sprintf("D3:D%d", dropdownLastRow), LengthHints); err != nil {
		return err
	}
	for _, colRange := range []string{
		fmt.Sprintf("E3:E%d", dropdownLastRow),
		fmt.Sprintf("F3:F%d", dropdownLastRow),
		fmt.Sprintf("G3:G%d", dropdownLastRow),
	} {
		if err := addInlineList(f, dataSheet, colRange, YesNoOptions); err != nil {
			return err
		}
	}
	return nil
}

func addInlineList(f *excelize.File, dataSheet, sqref string, keys []string) error {
	if len(keys) == 0 {
		return nil
	}
	dv := excelize.NewDataValidation(true)
	dv.Sqref = sqref
	if err := dv.SetDropList(keys); err != nil {
		return fmt.Errorf("下拉选项过长 %s: %w", sqref, err)
	}
	// 不设 SetInput，避免黄色提示框被当成「下拉列表」
	if err := f.AddDataValidation(dataSheet, dv); err != nil {
		return fmt.Errorf("下拉 %s: %w", sqref, err)
	}
	return nil
}
