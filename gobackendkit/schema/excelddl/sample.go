package excelddl

import (
	"fmt"

	"github.com/xuri/excelize/v2"
)

// WriteSample 生成教程用的示例 Excel（一张 demo_items 表）。
func WriteSample(path string) error {
	f := excelize.NewFile()
	defer func() { _ = f.Close() }()

	const sheet = "demo_items"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return fmt.Errorf("重命名工作表失败: %w", err)
	}

	headers1 := []string{"说明", "字段英文名", "类型", "长度", "非空", "唯一", "主键", "默认值"}
	headers2 := []string{"comment", "name", "type", "length", "not_null", "unique", "primary", "default"}
	for i, h := range headers1 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		_ = f.SetCellValue(sheet, cell, h)
	}
	for i, h := range headers2 {
		cell, _ := excelize.CoordinatesToCellName(i+1, 2)
		_ = f.SetCellValue(sheet, cell, h)
	}

	data := [][]any{
		{"标题", "title", "NVARCHAR", "200", "是", "", "", ""},
		{"备注", "remark", "NVARCHAR", "MAX", "", "", "", ""},
		{"数量", "qty", "INT", "", "是", "", "", "0"},
	}
	for r, row := range data {
		for c, v := range row {
			cell, _ := excelize.CoordinatesToCellName(c+1, r+3)
			_ = f.SetCellValue(sheet, cell, v)
		}
	}

	if err := applyDropdowns(f, sheet); err != nil {
		return err
	}

	if err := f.SaveAs(path); err != nil {
		return fmt.Errorf("保存示例 Excel 失败: %w", err)
	}
	return nil
}
