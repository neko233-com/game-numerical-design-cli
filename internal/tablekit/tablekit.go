// Package tablekit provides multi-format game-config table IO:
// xlsx / csv / tsv / json / yaml — unified model, convert, and row edits.
package tablekit

import (
	"fmt"
	"path/filepath"
	"strings"
)

// Format enumerates supported table formats.
type Format string

const (
	FormatXLSX Format = "xlsx"
	FormatCSV  Format = "csv"
	FormatTSV  Format = "tsv"
	FormatJSON Format = "json"
	FormatYAML Format = "yaml"
	FormatYML  Format = "yml"
)

// DetectFormat from file extension.
func DetectFormat(path string) (Format, error) {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(path), "."))
	switch ext {
	case "xlsx", "xlsm":
		return FormatXLSX, nil
	case "csv":
		return FormatCSV, nil
	case "tsv":
		return FormatTSV, nil
	case "json":
		return FormatJSON, nil
	case "yaml", "yml":
		return FormatYAML, nil
	default:
		return "", fmt.Errorf("unsupported table format %q (want xlsx/csv/tsv/json/yaml)", ext)
	}
}

// Table is an ordered rectangular grid with a single header row.
// All cell values are kept as strings to preserve precision (large ints, IDs).
type Table struct {
	Name    string     `json:"name"`
	Headers []string   `json:"headers"`
	Rows    [][]string `json:"rows"`
}

// Options tune load/save.
type Options struct {
	// Sheet for xlsx; empty = first non-empty sheet (or only sheet).
	Sheet string
	// DataStartRow is 1-based physical row where headers live (xlsx). Default 1.
	// For the 5-row BusinessConfig convention, set DataStartRow=3 (client name row)
	// or 5 (server name row); other header rows are ignored.
	DataStartRow int
	// IDField used by keyed ops. Default: first header named id/Id/ID, else headers[0].
	IDField string
}

func (o Options) dataStart() int {
	if o.DataStartRow <= 0 {
		return 1
	}
	return o.DataStartRow
}

// Load reads a table file in any supported format.
func Load(path string, opt Options) (*Table, error) {
	format, err := DetectFormat(path)
	if err != nil {
		return nil, err
	}
	return LoadFormat(path, format, opt)
}

// LoadFormat reads with an explicit format.
func LoadFormat(path string, format Format, opt Options) (*Table, error) {
	switch format {
	case FormatXLSX:
		return loadXLSX(path, opt)
	case FormatCSV:
		return loadDelimited(path, ',', opt)
	case FormatTSV:
		return loadDelimited(path, '\t', opt)
	case FormatJSON:
		return loadJSON(path, opt)
	case FormatYAML, FormatYML:
		return loadYAML(path, opt)
	default:
		return nil, fmt.Errorf("unknown format %q", format)
	}
}

// Save writes the table to path using format from extension (or explicit).
func (t *Table) Save(path string, opt Options) error {
	format, err := DetectFormat(path)
	if err != nil {
		return err
	}
	return t.SaveFormat(path, format, opt)
}

// SaveFormat writes with an explicit format.
func (t *Table) SaveFormat(path string, format Format, opt Options) error {
	switch format {
	case FormatXLSX:
		return t.saveXLSX(path, opt)
	case FormatCSV:
		return t.saveDelimited(path, ',', opt)
	case FormatTSV:
		return t.saveDelimited(path, '\t', opt)
	case FormatJSON:
		return t.saveJSONPath(path, true)
	case FormatYAML, FormatYML:
		return t.saveYAMLPath(path)
	default:
		return fmt.Errorf("unknown format %q", format)
	}
}

// Convert loads src and writes to dst (formats from extensions).
func Convert(src, dst string, opt Options) (*Table, error) {
	t, err := Load(src, opt)
	if err != nil {
		return nil, err
	}
	if err := t.Save(dst, opt); err != nil {
		return nil, err
	}
	return t, nil
}

// ColIndex returns header index or -1.
func (t *Table) ColIndex(name string) int {
	for i, h := range t.Headers {
		if h == name {
			return i
		}
	}
	return -1
}

// ResolveIDField picks the id column name.
func (t *Table) ResolveIDField(explicit string) (string, int, error) {
	if explicit != "" {
		idx := t.ColIndex(explicit)
		if idx < 0 {
			return "", -1, fmt.Errorf("id field %q not in headers %v", explicit, t.Headers)
		}
		return explicit, idx, nil
	}
	for _, cand := range []string{"id", "Id", "ID", "ID."} {
		if idx := t.ColIndex(cand); idx >= 0 {
			return cand, idx, nil
		}
	}
	if len(t.Headers) == 0 {
		return "", -1, fmt.Errorf("empty table")
	}
	return t.Headers[0], 0, nil
}

