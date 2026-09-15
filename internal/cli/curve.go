package cli

import (
	"fmt"
	"strconv"

	"github.com/neko233-com/game-numerical-design-cli/internal/curves"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
)

func (a *App) cmdCurve(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd curve <kind> [flags]
kinds:
`)
		for _, k := range curves.Kinds() {
			fmt.Fprintf(a.Stdout, "  %-14s %s\n", k, curves.Describe(k))
		}
		fmt.Fprint(a.Stdout, `
common flags:
  --a --b --c --l --k --x0 --base --s1 --s2 --break
  --max-x 100  --n 20  --format table|csv|json  --smooth
`)
		return 0
	}
	kind := curves.Kind(args[0])
	fs := a.newFlagSet("curve")
	var (
		p      curves.Params
		maxX   = fs.Float64("max-x", 100, "sample upper bound")
		n      = fs.Int("n", 20, "segments")
		format = fs.String("format", "table", "table|csv|json")
		smooth = fs.Bool("smooth", false, "run smoothness analysis")
		cliff  = fs.Float64("cliff", 0.35, "cliff threshold for --smooth")
	)
	fs.Float64Var(&p.A, "a", 1, "linear/exp/power a")
	fs.Float64Var(&p.B, "b", 1, "b")
	fs.Float64Var(&p.C, "c", 0, "quadratic c")
	fs.Float64Var(&p.L, "l", 1, "sigmoid L")
	fs.Float64Var(&p.K, "k", 0.2, "sigmoid k")
	fs.Float64Var(&p.X0, "x0", 50, "sigmoid x0")
	fs.Float64Var(&p.Base, "base", 0, "sigmoid base")
	fs.Float64Var(&p.S1, "s1", 1.8, "piecewise-log early slope")
	fs.Float64Var(&p.S2, "s2", 0.6, "piecewise-log late slope")
	fs.Float64Var(&p.BreakX, "break", 30, "piecewise-log break x")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}

	fmt.Fprintf(a.Stdout, "model: %s\nformula: %s\n\n", kind, curves.Describe(kind))

	if *smooth {
		rep, err := curves.AnalyzeSmoothness(kind, p, *maxX, *n, *cliff)
		if err != nil {
			return a.fail(err)
		}
		fmt.Fprintf(a.Stdout, "smoothness: max step %.1f%% at x=%.2f  cliff=%v flat=%v\n",
			rep.MaxAbsDeltaRatio*100, rep.AtX, rep.HasCliff, rep.HasLongFlat)
		for _, w := range rep.Warnings {
			fmt.Fprintf(a.Stdout, "  WARN %s\n", w)
		}
		ys := make([]float64, len(rep.Points))
		for i, pt := range rep.Points {
			ys[i] = pt.Y
		}
		fmt.Fprintf(a.Stdout, "sparkline: %s\n\n", report.MiniChart(ys, 48))
		if *format == "json" {
			return exitJSON(a, rep)
		}
	}

	pts, err := curves.Sample(kind, p, *maxX, *n)
	if err != nil {
		return a.fail(err)
	}
	switch *format {
	case "json":
		return exitJSON(a, pts)
	case "csv":
		rows := make([][]string, len(pts))
		for i, pt := range pts {
			rows[i] = []string{report.Ftoa(pt.X), report.Ftoa(pt.Y)}
		}
		if err := report.CSV(a.Stdout, []string{"x", "y"}, rows); err != nil {
			return a.fail(err)
		}
	default:
		rows := make([][]string, len(pts))
		for i, pt := range pts {
			gr, _ := curves.GrowthRate(kind, p, pt.X)
			rows[i] = []string{
				report.Ftoa(pt.X), report.Ftoa(pt.Y), fmt.Sprintf("%.2f%%", gr*100),
			}
		}
		if err := report.Table(a.Stdout, []string{"x", "y", "Δ%/step"}, rows); err != nil {
			return a.fail(err)
		}
	}
	return 0
}

func exitJSON(a *App, v any) int {
	if err := report.JSON(a.Stdout, v); err != nil {
		return a.fail(err)
	}
	return 0
}

func atoiDefault(s string, def int) int {
	if s == "" {
		return def
	}
	v, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return v
}
