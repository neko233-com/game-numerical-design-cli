// Package htmlreport renders self-contained HTML numerical-design reports
// with inline SVG charts (no CDN, no network).
package htmlreport

import (
	"fmt"
	"html"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Palette — dark-friendly design tokens for readable charts.
var (
	colorBG      = "#0f1419"
	colorPanel   = "#1a2332"
	colorInk     = "#e7ecf3"
	colorMuted   = "#8b9bb4"
	colorAccent  = "#5b9fd4"
	colorOK      = "#3ecf8e"
	colorWarn    = "#f0b429"
	colorBad     = "#f07178"
	colorGrid    = "#2a3548"
	seriesColors = []string{
		"#5b9fd4", "#3ecf8e", "#f0b429", "#f07178",
		"#c792ea", "#89ddff", "#ffcb6b", "#82aaff",
	}
)

// Report is a multi-section HTML document builder.
type Report struct {
	Title    string
	Subtitle string
	sections []section
	meta     [][2]string
}

type section struct {
	Title string
	HTML  string
}

// New creates a report.
func New(title, subtitle string) *Report {
	return &Report{Title: title, Subtitle: subtitle}
}

// Meta adds a key/value shown in the header.
func (r *Report) Meta(k, v string) *Report {
	r.meta = append(r.meta, [2]string{k, v})
	return r
}

// SectionRaw appends raw HTML.
func (r *Report) SectionRaw(title, rawHTML string) *Report {
	r.sections = append(r.sections, section{Title: title, HTML: rawHTML})
	return r
}

// SectionText appends a prose block.
func (r *Report) SectionText(title, body string) *Report {
	parts := strings.Split(strings.TrimSpace(body), "\n")
	var b strings.Builder
	for _, p := range parts {
		if strings.TrimSpace(p) == "" {
			continue
		}
		if strings.HasPrefix(p, "- ") {
			b.WriteString("<li>" + html.EscapeString(p[2:]) + "</li>")
			continue
		}
		b.WriteString("<p>" + html.EscapeString(p) + "</p>")
	}
	return r.SectionRaw(title, b.String())
}

// SectionKV appends a definition table.
func (r *Report) SectionKV(title string, kv [][2]string) *Report {
	var b strings.Builder
	b.WriteString(`<table class="kv"><tbody>`)
	for _, p := range kv {
		b.WriteString("<tr><th>" + html.EscapeString(p[0]) + "</th><td>" +
			html.EscapeString(p[1]) + "</td></tr>")
	}
	b.WriteString("</tbody></table>")
	return r.SectionRaw(title, b.String())
}

// SectionTable appends a data table.
func (r *Report) SectionTable(title string, headers []string, rows [][]string) *Report {
	var b strings.Builder
	b.WriteString(`<div class="table-wrap"><table class="data"><thead><tr>`)
	for _, h := range headers {
		b.WriteString("<th>" + html.EscapeString(h) + "</th>")
	}
	b.WriteString("</tr></thead><tbody>")
	for _, row := range rows {
		b.WriteString("<tr>")
		for i := 0; i < len(headers); i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			b.WriteString("<td>" + html.EscapeString(cell) + "</td>")
		}
		b.WriteString("</tr>")
	}
	b.WriteString("</tbody></table></div>")
	return r.SectionRaw(title, b.String())
}

// SectionWarnings appends warning/callout list with severity colors.
func (r *Report) SectionWarnings(title string, warnings []string) *Report {
	if len(warnings) == 0 {
		return r.SectionRaw(title, `<p class="ok">未发现告警</p>`)
	}
	var b strings.Builder
	b.WriteString(`<ul class="warn-list">`)
	for _, w := range warnings {
		b.WriteString(`<li class="warn">` + html.EscapeString(w) + `</li>`)
	}
	b.WriteString("</ul>")
	return r.SectionRaw(title, b.String())
}

// WriteFile renders and saves HTML.
func (r *Report) WriteFile(path string) error {
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, []byte(r.Render()), 0o644)
}

