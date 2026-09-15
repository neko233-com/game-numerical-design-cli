// Package validate performs pre-release numerical table safety checks.
package validate

import (
	"fmt"
	"math"
	"strings"
)

// Row is a generic numeric table row (level / item / stage).
type Row struct {
	ID     int64              `json:"id"`
	Name   string             `json:"name"`
	Fields map[string]float64 `json:"fields"`
}

// Issue is one validation finding.
type Issue struct {
	Severity string `json:"severity"` // error | warn | info
	RowID    int64  `json:"row_id"`
	Field    string `json:"field"`
	Message  string `json:"message"`
}

// Options tune thresholds.
type Options struct {
	// Growth cliff: relative step > this flags as warn.
	GrowthCliff float64
	// Negative fields allowed?
	AllowNegative bool
	// Allowed non-finite? always false; listed for clarity.
	// Required fields (empty fields on a row = error).
	Required []string
	// Monotonic fields that must be non-decreasing by ID.
	Monotonic []string
	// MaxInt: if >0, fields exceeding this as integer overflow risk for int32 configs.
	MaxSafe float64
}

// DefaultOptions returns conservative production checks.
func DefaultOptions() Options {
	return Options{
		GrowthCliff:   0.5,
		AllowNegative: false,
		MaxSafe:       2_147_483_647,
	}
}

// Table runs all checks.
func Table(rows []Row, opt Options) []Issue {
	var issues []Issue
	if len(rows) == 0 {
		return []Issue{{Severity: "error", Message: "表为空"}}
	}
	// sort by ID copy to check monotonicity by design order
	ordered := make([]Row, len(rows))
	copy(ordered, rows)
	for i := 1; i < len(ordered); i++ {
		for j := i; j > 0 && ordered[j].ID < ordered[j-1].ID; j-- {
			ordered[j], ordered[j-1] = ordered[j-1], ordered[j]
		}
	}

	for _, r := range ordered {
		for _, req := range opt.Required {
			if _, ok := r.Fields[req]; !ok {
				issues = append(issues, Issue{
					Severity: "error", RowID: r.ID, Field: req,
					Message: fmt.Sprintf("缺少必填字段 %s", req),
				})
			}
		}
		for k, v := range r.Fields {
			if math.IsNaN(v) || math.IsInf(v, 0) {
				issues = append(issues, Issue{
					Severity: "error", RowID: r.ID, Field: k,
					Message: "非法数值 NaN/Inf",
				})
				continue
			}
			if v < 0 && !opt.AllowNegative {
				issues = append(issues, Issue{
					Severity: "warn", RowID: r.ID, Field: k,
					Message: fmt.Sprintf("出现负值 %v（若业务允许请开启 AllowNegative）", v),
				})
			}
			if opt.MaxSafe > 0 && v > opt.MaxSafe {
				issues = append(issues, Issue{
					Severity: "error", RowID: r.ID, Field: k,
					Message: fmt.Sprintf("数值 %v 超过 int32 安全上限 %v，配置导出可能溢出", v, opt.MaxSafe),
				})
			}
		}
	}

	// cliff + monotonic on shared numeric fields
	fieldSet := map[string]bool{}
	for _, r := range ordered {
		for k := range r.Fields {
			fieldSet[k] = true
		}
	}
	fields := make([]string, 0, len(fieldSet))
	for k := range fieldSet {
		fields = append(fields, k)
	}
	// stable-ish order
	for i := 0; i < len(fields); i++ {
		for j := i + 1; j < len(fields); j++ {
			if fields[j] < fields[i] {
				fields[i], fields[j] = fields[j], fields[i]
			}
		}
	}

	mono := map[string]bool{}
	for _, m := range opt.Monotonic {
		mono[m] = true
	}

	for _, f := range fields {
		var prev *Row
		for _, r := range ordered {
			v, ok := r.Fields[f]
			if !ok {
				prev = nil
				continue
			}
			if prev != nil {
				pv := prev.Fields[f]
				if opt.GrowthCliff > 0 && pv != 0 {
					ratio := math.Abs(v-pv) / math.Abs(pv)
					if ratio > opt.GrowthCliff && math.Abs(v-pv) > 1e-9 {
						issues = append(issues, Issue{
							Severity: "warn", RowID: r.ID, Field: f,
							Message: fmt.Sprintf("相邻 ID %d→%d 相对跳变 %.1f%%（阈值 %.0f%%）— 疑似断崖",
								prev.ID, r.ID, ratio*100, opt.GrowthCliff*100),
						})
					}
				}
				if mono[f] && v < pv-1e-9 {
					issues = append(issues, Issue{
						Severity: "error", RowID: r.ID, Field: f,
						Message: fmt.Sprintf("要求单调不减，但 %v < 上一行 %v", v, pv),
					})
				}
			}
			// duplicate ID check handled below
			pr := r
			prev = &pr
		}
	}

	// duplicate IDs
	seen := map[int64]int{}
	for _, r := range ordered {
		seen[r.ID]++
		if seen[r.ID] == 2 {
			issues = append(issues, Issue{
				Severity: "error", RowID: r.ID,
				Message: "重复 ID",
			})
		}
	}
	return issues
}

// Summarize counts issues by severity.
func Summarize(issues []Issue) string {
	var e, w, i int
	for _, is := range issues {
		switch strings.ToLower(is.Severity) {
		case "error":
			e++
		case "warn":
			w++
		default:
			i++
		}
	}
	return fmt.Sprintf("error=%d warn=%d info=%d", e, w, i)
}

// ParseCSVTable parses a simple CSV: first column id, second name, rest numeric fields.
// Header row required.
func ParseCSVTable(data string) ([]Row, error) {
	lines := strings.Split(strings.ReplaceAll(data, "\r\n", "\n"), "\n")
	var header []string
	var rows []Row
	for li, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		parts := strings.Split(line, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		if header == nil {
			if len(parts) < 3 {
				return nil, fmt.Errorf("header needs >=3 columns: id,name,fields...")
			}
			header = parts
			continue
		}
		if len(parts) < len(header) {
			return nil, fmt.Errorf("line %d: got %d cols, want %d", li+1, len(parts), len(header))
		}
		var id int64
		if _, err := fmt.Sscanf(parts[0], "%d", &id); err != nil {
			return nil, fmt.Errorf("line %d: bad id %q", li+1, parts[0])
		}
		r := Row{ID: id, Name: parts[1], Fields: map[string]float64{}}
		for i := 2; i < len(header); i++ {
			var v float64
			if _, err := fmt.Sscanf(parts[i], "%g", &v); err != nil {
				return nil, fmt.Errorf("line %d: field %s bad number %q", li+1, header[i], parts[i])
			}
			r.Fields[header[i]] = v
		}
		rows = append(rows, r)
	}
	if header == nil {
		return nil, fmt.Errorf("empty csv")
	}
	return rows, nil
}
