package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/neko233-com/game-numerical-design-cli/internal/economy"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
)

func (a *App) cmdEconomy(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd economy <balance|project|pace|loop> [flags]

balance
  --name gold --prod 1200 --cons 1000 --stock 50000 --format table|json

project
  --prod 1200 --cons 1000 --stock 50000 --periods 14 --decay 0 --format table|json

pace
  --daily-exp 800 --levels 100,300,800,2000 --targets 0.5,1,2,4
  (levels = cumulative exp required; targets = design days)

loop
  --stage "source|金币产出|1000|" --stage "sink|强化|900|"
  --max-leak 0.1
`)
		return 0
	}
	switch args[0] {
	case "balance":
		return a.ecoBalance(args[1:])
	case "project":
		return a.ecoProject(args[1:])
	case "pace":
		return a.ecoPace(args[1:])
	case "loop":
		return a.ecoLoop(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown economy subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) ecoBalance(args []string) int {
	fs := a.newFlagSet("economy balance")
	var (
		name   string
		prod   float64
		cons   float64
		stock  float64
		format string
	)
	fs.StringVar(&name, "name", "currency", "currency name")
	fs.Float64Var(&prod, "prod", 0, "production per period")
	fs.Float64Var(&cons, "cons", 0, "consumption per period")
	fs.Float64Var(&stock, "stock", 0, "circulating stock (optional)")
	fs.StringVar(&format, "format", "table", "table|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rep := economy.AnalyzeBalance(economy.Flow{
		Name: name, Production: prod, Consumption: cons, Stock: stock,
	})
	if format == "json" {
		return exitJSON(a, rep)
	}
	kv := [][2]string{
		{"name", rep.Name},
		{"prod/cons", report.Ftoa(rep.Ratio)},
		{"net", report.Ftoa(rep.Net)},
		{"severity", rep.Severity},
		{"verdict", rep.Verdict},
		{"inflation risk", rep.InflationRisk},
	}
	for _, r := range rep.Recommendations {
		kv = append(kv, [2]string{"→", r})
	}
	_ = report.KeyValue(a.Stdout, kv)
	return 0
}

func (a *App) ecoProject(args []string) int {
	fs := a.newFlagSet("economy project")
	var (
		prod, cons, stock, decay float64
		periods                  int
		format                   string
	)
	fs.Float64Var(&prod, "prod", 0, "production per period")
	fs.Float64Var(&cons, "cons", 0, "consumption per period")
	fs.Float64Var(&stock, "stock", 0, "initial stock")
	fs.Float64Var(&decay, "decay", 0, "stock decay rate 0..1 per period")
	fs.IntVar(&periods, "periods", 14, "periods to project")
	fs.StringVar(&format, "format", "table", "table|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	series, err := economy.ProjectStock(stock, economy.Flow{Production: prod, Consumption: cons}, periods, decay)
	if err != nil {
		return a.fail(err)
	}
	if format == "json" {
		return exitJSON(a, series)
	}
	ys := make([]float64, len(series))
	copy(ys, series)
	fmt.Fprintf(a.Stdout, "sparkline: %s\n", report.MiniChart(ys, 48))
	rows := make([][]string, len(series))
	for i, v := range series {
		rows[i] = []string{strconv.Itoa(i), report.Ftoa(v)}
	}
	if err := report.Table(a.Stdout, []string{"t", "stock"}, rows); err != nil {
		return a.fail(err)
	}
	return 0
}

func parseFloats(s string) ([]float64, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	parts := strings.Split(s, ",")
	out := make([]float64, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		v, err := strconv.ParseFloat(p, 64)
		if err != nil {
			return nil, fmt.Errorf("bad number %q", p)
		}
		out = append(out, v)
	}
	return out, nil
}

func (a *App) ecoPace(args []string) int {
	fs := a.newFlagSet("economy pace")
	var (
		daily  float64
		levels string
		targs  string
		format string
	)
	fs.Float64Var(&daily, "daily-exp", 800, "daily exp production")
	fs.StringVar(&levels, "levels", "", "comma cumulative exp required per level")
	fs.StringVar(&targs, "targets", "", "comma target days per level")
	fs.StringVar(&format, "format", "table", "table|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	lv, err := parseFloats(levels)
	if err != nil {
		return a.fail(err)
	}
	tg, err := parseFloats(targs)
	if err != nil {
		return a.fail(err)
	}
	reps, err := economy.AnalyzePace(economy.PaceConfig{
		LevelExp: lv, DailyExp: daily, TargetDays: tg,
	})
	if err != nil {
		return a.fail(err)
	}
	if format == "json" {
		return exitJSON(a, reps)
	}
	rows := make([][]string, len(reps))
	for i, r := range reps {
		rows[i] = []string{
			strconv.Itoa(r.Level), report.Ftoa(r.RequiredTotal),
			report.Ftoa(r.DaysNeeded), report.Ftoa(r.TargetDays),
			fmt.Sprintf("%.1f%%", r.DeltaRatio*100), r.Verdict,
		}
	}
	if err := report.Table(a.Stdout,
		[]string{"lv", "req_total", "days", "target", "Δ", "verdict"}, rows); err != nil {
		return a.fail(err)
	}
	return 0
}

func (a *App) ecoLoop(args []string) int {
	fs := a.newFlagSet("economy loop")
	var stages multiFlag
	var maxLeak float64
	var format string
	fs.Var(&stages, "stage", "role|name|in|out  (repeatable), e.g. source|金币|0|1000")
	fs.Float64Var(&maxLeak, "max-leak", 0.1, "max |leak|/total_in")
	fs.StringVar(&format, "format", "table", "table|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var parsed []economy.LoopStage
	for _, s := range stages {
		parts := strings.Split(s, "|")
		if len(parts) < 3 {
			return a.fail(fmt.Errorf("bad --stage %q, want role|name|in|out", s))
		}
		st := economy.LoopStage{Role: parts[0], Name: parts[1]}
		in, _ := strconv.ParseFloat(parts[2], 64)
		st.Inflow = in
		if len(parts) > 3 {
			out, _ := strconv.ParseFloat(parts[3], 64)
			st.Outflow = out
		}
		parsed = append(parsed, st)
	}
	if len(parsed) == 0 {
		return a.fail(fmt.Errorf("need at least one --stage"))
	}
	rep := economy.AnalyzeLoop(parsed, maxLeak)
	if format == "json" {
		return exitJSON(a, rep)
	}
	kv := [][2]string{
		{"total_in", report.Ftoa(rep.TotalIn)},
		{"total_out", report.Ftoa(rep.TotalOut)},
		{"leak", report.Ftoa(rep.Leak)},
		{"closed", fmt.Sprint(rep.IsClosed)},
	}
	for _, w := range rep.Warnings {
		kv = append(kv, [2]string{"WARN", w})
	}
	_ = report.KeyValue(a.Stdout, kv)
	return 0
}

// multiFlag collects repeated string flags.
type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ";") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}