// FindRow returns the first data-row index whose id field equals id.
func (t *Table) FindRow(id string, opt Options) (int, error) {
	_, idx, err := t.ResolveIDField(opt.IDField)
	if err != nil {
		return -1, err
	}
	for i, row := range t.Rows {
		if idx < len(row) && row[idx] == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("id %q not found", id)
}

// GetCell returns cell value by id and field.
func (t *Table) GetCell(id, field string, opt Options) (string, error) {
	ri, err := t.FindRow(id, opt)
	if err != nil {
		return "", err
	}
	ci := t.ColIndex(field)
	if ci < 0 {
		return "", fmt.Errorf("field %q not in headers", field)
	}
	if ci >= len(t.Rows[ri]) {
		return "", nil
	}
	return t.Rows[ri][ci], nil
}

// Change is one planned cell edit.
type Change struct {
	RowIndex int    `json:"rowIndex"` // 0-based data row
	Column   string `json:"column"`
	OldValue string `json:"oldValue"`
	NewValue string `json:"newValue"`
	ID       string `json:"id,omitempty"`
}

// SetCell sets field of row identified by id. Returns the change (even if dry).
func (t *Table) SetCell(id, field, newValue string, opt Options) (Change, error) {
	ri, err := t.FindRow(id, opt)
	if err != nil {
		return Change{}, err
	}
	ci := t.ColIndex(field)
	if ci < 0 {
		return Change{}, fmt.Errorf("field %q not in headers", field)
	}
	old := ""
	if ci < len(t.Rows[ri]) {
		old = t.Rows[ri][ci]
	}
	// pad row if needed
	for len(t.Rows[ri]) <= ci {
		t.Rows[ri] = append(t.Rows[ri], "")
	}
	t.Rows[ri][ci] = newValue
	return Change{
		RowIndex: ri, Column: field,
		OldValue: old, NewValue: newValue, ID: id,
	}, nil
}

// SetCellExpected is SetCell with optimistic concurrency on old value.
func (t *Table) SetCellExpected(id, field, newValue, expected string, opt Options) (Change, error) {
	cur, err := t.GetCell(id, field, opt)
	if err != nil {
		return Change{}, err
	}
	if cur != expected {
		return Change{}, fmt.Errorf("expected %q but current %q for id=%s field=%s", expected, cur, id, field)
	}
	return t.SetCell(id, field, newValue, opt)
}

// AddRow appends a sparse map of header→value; missing headers become "".
func (t *Table) AddRow(values map[string]string) int {
	row := make([]string, len(t.Headers))
	for i, h := range t.Headers {
		if v, ok := values[h]; ok {
			row[i] = v
		}
	}
	t.Rows = append(t.Rows, row)
	return len(t.Rows) - 1
}

// UpsertRow sets fields on existing id, or appends if missing.
// values must include the id field.
func (t *Table) UpsertRow(values map[string]string, opt Options) (idx int, created bool, changes []Change, err error) {
	idField, _, err := t.ResolveIDField(opt.IDField)
	if err != nil {
		return 0, false, nil, err
	}
	id, ok := values[idField]
	if !ok || id == "" {
		return 0, false, nil, fmt.Errorf("upsert requires id field %q", idField)
	}
	ri, findErr := t.FindRow(id, opt)
	if findErr != nil {
		idx = t.AddRow(values)
		return idx, true, nil, nil
	}
	for k, v := range values {
		ci := t.ColIndex(k)
		if ci < 0 {
			continue
		}
		old := ""
		if ci < len(t.Rows[ri]) {
			old = t.Rows[ri][ci]
		}
		if old == v {
			continue // no-op
		}
		for len(t.Rows[ri]) <= ci {
			t.Rows[ri] = append(t.Rows[ri], "")
		}
		t.Rows[ri][ci] = v
		changes = append(changes, Change{
			RowIndex: ri, Column: k, OldValue: old, NewValue: v, ID: id,
		})
	}
	return ri, false, changes, nil
}

// Clone deep-copies the table (for dry-run preview).
func (t *Table) Clone() *Table {
	c := &Table{Name: t.Name, Headers: append([]string(nil), t.Headers...)}
	c.Rows = make([][]string, len(t.Rows))
	for i, r := range t.Rows {
		c.Rows[i] = append([]string(nil), r...)
	}
	return c
}

// AsRecords converts to []map[string]string for validate integration.
func (t *Table) AsRecords() []map[string]string {
	out := make([]map[string]string, 0, len(t.Rows))
	for _, r := range t.Rows {
		m := make(map[string]string, len(t.Headers))
		for i, h := range t.Headers {
			if i < len(r) {
				m[h] = r[i]
			} else {
				m[h] = ""
			}
		}
		out = append(out, m)
	}
	return out
}