// Render returns the full HTML document.
func (r *Report) Render() string {
	var b strings.Builder
	b.WriteString(`<!DOCTYPE html>
<html lang="zh-CN">
<head>
<meta charset="utf-8"/>
<meta name="viewport" content="width=device-width, initial-scale=1"/>
<title>`)
	b.WriteString(html.EscapeString(r.Title))
	b.WriteString(`</title>
<style>
:root{--bg:#0f1419;--panel:#1a2332;--ink:#e7ecf3;--muted:#8b9bb4;--accent:#5b9fd4;
--ok:#3ecf8e;--warn:#f0b429;--bad:#f07178;--grid:#2a3548;--border:#2a3548}
*{box-sizing:border-box}
body{margin:0;font:14px/1.55 -apple-system,"Segoe UI","PingFang SC","Microsoft YaHei",sans-serif;
background:var(--bg);color:var(--ink)}
header{padding:28px 32px 18px;border-bottom:1px solid var(--border);
background:linear-gradient(180deg,#152033 0%,var(--bg) 100%)}
h1{margin:0 0 6px;font-size:22px;font-weight:600;letter-spacing:.02em}
.sub{color:var(--muted);font-size:13px}
.meta{display:flex;flex-wrap:wrap;gap:10px 20px;margin-top:14px;font-size:12px;color:var(--muted)}
.meta b{color:var(--ink);font-weight:500}
main{padding:20px 32px 48px;max-width:1100px;margin:0 auto}
section{background:var(--panel);border:1px solid var(--border);border-radius:10px;
padding:18px 20px;margin-bottom:16px}
section h2{margin:0 0 12px;font-size:15px;font-weight:600;color:var(--accent);
text-transform:none;letter-spacing:.03em}
p{margin:0 0 10px}
li{margin:0 0 6px}
.kv{width:100%;border-collapse:collapse}
.kv th{text-align:left;color:var(--muted);font-weight:500;padding:6px 12px 6px 0;
white-space:nowrap;vertical-align:top;width:140px}
.kv td{padding:6px 0}
.table-wrap{overflow-x:auto}
table.data{width:100%;border-collapse:collapse;font-size:13px}
table.data th{background:#243044;color:var(--muted);text-align:left;padding:8px 10px;
border-bottom:1px solid var(--border);font-weight:500}
table.data td{padding:7px 10px;border-bottom:1px solid #243044}
table.data tr:hover td{background:#1e2a3d}
.warn-list{list-style:none;padding:0;margin:0}
.warn-list li{padding:8px 12px;border-radius:6px;margin-bottom:6px;
border-left:3px solid var(--warn);background:#2a2418}
.warn-list li.warn{border-color:var(--warn);background:#2a2418}
.ok{color:var(--ok)}
.chart-grid{display:grid;grid-template-columns:1fr;gap:14px}
@media(min-width:800px){.chart-grid.two{grid-template-columns:1fr 1fr}}
.chart-card{background:#141c28;border:1px solid var(--border);border-radius:8px;padding:12px}
.chart-card h3{margin:0 0 8px;font-size:13px;font-weight:500;color:var(--muted)}
.chart-card svg{display:block;width:100%;height:auto}
.legend{display:flex;flex-wrap:wrap;gap:10px 16px;margin-top:8px;font-size:12px;color:var(--muted)}
.legend span{display:inline-flex;align-items:center;gap:6px}
.legend i{width:10px;height:10px;border-radius:2px;display:inline-block}
footer{color:var(--muted);font-size:12px;text-align:center;padding:16px 0 28px}
.stat-row{display:flex;flex-wrap:wrap;gap:12px;margin-bottom:8px}
.stat{flex:1 1 120px;background:#141c28;border:1px solid var(--border);border-radius:8px;padding:12px 14px}
.stat .v{font-size:22px;font-weight:600;color:var(--ink)}
.stat .k{font-size:12px;color:var(--muted);margin-top:2px}
.stat.good .v{color:var(--ok)} .stat.bad .v{color:var(--bad)} .stat.warn .v{color:var(--warn)}
</style>
</head>
<body>
<header>
<h1>`)
	b.WriteString(html.EscapeString(r.Title))
	b.WriteString(`</h1>
<div class="sub">`)
	b.WriteString(html.EscapeString(r.Subtitle))
	b.WriteString(`</div>
<div class="meta">`)
	for _, m := range r.meta {
		b.WriteString("<span>" + html.EscapeString(m[0]) + " <b>" + html.EscapeString(m[1]) + "</b></span>")
	}
	b.WriteString(`<span>生成时间 <b>`)
	b.WriteString(time.Now().Format("2006-01-02 15:04:05"))
	b.WriteString(`</b></span>
</div>
</header>
<main>
`)
	for _, s := range r.sections {
		b.WriteString("<section><h2>" + html.EscapeString(s.Title) + "</h2>")
		b.WriteString(s.HTML)
		b.WriteString("</section>\n")
	}
	b.WriteString(`</main>
<footer>gnd — game numerical design CLI · self-contained HTML · no external assets</footer>
</body>
</html>
`)
	return b.String()
}

