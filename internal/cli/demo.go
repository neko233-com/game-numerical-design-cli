package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/neko233-com/game-numerical-design-cli/internal/goal"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
)

func (a *App) cmdDemo(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd demo <init|build|check|sim|goal> [flags]

init    脱敏脚手架：写出默认 goal + 生成配置表
  --dir demo              输出目录（默认 demo）
  --xlsx                  额外生成 xlsx

build   按 goal 文件生成配置表
  --goal demo/goal.yaml
  --out demo/configs
  --xlsx

check   生成后跑数值校验 + 目标对照
  --goal demo/goal.yaml

sim     用生成的表做战斗/节奏快检
  --goal demo/goal.yaml

goal    打印内置默认 goal（YAML）

「我给目标，你去配置」：改 goal.yaml 再 build/check。
`)
		return 0
	}
	switch args[0] {
	case "init":
		return a.demoInit(args[1:])
	case "build":
		return a.demoBuild(args[1:])
	case "check":
		return a.demoCheck(args[1:])
	case "sim":
		return a.demoSim(args[1:])
	case "goal":
		return a.demoGoalPrint(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown demo subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) demoInit(args []string) int {
	fs := a.newFlagSet("demo init")
	dir := fs.String("dir", "demo", "output directory")
	xlsx := fs.Bool("xlsx", false, "also write xlsx")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	goalPath := filepath.Join(*dir, "goal.yaml")
	cfgDir := filepath.Join(*dir, "configs")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return a.fail(err)
	}
	// write default goal yaml via json-ish manual? use Build then also dump goal file
	g := goal.DefaultGoal()
	if err := writeGoalYAML(goalPath, g); err != nil {
		return a.fail(err)
	}
	gen, err := goal.Build(g)
	if err != nil {
		return a.fail(err)
	}
	written, err := gen.WriteAll(cfgDir, *xlsx)
	if err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "demo ready\n  goal: %s\n  configs:\n", goalPath)
	for _, w := range written {
		fmt.Fprintf(a.Stdout, "    %s\n", w)
	}
	a.printReport(gen)
	fmt.Fprintf(a.Stdout, "\nnext:\n  1) edit %s\n  2) gnd demo check --goal %s\n  3) gnd table set demo/configs/HeroConfig.csv --id 1001 --field base_atk --value 620 --write\n",
		goalPath, goalPath)
	return 0
}

func (a *App) demoBuild(args []string) int {
	fs := a.newFlagSet("demo build")
	goalPath := fs.String("goal", "demo/goal.yaml", "goal file")
	out := fs.String("out", "demo/configs", "output dir")
	xlsx := fs.Bool("xlsx", false, "also write xlsx")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	g, gen, code := a.loadAndBuild(*goalPath)
	if code != 0 {
		return code
	}
	if err := os.MkdirAll(*out, 0o755); err != nil {
		return a.fail(err)
	}
	written, err := gen.WriteAll(*out, *xlsx)
	if err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "built from goal %q\n", g.Name)
	for _, w := range written {
		fmt.Fprintf(a.Stdout, "  %s\n", w)
	}
	a.printReport(gen)
	return 0
}

func (a *App) demoCheck(args []string) int {
	fs := a.newFlagSet("demo check")
	goalPath := fs.String("goal", "demo/goal.yaml", "goal file")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	g, gen, code := a.loadAndBuild(*goalPath)
	if code != 0 {
		return code
	}
	issues := gen.CheckTables()
	if *format == "json" {
		return exitJSON(a, map[string]any{
			"goal": g.Name, "report": gen.Report, "validate": issues,
		})
	}
	a.printReport(gen)
	if len(issues) > 0 {
		fmt.Fprintln(a.Stdout, "\nvalidate issues:")
		rows := make([][]string, len(issues))
		for i, is := range issues {
			rows[i] = []string{is.Severity, fmt.Sprint(is.RowID), is.Field, is.Message}
		}
		_ = report.Table(a.Stdout, []string{"sev", "id", "field", "msg"}, rows)
	}
	if gen.Report.OK && len(issues) == 0 {
		fmt.Fprintln(a.Stdout, "\nOK 目标对照与表校验均通过")
		return 0
	}
	fmt.Fprintln(a.Stdout, "\n存在告警 — 调整 goal 或手工 gnd table set 后重试")
	return 0
}

func (a *App) demoSim(args []string) int {
	fs := a.newFlagSet("demo sim")
	goalPath := fs.String("goal", "demo/goal.yaml", "goal file")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	_, gen, code := a.loadAndBuild(*goalPath)
	if code != 0 {
		return code
	}
	// focus on combat + pace
	fmt.Fprintf(a.Stdout, "=== combat (%s) ===\n", gen.Report.GoalName)
	rows := make([][]string, len(gen.Report.CombatChecks))
	for i, c := range gen.Report.CombatChecks {
		status := "OK"
		if !c.OK {
			status = "BAD"
		}
		rows[i] = []string{
			status, c.HeroName, c.VsEnemy,
			fmt.Sprintf("%.0f%%", c.WinRate*100),
			fmt.Sprintf("%.1f", c.AvgTurns), c.Note,
		}
	}
	_ = report.Table(a.Stdout, []string{"", "hero", "vs", "win", "turns", "note"}, rows)

	fmt.Fprintf(a.Stdout, "\n=== level pace (daily_exp from goal) ===\n")
	prows := make([][]string, 0, len(gen.Report.Pace))
	for _, p := range gen.Report.Pace {
		if p.TargetDays == 0 {
			continue
		}
		prows = append(prows, []string{
			fmt.Sprintf("Lv%d", p.Level),
			report.Ftoa(p.RequiredTotal),
			report.Ftoa(p.DaysNeeded),
			report.Ftoa(p.TargetDays),
			fmt.Sprintf("%+.0f%%", p.DeltaRatio*100),
			p.Verdict,
		})
	}
	_ = report.Table(a.Stdout, []string{"lv", "cum_exp", "days", "target", "Δ", "verdict"}, prows)

	fmt.Fprintf(a.Stdout, "\n=== gacha ===\n")
	_ = report.KeyValue(a.Stdout, [][2]string{
		{"5★ rate", fmt.Sprintf("%.3f%%", gen.Report.Gacha.BaseRate5*100)},
		{"soft pity", fmt.Sprintf("start=%d step=%.2f%%", gen.Report.Gacha.SoftStart, gen.Report.Gacha.SoftStep*100)},
		{"hard pity", fmt.Sprint(gen.Report.Gacha.HardPity)},
		{"featured", fmt.Sprintf("%.1f%%", gen.Report.Gacha.Featured*100)},
	})
	return 0
}

func (a *App) demoGoalPrint(args []string) int {
	g := goal.DefaultGoal()
	if err := writeGoalYAML("", g); err != nil && !strings.Contains(err.Error(), "empty path") {
		// print to stdout instead
	}
	// always print
	b, err := marshalGoalYAML(g)
	if err != nil {
		return a.fail(err)
	}
	fmt.Fprint(a.Stdout, string(b))
	return 0
}

func (a *App) loadAndBuild(goalPath string) (goal.Goal, *goal.Generated, int) {
	g, err := goal.LoadGoal(goalPath)
	if err != nil {
		// fallback: if missing, use default
		if os.IsNotExist(err) {
			g = goal.DefaultGoal()
		} else {
			return goal.Goal{}, nil, a.fail(err)
		}
	}
	gen, err := goal.Build(g)
	if err != nil {
		return goal.Goal{}, nil, a.fail(err)
	}
	return g, gen, 0
}

func (a *App) printReport(gen *goal.Generated) {
	r := gen.Report
	fmt.Fprintf(a.Stdout, "\nreport: %s  level_max=%d heroes=%d skills=%d tasks=%d enemies=%d ok=%v\n",
		r.GoalName, r.MaxLevel, r.HeroCount, r.SkillCount, r.TaskCount, r.EnemyCount, r.OK)
	if len(r.Warnings) > 0 {
		fmt.Fprintln(a.Stdout, "warnings:")
		for _, w := range r.Warnings {
			fmt.Fprintf(a.Stdout, "  - %s\n", w)
		}
	}
}

// writeGoalYAML dumps goal as YAML to path; empty path → no-op error sentinel.
func writeGoalYAML(path string, g goal.Goal) error {
	if path == "" {
		return fmt.Errorf("empty path")
	}
	b, err := marshalGoalYAML(g)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0o644)
}

func marshalGoalYAML(g goal.Goal) ([]byte, error) {
	return yaml.Marshal(g)
}
