package excelddl

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestWriteAndReadSample(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.xlsx")
	if err := WriteSample(path); err != nil {
		t.Fatal(err)
	}
	tables, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(tables) != 1 || tables[0].Name != "demo_items" {
		t.Fatalf("tables=%+v", tables)
	}
	// 6 audit + 3 business
	if len(tables[0].Columns) < 9 {
		t.Fatalf("cols=%d %+v", len(tables[0].Columns), tables[0].Columns)
	}
	ddl, err := BuildMSSQLDDL(tables[0])
	if err != nil {
		t.Fatal(err)
	}
	if ddl == "" {
		t.Fatal("empty ddl")
	}

	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dvs, err := f.GetDataValidations("demo_items")
	if err != nil {
		t.Fatal(err)
	}
	if len(dvs) < 4 {
		t.Fatalf("want type/length/yes-no dropdowns, got %d", len(dvs))
	}
	_ = os.Remove(path)
}