// --- chart primitives (inline SVG) ---

// LineSeries is one line on a chart.
type LineSeries struct {
	Name   string
	Points []Point // X,Y
	Color  string
}

// Point on a chart.
type Point struct {
	X, Y float64
}

// BarItem is one bar.
type BarItem struct {
	Label string
	Value float64
	Color string
}

// LineChartConfig configures a multi-line chart.
type LineChartConfig struct {
	Title                  string
	Width                  int
	Height                 int
	XLabel                 string
	YLabel                 string
	XMin, XMax, YMin, YMax float64 // auto if equal
	Series                 []LineSeries
	ShowPoints             bool
}

// LineChartSVG renders a multi-series line chart.
func LineChartSVG(c LineChartConfig) string {
	w, h := c.Width, c.Height
	if w <= 0 {
		w = 640
	}
	if h <= 0 {
		h = 280
	}
	padL, padR, padT, padB := 48, 16, 28, 40
	plotW := w - padL - padR
	plotH := h - padT - padB

	xmin, xmax := c.XMin, c.XMax
	ymin, ymax := c.YMin, c.YMax
	hasData := false
	for _, s := range c.Series {
		for _, p := range s.Points {
			hasData = true
			if xmin == xmax {
				if p.X < xmin {
					xmin = p.X
				}
				if p.X > xmax {
					xmax = p.X
				}
			}
			if ymin == ymax {
				if p.Y < ymin {
					ymin = p.Y
				}
				if p.Y > ymax {
					ymax = p.Y
				}
			}
		}
	}
	if !hasData {
		return `<div class="chart-card"><h3>` + html.EscapeString(c.Title) + `</h3><p class="muted">无数据</p></div>`
	}
	// refine auto bounds when still equal
	if xmin == xmax {
		xmin -= 1
		xmax += 1
	}
	if ymin == ymax {
		ymin -= 1
		ymax += 1
	}
	if ymin > 0 && ymin < ymax*0.3 {
		// keep
	}
	if ymin == 0 && ymax > 0 {
		// ok
	}
	// pad y a bit
	yPad := (ymax - ymin) * 0.05
	if yPad == 0 {
		yPad = 1
	}
	ymin -= yPad
	ymax += yPad

	sx := func(x float64) float64 {
		return float64(padL) + (x-xmin)/(xmax-xmin)*float64(plotW)
	}
	sy := func(y float64) float64 {
		return float64(padT) + (1-(y-ymin)/(ymax-ymin))*float64(plotH)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<div class="chart-card"><h3>%s</h3><svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`,
		html.EscapeString(c.Title), w, h))
	// bg
	b.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" fill="#141c28"/>`, w, h))
	// grid + y labels
	for i := 0; i <= 4; i++ {
		yv := ymin + (ymax-ymin)*float64(i)/4
		y := sy(yv)
		b.WriteString(fmt.Sprintf(`<line x1="%d" y1="%.1f" x2="%d" y2="%.1f" stroke="%s" stroke-width="1"/>`,
			padL, y, w-padR, y, colorGrid))
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%.1f" fill="%s" font-size="10" text-anchor="end">%s</text>`,
			padL-6, y+3, colorMuted, fmtY(yv)))
	}
	// x labels
	for i := 0; i <= 4; i++ {
		xv := xmin + (xmax-xmin)*float64(i)/4
		x := sx(xv)
		b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%d" fill="%s" font-size="10" text-anchor="middle">%s</text>`,
			x, h-padB+16, colorMuted, fmtY(xv)))
	}
	// axis titles
	if c.XLabel != "" {
		b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="%s" font-size="11" text-anchor="middle">%s</text>`,
			padL+plotW/2, h-4, colorMuted, html.EscapeString(c.XLabel)))
	}
	if c.YLabel != "" {
		b.WriteString(fmt.Sprintf(`<text x="12" y="%d" fill="%s" font-size="11" transform="rotate(-90 12 %d)" text-anchor="middle">%s</text>`,
			padT+plotH/2, colorMuted, padT+plotH/2, html.EscapeString(c.YLabel)))
	}
	// lines
	for si, s := range c.Series {
		color := s.Color
		if color == "" {
			color = seriesColors[si%len(seriesColors)]
		}
		if len(s.Points) == 0 {
			continue
		}
		// sort by X copy
		pts := append([]Point(nil), s.Points...)
		sortPoints(pts)
		var path strings.Builder
		for i, p := range pts {
			if i == 0 {
				path.WriteString(fmt.Sprintf("M%.2f %.2f", sx(p.X), sy(p.Y)))
			} else {
				path.WriteString(fmt.Sprintf(" L%.2f %.2f", sx(p.X), sy(p.Y)))
			}
		}
		b.WriteString(fmt.Sprintf(`<path d="%s" fill="none" stroke="%s" stroke-width="2" stroke-linejoin="round"/>`,
			path.String(), color))
		if c.ShowPoints {
			for _, p := range pts {
				b.WriteString(fmt.Sprintf(`<circle cx="%.2f" cy="%.2f" r="3" fill="%s"/>`, sx(p.X), sy(p.Y), color))
			}
		}
	}
	b.WriteString(`</svg>`)
	// legend
	b.WriteString(`<div class="legend">`)
	for si, s := range c.Series {
		color := s.Color
		if color == "" {
			color = seriesColors[si%len(seriesColors)]
		}
		b.WriteString(`<span><i style="background:` + color + `"></i>` + html.EscapeString(s.Name) + `</span>`)
	}
	b.WriteString(`</div></div>`)
	return b.String()
}

// BarChartSVG renders a horizontal or vertical bar chart.
func BarChartSVG(title string, items []BarItem, width, height int, horizontal bool) string {
	if width <= 0 {
		width = 640
	}
	if height <= 0 {
		if horizontal {
			height = 40 + len(items)*28
		} else {
			height = 260
		}
	}
	if len(items) == 0 {
		return `<div class="chart-card"><h3>` + html.EscapeString(title) + `</h3><p>无数据</p></div>`
	}
	maxV := 0.0
	for _, it := range items {
		if it.Value > maxV {
			maxV = it.Value
		}
	}
	if maxV <= 0 {
		maxV = 1
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf(`<div class="chart-card"><h3>%s</h3><svg viewBox="0 0 %d %d" xmlns="http://www.w3.org/2000/svg">`,
		html.EscapeString(title), width, height))
	b.WriteString(fmt.Sprintf(`<rect width="%d" height="%d" fill="#141c28"/>`, width, height))

	if horizontal {
		padL, padR, padT := 100, 50, 8
		barH := 18
		gap := 10
		plotW := width - padL - padR
		for i, it := range items {
			color := it.Color
			if color == "" {
				color = seriesColors[i%len(seriesColors)]
			}
			y := padT + i*(barH+gap)
			w := it.Value / maxV * float64(plotW)
			b.WriteString(fmt.Sprintf(`<text x="%d" y="%d" fill="%s" font-size="11" text-anchor="end">%s</text>`,
				padL-8, y+13, colorMuted, html.EscapeString(trunc(it.Label, 14))))
			b.WriteString(fmt.Sprintf(`<rect x="%d" y="%d" width="%.1f" height="%d" rx="3" fill="%s" opacity="0.9"/>`,
				padL, y, w, barH, color))
			b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%d" fill="%s" font-size="11">%s</text>`,
				float64(padL)+w+6, y+13, colorInk, fmtY(it.Value)))
		}
	} else {
		padL, padR, padT, padB := 40, 12, 16, 48
		plotW := width - padL - padR
		plotH := height - padT - padB
		n := len(items)
		bw := float64(plotW) / float64(n) * 0.65
		step := float64(plotW) / float64(n)
		for i, it := range items {
			color := it.Color
			if color == "" {
				color = seriesColors[i%len(seriesColors)]
			}
			hh := it.Value / maxV * float64(plotH)
			x := float64(padL) + float64(i)*step + (step-bw)/2
			y := float64(padT) + float64(plotH) - hh
			b.WriteString(fmt.Sprintf(`<rect x="%.1f" y="%.1f" width="%.1f" height="%.1f" rx="3" fill="%s" opacity="0.9"/>`,
				x, y, bw, hh, color))
			b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%d" fill="%s" font-size="10" text-anchor="middle" transform="rotate(-35 %.1f %d)">%s</text>`,
				x+bw/2, height-padB+12, colorMuted, x+bw/2, height-padB+12, html.EscapeString(trunc(it.Label, 12))))
			b.WriteString(fmt.Sprintf(`<text x="%.1f" y="%.1f" fill="%s" font-size="10" text-anchor="middle">%s</text>`,
				x+bw/2, y-4, colorInk, fmtY(it.Value)))
		}
	}
	b.WriteString(`</svg></div>`)
	return b.String()
}

