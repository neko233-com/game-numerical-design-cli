package tablekit

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"

	"gopkg.in/yaml.v3"
)

// loadJSON accepts:
//  1. array of objects: [{"id":"1","hp":"100"}, ...]
//  2. object with "headers" + "rows"
//  3. object with "columns" (header list) + "data" (array of objects)
func loadJSON(path string, opt Options) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseJSONTable(path, data)
}

func parseJSONTable(name string, data []byte) (*Table, error) {
	trimmed := trimBOM(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%s is empty", name)
	}
	switch trimmed[0] {
	case '[':
		var arr []map[string]any
		if err := json.Unmarshal(trimmed, &arr); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		return objectsToTable(name, arr)
	case '{':
		var obj map[string]any
		if err := json.Unmarshal(trimmed, &obj); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		// headers + rows
		if hs, ok := obj["headers"].([]any); ok {
			headers := make([]string, len(hs))
			for i, h := range hs {
				headers[i] = fmt.Sprint(h)
			}
			var rows [][]string
			if rs, ok := obj["rows"].([]any); ok {
				for _, raw := range rs {
					switch v := raw.(type) {
					case []any:
						row := make([]string, len(headers))
						for i := 0; i < len(headers) && i < len(v); i++ {
							row[i] = anyToString(v[i])
						}
						rows = append(rows, row)
					case map[string]any:
						row := make([]string, len(headers))
						for i, h := range headers {
							if val, ok := v[h]; ok {
								row[i] = anyToString(val)
							}
						}
						rows = append(rows, row)
					}
				}
			}
			return &Table{Name: name, Headers: headers, Rows: rows}, nil
		}
		// columns + data
		if cs, ok := obj["columns"].([]any); ok {
			headers := make([]string, len(cs))
			for i, h := range cs {
				headers[i] = fmt.Sprint(h)
			}
			var rows [][]string
			if ds, ok := obj["data"].([]any); ok {
				for _, raw := range ds {
					m, ok := raw.(map[string]any)
					if !ok {
						continue
					}
					row := make([]string, len(headers))
					for i, h := range headers {
						if val, ok := m[h]; ok {
							row[i] = anyToString(val)
						}
					}
					rows = append(rows, row)
				}
			}
			return &Table{Name: name, Headers: headers, Rows: rows}, nil
		}
		// single object → one-row table
		return objectsToTable(name, []map[string]any{obj})
	default:
		return nil, fmt.Errorf("%s: JSON must start with [ or {", name)
	}
}

func (t *Table) saveJSONPath(path string, pretty bool) error {
	// headers+rows preserves column order (object arrays cannot).
	doc := map[string]any{
		"headers": t.Headers,
		"rows":    t.Rows,
	}
	var data []byte
	var err error
	if pretty {
		data, err = json.MarshalIndent(doc, "", "  ")
	} else {
		data, err = json.Marshal(doc)
	}
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}

func loadYAML(path string, opt Options) (*Table, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseYAMLTable(path, data)
}

func parseYAMLTable(name string, data []byte) (*Table, error) {
	var arr []map[string]any
	if err := yaml.Unmarshal(data, &arr); err != nil {
		// try document with headers/rows
		var obj map[string]any
		if err2 := yaml.Unmarshal(data, &obj); err2 != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		if hs, ok := obj["headers"].([]any); ok {
			headers := make([]string, len(hs))
			for i, h := range hs {
				headers[i] = fmt.Sprint(h)
			}
			var rows [][]string
			if rs, ok := obj["rows"].([]any); ok {
				for _, raw := range rs {
					if v, ok := raw.([]any); ok {
						row := make([]string, len(headers))
						for i := 0; i < len(headers) && i < len(v); i++ {
							row[i] = anyToString(v[i])
						}
						rows = append(rows, row)
					}
				}
			}
			return &Table{Name: name, Headers: headers, Rows: rows}, nil
		}
		return nil, fmt.Errorf("%s: YAML must be a list of maps or headers/rows doc", name)
	}
	return objectsToTable(name, arr)
}

func (t *Table) saveYAMLPath(path string) error {
	// headers+rows preserves column order
	doc := map[string]any{
		"headers": t.Headers,
		"rows":    t.Rows,
	}
	data, err := yaml.Marshal(doc)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// objectsToTable builds header order: sorted keys (stable across runs).
// JSON object key order is not preserved by encoding/json, so we sort.
func objectsToTable(name string, arr []map[string]any) (*Table, error) {
	if len(arr) == 0 {
		return &Table{Name: name, Headers: nil, Rows: nil}, nil
	}
	seen := map[string]bool{}
	var headers []string
	for _, obj := range arr {
		for k := range obj {
			if !seen[k] {
				seen[k] = true
				headers = append(headers, k)
			}
		}
	}
	sort.Strings(headers)

	rows := make([][]string, len(arr))
	for i, obj := range arr {
		row := make([]string, len(headers))
		for j, h := range headers {
			if v, ok := obj[h]; ok {
				row[j] = anyToString(v)
			}
		}
		rows[i] = row
	}
	return &Table{Name: name, Headers: headers, Rows: rows}, nil
}

func anyToString(v any) string {
	if v == nil {
		return ""
	}
	switch x := v.(type) {
	case string:
		return x
	case float64:
		// JSON numbers: avoid 1.000000 for integers
		if x == float64(int64(x)) {
			return fmt.Sprintf("%d", int64(x))
		}
		return fmt.Sprintf("%g", x)
	case bool:
		if x {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return fmt.Sprint(x)
		}
		return string(b)
	}
}

func trimBOM(data []byte) []byte {
	if len(data) >= 3 && data[0] == 0xEF && data[1] == 0xBB && data[2] == 0xBF {
		return data[3:]
	}
	return data
}
