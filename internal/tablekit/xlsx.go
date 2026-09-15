package tablekit

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

func loadXLSX(path string, opt Options) (*Table, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	sheet := strings.TrimSpace(opt.Sheet)
	if sheet == "" {
		sheet, err = pickSheet(f)
		if err != nil {
			return nil, err
		}
	} else {
		if !containsSheet(f, sheet) {
			return nil, fmt.Errorf("sheet %q not found in %s (have %v)", sheet, path, f.GetSheetList())
		}
	}

	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, err
	}
	start := opt.dataStart() // 1-based
	if start > len(rows) {
		return nil, fmt.Errorf("data start row %d beyond sheet rows %d", start, len(rows))
	}
	records := rows[start-1:]
	return tableFromRecords(path+"#"+sheet, records)
}

func pickSheet(f *excelize.File) (string, error) {
	names := f.GetSheetList()
	var nonEmpty []string
	for _, n := range names {
		rows, err := f.GetRows(n)
		if err != nil {
			continue
		}
		if !allEmpty(rows) {
			nonEmpty = append(nonEmpty, n)
		}
	}
	if len(nonEmpty) == 1 {
		return nonEmpty[0], nil
	}
	if len(nonEmpty) == 0 {
		if len(names) == 1 {
			return names[0], nil
		}
		return "", fmt.Errorf("no non-empty sheet; specify --sheet (have %v)", names)
	}
	return "", fmt.Errorf("multiple non-empty sheets %v; specify --sheet", nonEmpty)
}

func containsSheet(f *excelize.File, name string) bool {
	for _, n := range f.GetSheetList() {
		if n == name {
			return true
		}
	}
	return false
}

func allEmpty(rows [][]string) bool {
	for _, r := range rows {
		for _, c := range r {
			if strings.TrimSpace(c) != "" {
				return false
			}
		}
	}
	return true
}

func (t *Table) saveXLSX(path string, opt Options) error {
	f := excelize.NewFile()
	sheet := strings.TrimSpace(opt.Sheet)
	if sheet == "" {
		sheet = "Sheet1"
	}
	defaultSheet := f.GetSheetName(0)
	if defaultSheet != sheet {
		if err := f.SetSheetName(defaultSheet, sheet); err != nil {
			return err
		}
	}
	// header
	for i, h := range t.Headers {
		cell, err := excelize.CoordinatesToCellName(i+1, 1)
		if err != nil {
			return err
		}
		if err := f.SetCellValue(sheet, cell, h); err != nil {
			return err
		}
	}
	// rows
	for r, row := range t.Rows {
		for c := 0; c < len(t.Headers); c++ {
			cell, err := excelize.CoordinatesToCellName(c+1, r+2)
			if err != nil {
				return err
			}
			val := ""
			if c < len(row) {
				val = row[c]
			}
			if val == "" {
				continue
			}
			if err := f.SetCellValue(sheet, cell, val); err != nil {
				return err
			}
		}
	}
	if err := f.SaveAs(path); err != nil {
		return err
	}
	return f.Close()
}

// SheetNames lists non-empty sheets.
func SheetNames(path string) ([]string, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var out []string
	for _, n := range f.GetSheetList() {
		rows, err := f.GetRows(n)
		if err != nil {
			continue
		}
		if !allEmpty(rows) {
			out = append(out, n)
		}
	}
	return out, nil
}