// HistogramSVG draws a histogram of integer buckets.
func HistogramSVG(title string, hist map[int]int, width, height int) string {
	if len(hist) == 0 {
		return BarChartSVG(title, nil, width, height, false)
	}
	keys := make([]int, 0, len(hist))
	for k := range hist {
		keys = append(keys, k)
	}
	sortInts(keys)
	items := make([]BarItem, len(keys))
	maxCount := 1
	for _, k := range keys {
		if hist[k] > maxCount {
			maxCount = hist[k]
		}
	}
	for i, k := range keys {
		items[i] = BarItem{Label: fmt.Sprint(k), Value: float64(hist[k])}
	}
	// vertical bars for turn histogram
	_ = maxCount
	return BarChartSVG(title, items, width, height, false)
}

// StatRow renders KPI cards.
func StatRow(stats [][3]string) string { // label, value, tone(good/bad/warn/"")
	var b strings.Builder
	b.WriteString(`<div class="stat-row">`)
	for _, s := range stats {
		tone := s[2]
		cls := "stat"
		if tone != "" {
			cls += " " + tone
		}
		b.WriteString(`<div class="` + cls + `"><div class="v">` +
			html.EscapeString(s[1]) + `</div><div class="k">` +
			html.EscapeString(s[0]) + `</div></div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

func sortPoints(pts []Point) {
	for i := 1; i < len(pts); i++ {
		for j := i; j > 0 && pts[j].X < pts[j-1].X; j-- {
			pts[j], pts[j-1] = pts[j-1], pts[j]
		}
	}
}

func sortInts(xs []int) {
	for i := 1; i < len(xs); i++ {
		for j := i; j > 0 && xs[j] < xs[j-1]; j-- {
			xs[j], xs[j-1] = xs[j-1], xs[j]
		}
	}
}

func fmtY(v float64) string {
	switch {
	case math.IsNaN(v) || math.IsInf(v, 0):
		return "-"
	case math.Abs(v) >= 10000:
		return fmt.Sprintf("%.0fk", v/1000)
	case math.Abs(v) >= 100:
		return fmt.Sprintf("%.0f", v)
	case math.Abs(v) >= 1:
		return fmt.Sprintf("%.1f", v)
	default:
		return fmt.Sprintf("%.2f", v)
	}
}

func trunc(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// ChartGrid wraps multiple chart cards, optionally 2-col.
func ChartGrid(twoCol bool, charts ...string) string {
	cls := "chart-grid"
	if twoCol {
		cls += " two"
	}
	return `<div class="` + cls + `">` + strings.Join(charts, "") + `</div>`
}
