package cli

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/neko233-com/game-numerical-design-cli/internal/balance"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
)

func (a *App) cmdBalance(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd balance <anchors|sensitivity|dimensions|winrate> [flags]

anchors
  --anchor "Boss ATK|1500|atk|基准怪攻击，来自 X 级副本通关模拟"
  (repeatable; role|value|unit|rationale)

sensitivity
  --base "atk=800,def=200,cr=0.2,cm=1.5"
  --rel 0.1
  --objective winrate
  Built-in objectives:
    winrate — closer attacker win rate to 0.5 is better (uses internal quick sim)

dimensions
  --dim "DPS|0.3|70|75"   name|weight|score|target  (repeatable)
  --gap-warn 15

winrate
  --observed 0.54 --target 0.5
`)
		return 0
	}
	switch args[0] {
	case "anchors":
		return a.balAnchors(args[1:])
	case "sensitivity":
		return a.balSensitivity(args[1:])
	case "dimensions":
		return a.balDimensions(args[1:])
	case "winrate":
		return a.balWinrate(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown balance subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) balAnchors(args []string) int {
	fs := a.newFlagSet("balance anchors")
	var anchors multiFlag
	fs.Var(&anchors, "anchor", "name|value|unit|rationale (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var list []balance.Anchor
	for _, s := range anchors {
		parts := strings.SplitN(s, "|", 4)
		if len(parts) < 2 {
			return a.fail(fmt.Errorf("bad --anchor %q", s))
		}
		v, err := strconv.ParseFloat(parts[1], 64)
		if err != nil {
			return a.fail(fmt.Errorf("bad anchor value in %q", s))
		}
		an := balance.Anchor{Name: parts[0], Value: v}
		if len(parts) > 2 {
			an.Unit = parts[2]
		}
		if len(parts) > 3 {
			an.Rationale = parts[3]
		}
		list = append(list, an)
	}
	if len(list) == 0 {
		return a.fail(fmt.Errorf("need at least one --anchor"))
	}
	warns := balance.ValidateAnchors(list)
	rows := make([][]string, len(list))
	for i, an := range list {
		rows[i] = []string{an.Name, report.Ftoa(an.Value), an.Unit, an.Rationale}
	}
	_ = report.Table(a.Stdout, []string{"anchor", "value", "unit", "rationale"}, rows)
	for _, w := range warns {
		fmt.Fprintf(a.Stdout, "WARN %s\n", w)
	}
	if len(warns) == 0 {
		fmt.Fprintln(a.Stdout, "OK 所有锚点均有依据")
	}
	return 0
}

func parseMap(s string) (map[string]float64, error) {
	out := map[string]float64{}
	if strings.TrimSpace(s) == "" {
		return out, nil
	}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			return nil, fmt.Errorf("bad pair %q, want k=v", part)
		}
		v, err := strconv.ParseFloat(strings.TrimSpace(kv[1]), 64)
		if err != nil {
			return nil, fmt.Errorf("bad value in %q", part)
		}
		out[strings.TrimSpace(kv[0])] = v
	}
	return out, nil
}

func (a *App) balSensitivity(args []string) int {
	fs := a.newFlagSet("balance sensitivity")
	baseStr := fs.String("base", "atk=800,def=200,skill=1.2,cr=0.2,cm=1.5", "base params k=v,...")
	rel := fs.Float64("rel", 0.1, "relative probe delta")
	objName := fs.String("objective", "winrate", "winrate")
	format := fs.String("format", "table", "table|json")
	// fixed opponent for winrate objective
	var (
		dhp, datk, ddef, dcr, dcm, dspd float64
		nSim                            int
		seed                            int64
	)
	fs.Float64Var(&dhp, "d-hp", 4000, "opponent HP")
	fs.Float64Var(&datk, "d-atk", 600, "opponent ATK")
	fs.Float64Var(&ddef, "d-def", 250, "opponent DEF")
	fs.Float64Var(&dcr, "d-crit-rate", 0.1, "opponent crit rate")
	fs.Float64Var(&dcm, "d-crit-mult", 1.5, "opponent crit mult")
	fs.Float64Var(&dspd, "d-speed", 1, "opponent speed")
	fs.IntVar(&nSim, "n", 400, "sim n per probe (keep small for speed)")
	fs.Int64Var(&seed, "seed", 42, "RNG seed")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	base, err := parseMap(*baseStr)
	if err != nil {
		return a.fail(err)
	}
	obj, err := makeObjective(*objName, map[string]float64{
		"d_hp": dhp, "d_atk": datk, "d_def": ddef,
		"d_cr": dcr, "d_cm": dcm, "d_speed": dspd,
	}, nSim, seed)
	if err != nil {
		return a.fail(err)
	}
	results, err := balance.Sensitivity(base, *rel, obj)
	if err != nil {
		return a.fail(err)
	}
	if *format == "json" {
		return exitJSON(a, results)
	}
	rows := make([][]string, len(results))
	for i, r := range results {
		rows[i] = []string{
			r.Param, report.Ftoa(r.BaseValue),
			fmt.Sprintf("%.1f%%", r.Elasticity*100),
			report.Ftoa(r.Derivative), report.Ftoa(r.Influence),
		}
	}
	if err := report.Table(a.Stdout,
		[]string{"param", "base", "elasticity", "deriv", "influence"}, rows); err != nil {
		return a.fail(err)
	}
	fmt.Fprintln(a.Stdout, "\n优先精调 influence 最高的 2~3 个参数。")
	return 0
}

func (a *App) balDimensions(args []string) int {
	fs := a.newFlagSet("balance dimensions")
	var dim multiFlag
	gapWarn := fs.Float64("gap-warn", 15, "gap warn threshold")
	format := fs.String("format", "table", "table|json")
	fs.Var(&dim, "dim", "name|weight|score|target (repeatable)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	var dims []balance.Dimension
	for _, s := range dim {
		parts := strings.Split(s, "|")
		if len(parts) < 3 {
			return a.fail(fmt.Errorf("bad --dim %q", s))
		}
		w, _ := strconv.ParseFloat(parts[1], 64)
		sc, _ := strconv.ParseFloat(parts[2], 64)
		d := balance.Dimension{Name: parts[0], Weight: w, Score: sc}
		if len(parts) > 3 {
			d.Target, _ = strconv.ParseFloat(parts[3], 64)
		}
		dims = append(dims, d)
	}
	rep, err := balance.AnalyzeDimensions(dims, *gapWarn)
	if err != nil {
		return a.fail(err)
	}
	if *format == "json" {
		return exitJSON(a, rep)
	}
	fmt.Fprintf(a.Stdout, "weighted score: %.2f\nverdict: %s\n\n", rep.WeightedScore, rep.Verdict)
	if len(rep.Gaps) > 0 {
		rows := make([][]string, len(rep.Gaps))
		for i, g := range rep.Gaps {
			rows[i] = []string{
				g.Name, report.Ftoa(g.Score), report.Ftoa(g.Target),
				report.Ftoa(g.Gap), report.Ftoa(g.Weighted),
			}
		}
		_ = report.Table(a.Stdout,
			[]string{"dim", "score", "target", "gap", "w*gap"}, rows)
	}
	return 0
}

func (a *App) balWinrate(args []string) int {
	fs := a.newFlagSet("balance winrate")
	obs := fs.Float64("observed", 0.5, "observed win rate")
	tgt := fs.Float64("target", 0.5, "target win rate")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	d, v := balance.WinRateTarget(*obs, *tgt)
	fmt.Fprintf(a.Stdout, "delta: %+.1f pp\n%s\n", d*100, v)
	return 0
}
