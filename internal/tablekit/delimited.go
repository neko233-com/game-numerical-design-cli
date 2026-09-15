package tablekit

import (
	"encoding/csv"
	"fmt"
	"os"
	"strings"
)

func loadDelimited(path string, comma rune, opt Options) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	r := csv.NewReader(strings.NewReader(string(data)))
	r.Comma = comma
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse delimited %s: %w", path, err)
	}
	return tableFromRecords(path, records)
}

func tableFromRecords(name string, records [][]string) (*Table, error) {
	// drop fully empty trailing lines
	for len(records) > 0 && isEmptyRecord(records[len(records)-1]) {
		records = records[:len(records)-1]
	}
	if len(records) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	headers := make([]string, len(records[0]))
	copy(headers, records[0])
	for i := range headers {
		headers[i] = strings.TrimSpace(headers[i])
	}
	rows := make([][]string, 0, len(records)-1)
	width := len(headers)
	for _, rec := range records[1:] {
		if isEmptyRecord(rec) {
			continue
		}
		row := make([]string, width)
		for i := 0; i < width && i < len(rec); i++ {
			row[i] = rec[i]
		}
		rows = append(rows, row)
	}
	return &Table{Name: name, Headers: headers, Rows: rows}, nil
}

func isEmptyRecord(rec []string) bool {
	for _, c := range rec {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func (t *Table) saveDelimited(path string, comma rune, _ Options) error {
	var b strings.Builder
	w := csv.NewWriter(&b)
	w.Comma = comma
	if err := w.Write(t.Headers); err != nil {
		return err
	}
	for _, row := range t.Rows {
		// pad / trim to header width
		rec := make([]string, len(t.Headers))
		for i := range rec {
			if i < len(row) {
				rec[i] = row[i]
			}
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
