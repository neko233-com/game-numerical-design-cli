package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/neko233-com/game-numerical-design-cli/internal/htmlreport"
	"github.com/neko233-com/game-numerical-design-cli/internal/simulator"
)

func (a *App) cmdReport(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd report <demo|sim|curve> [flags]

demo    从 goal 生成完整 HTML 数值报告（曲线/节奏/战斗/抽卡/配置表）
  --goal demo/goal.yaml
  --out  demo/report.html          (default: <goal-dir>/report.html)

sim     从一次模拟 run 生成 HTML 报告
  --root .gnd/sim --run <run_id>
  --out  .gnd/sim/<run_id>/report.html
  --sample 20                      日志抽样条数

curve   单独绘制成长曲线报告
  --kind piecewise-log --max-x 40 --n 40
  --a --b --s1 --s2 --break --out curve.html

默认输出 .html，自包含内联 SVG，可直接浏览器打开。
`)
		return 0
	}
	switch args[0] {
	case "demo":
		return a.reportDemo(args[1:])
	case "sim":
		return a.reportSim(args[1:])
	case "curve":
		return a.reportCurve(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown report subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) reportDemo(args []string) int {
	fs := a.newFlagSet("report demo")
	goalPath := fs.String("goal", "demo/goal.yaml", "goal file")
	out := fs.String("out", "", "output html path")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	g, gen, code := a.loadAndBuild(*goalPath)
	if code != 0 {
		return code
	}
	if *out == "" {
		dir := filepath.Dir(*goalPath)
		*out = filepath.Join(dir, "report.html")
	}
	rep := htmlreport.BuildDemoReport(gen, g)
	if err := rep.WriteFile(*out); err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "wrote %s\n", absPrint(*out))
	if abs, err := filepath.Abs(*out); err == nil {
		fmt.Fprintf(a.Stdout, "open: %s\n", abs)
	}
	return 0
}

func absPrint(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		return abs
	}
	return p
}

func (a *App) reportSim(args []string) int {
	fs := a.newFlagSet("report sim")
	root := fs.String("root", simulator.DefaultDir, "sim root")
	runID := fs.String("run", "", "run id (default: latest)")
	out := fs.String("out", "", "output html")
	sample := fs.Int("sample", 20, "battle log samples")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	r := simulator.NewRunner(*root)
	id := *runID
	if id == "" {
		list, err := r.Search(simulator.Query{Limit: 1})
		if err != nil || len(list) == 0 {
			return a.fail(fmt.Errorf("no sim runs under %s", *root))
		}
		id = list[0].RunID
	}
	man, err := r.GetManifest(id)
	if err != nil {
		return a.fail(err)
	}
	var battles []simulator.BattleLog
	names, _ := r.ListBattles(id)
	for i, n := range names {
		if i >= *sample {
			break
		}
		var bid int
		fmt.Sscanf(n, "battle_%d.json", &bid)
		if bl, err := r.GetBattle(id, bid); err == nil {
			battles = append(battles, *bl)
		}
	}
	if *out == "" {
		*out = filepath.Join(*root, id, "report.html")
	}
	rep := htmlreport.BuildSimReport(man, battles)
	if err := rep.WriteFile(*out); err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "wrote %s\nrun_id=%s\n", absPrint(*out), id)
	return 0
}

func (a *App) reportCurve(args []string) int {
	fs := a.newFlagSet("report curve")
	kind := fs.String("kind", "piecewise-log", "curve kind")
	out := fs.String("out", "curve-report.html", "output html")
	maxX := fs.Float64("max-x", 40, "max x")
	n := fs.Int("n", 40, "samples")
	var (
		pA, pB, pS1, pS2, pBreak, pL, pK, pX0 float64
	)
	fs.Float64Var(&pA, "a", 1, "a")
	fs.Float64Var(&pB, "b", 1, "b")
	fs.Float64Var(&pS1, "s1", 1.8, "s1")
	fs.Float64Var(&pS2, "s2", 0.6, "s2")
	fs.Float64Var(&pBreak, "break", 20, "break")
	fs.Float64Var(&pL, "l", 100, "sigmoid L")
	fs.Float64Var(&pK, "k", 0.25, "sigmoid k")
	fs.Float64Var(&pX0, "x0", 20, "sigmoid x0")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	pts, err := sampleCurve(*kind, *maxX, *n, pA, pB, pS1, pS2, pBreak, pL, pK, pX0)
	if err != nil {
		return a.fail(err)
	}
	var series []htmlreport.Point
	var growth []htmlreport.Point
	for i, p := range pts {
		series = append(series, htmlreport.Point{X: p.X, Y: p.Y})
		if i > 0 && pts[i-1].Y != 0 {
			gr := (p.Y - pts[i-1].Y) / pts[i-1].Y * 100
			growth = append(growth, htmlreport.Point{X: p.X, Y: gr})
		}
	}
	rep := htmlreport.New("成长曲线报告", *kind)
	rep.Meta("max_x", fmt.Sprintf("%g", *maxX))
	rep.Meta("n", strconv.Itoa(*n))
	rep.SectionRaw("曲线", htmlreport.ChartGrid(true,
		htmlreport.LineChartSVG(htmlreport.LineChartConfig{
			Title: "y(x)", XLabel: "x", YLabel: "y",
			Series: []htmlreport.LineSeries{{Name: *kind, Points: series}},
		}),
		htmlreport.LineChartSVG(htmlreport.LineChartConfig{
			Title: "相邻步增长率 %", XLabel: "x", YLabel: "Δ%",
			Series: []htmlreport.LineSeries{{Name: "growth%", Points: growth, Color: "#f0b429"}},
		}),
	))
	if err := rep.WriteFile(*out); err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "wrote %s\n", absPrint(*out))
	return 0
}

func sampleCurve(kind string, maxX float64, n int, a, b, s1, s2, brk, l, k, x0 float64) ([]struct{ X, Y float64 }, error) {
	// local import to avoid cycle — use curves package via thin wrapper
	return sampleCurveImpl(kind, maxX, n, a, b, s1, s2, brk, l, k, x0)
}

// ensure os used
var _ = os.Stdout
