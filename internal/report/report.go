// Package report formats CLI output as text tables or JSON.
package report

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// JSON writes v as pretty JSON.
func JSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// Table writes a simple aligned text table.
func Table(w io.Writer, headers []string, rows [][]string) error {
	if len(headers) == 0 {
		return fmt.Errorf("no headers")
	}
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len(h)
	}
	for _, r := range rows {
		for i := 0; i < len(headers) && i < len(r); i++ {
			if n := len(r[i]); n > widths[i] {
				widths[i] = n
			}
		}
	}
	var b strings.Builder
	writeRow := func(cols []string) {
		for i := 0; i < len(headers); i++ {
			cell := ""
			if i < len(cols) {
				cell = cols[i]
			}
			b.WriteString(cell)
			if i < len(headers)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-len(cell)+2))
			}
		}
		b.WriteString("\n")
	}
	writeRow(headers)
	seps := make([]string, len(headers))
	for i, w := range widths {
		seps[i] = strings.Repeat("-", w)
	}
	writeRow(seps)
	for _, r := range rows {
		writeRow(r)
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// KeyValue writes aligned key/value pairs.
func KeyValue(w io.Writer, kv [][2]string) error {
	width := 0
	for _, p := range kv {
		if len(p[0]) > width {
			width = len(p[0])
		}
	}
	var b strings.Builder
	for _, p := range kv {
		b.WriteString(fmt.Sprintf("%-*s  %s\n", width, p[0], p[1]))
	}
	_, err := io.WriteString(w, b.String())
	return err
}

// CSV writes sample points as x,y CSV.
func CSV(w io.Writer, headers []string, rows [][]string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(headers); err != nil {
		return err
	}
	if err := cw.WriteAll(rows); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

// Ftoa formats a float compactly for tables.
func Ftoa(v float64) string {
	return strconv.FormatFloat(v, 'g', 6, 64)
}

// MiniChart renders a unicode sparkline/bar for a series (scale to width).
func MiniChart(values []float64, width int) string {
	if len(values) == 0 || width <= 0 {
		return ""
	}
	// downsample
	n := len(values)
	if n > width {
		// pick width buckets
		out := make([]float64, width)
		for i := 0; i < width; i++ {
			lo := i * n / width
			hi := (i + 1) * n / width
			if hi <= lo {
				hi = lo + 1
			}
			var sum float64
			for j := lo; j < hi && j < n; j++ {
				sum += values[j]
			}
			out[i] = sum / float64(hi-lo)
		}
		values = out
	}
	minV, maxV := values[0], values[0]
	for _, v := range values {
		if v < minV {
			minV = v
		}
		if v > maxV {
			maxV = v
		}
	}
	ramp := []rune("▁▂▃▄▅▆▇█")
	var b strings.Builder
	for _, v := range values {
		var t float64
		if maxV > minV {
			t = (v - minV) / (maxV - minV)
		}
		idx := int(t * float64(len(ramp)-1))
		if idx < 0 {
			idx = 0
		}
		if idx >= len(ramp) {
			idx = len(ramp) - 1
		}
		b.WriteRune(ramp[idx])
	}
	return b.String()
}
