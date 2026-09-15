package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/neko233-com/game-numerical-design-cli/internal/goal"
	"github.com/neko233-com/game-numerical-design-cli/internal/htmlreport"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
	"github.com/neko233-com/game-numerical-design-cli/internal/tablekit"
)

// Exit codes for production CI.
const (
	exitOK      = 0
	exitCheck   = 1
	exitConfirm = 3
	exitUsage   = 2
)

func (a *App) cmdApply(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd apply [one-line] [flags]

一句话配置数值 + 自校验（生产级）。

配新（目录为空或不存在）:
  gnd apply "preset=slg max_level=30 win_rate=0.52" --dir out --write
  gnd apply --preset onmyoji --dir out --max-level 40 --win-rate 0.58 --write

改旧（目录已有 goal/configs）:
  gnd apply --goal out/goal.yaml --dir out --write
  # 需二次确认：交互输入 yes，或非交互加 --yes

Flags:
  --preset genshin|slg|onmyoji
  --goal PATH          以已有 goal 为底（改旧）
  --dir PATH           工作目录（默认 apply-out）
  --name NAME          覆盖目标名
  --max-level N --daily-exp N --win-rate F --hard-pity N
  --stat-gain F --base-rate-5 F
  --xlsx               额外输出 xlsx
  --write              落盘（默认 dry-run 只出计划）
  --yes                覆盖已有配置时跳过二次确认
  --sim N              写盘后跑 N 场模拟冒烟（0=跳过）
  --format table|json
  --strict             告警也视为失败（exit 1）

Exit: 0=成功 1=自校验失败 2=用法 3=需要确认未通过
`)
		return 0
	}

	fs := a.newFlagSet("apply")
	var (
		preset, goalPath, dir, name, format, oneLine string
		maxLevel, hardPity, simN                     int
		dailyExp, winRate, turns, statGain, rate5    float64
		xlsx, write, yes, strict                     bool
	)
	// optional one-liner positional
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		oneLine = args[0]
		args = args[1:]
	}
	fs.StringVar(&preset, "preset", string(goal.PresetGenshinRPG), "preset id")
	fs.StringVar(&goalPath, "goal", "", "existing goal file (改旧)")
	fs.StringVar(&dir, "dir", "apply-out", "workspace dir")
	fs.StringVar(&name, "name", "", "goal name override")
	fs.IntVar(&maxLevel, "max-level", 0, "max level")
	fs.Float64Var(&dailyExp, "daily-exp", 0, "daily exp")
	fs.Float64Var(&winRate, "win-rate", 0, "target win rate 0..1")
	fs.Float64Var(&turns, "target-turns", 0, "target battle turns")
	fs.IntVar(&hardPity, "hard-pity", 0, "5★ hard pity")
	fs.Float64Var(&statGain, "stat-gain", 0, "stat gain % per level")
	fs.Float64Var(&rate5, "base-rate-5", 0, "5★ base rate 0..1")
	fs.BoolVar(&xlsx, "xlsx", false, "also write xlsx")
	fs.BoolVar(&write, "write", false, "persist files (default dry-run)")
	fs.BoolVar(&yes, "yes", false, "skip confirm when overwriting existing")
	fs.IntVar(&simN, "sim", 0, "post-write sim battles")
	fs.StringVar(&format, "format", "table", "table|json")
	fs.BoolVar(&strict, "strict", false, "fail on warnings too")
	if err := parseTableFlags(fs, args); err != nil {
		return exitUsage
	}

	res, err := goal.ResolveApply(goal.ApplyOptions{
		Preset: preset, FromGoal: goalPath, Name: name, OneLine: oneLine,
		MaxLevel: maxLevel, DailyExp: dailyExp, WinRate: winRate,
		TargetTurns: turns, HardPity: hardPity, BaseRate5: rate5,
		StatGainPct: statGain,
	})
	if err != nil {
		return a.fail(err)
	}
	ch := res.SelfCheck()
	goalFile := filepath.Join(dir, "goal.yaml")
	cfgDir := filepath.Join(dir, "configs")
	repPath := filepath.Join(dir, "report.html")

	existing := destHasConfigs(cfgDir)
	mode := "create"
	if existing || (goalPath != "" && fileExists(goalFile)) {
		mode = "modify"
	}

	// build plan
	type planFile struct {
		Path string `json:"path"`
		Kind string `json:"kind"` // new | overwrite
	}
	var plan []planFile
	for _, name := range []string{
		"HeroConfig", "HeroLvUpConfig", "SkillConfig", "EnemyConfig",
		"TaskConfig", "ItemConfig", "GachaPoolConfig",
	} {
		p := filepath.Join(cfgDir, name+".csv")
		k := "new"
		if fileExists(p) {
			k = "overwrite"
		}
		plan = append(plan, planFile{Path: p, Kind: k})
		if xlsx {
			xp := filepath.Join(cfgDir, name+".xlsx")
			xk := "new"
			if fileExists(xp) {
				xk = "overwrite"
			}
			plan = append(plan, planFile{Path: xp, Kind: xk})
		}
	}
	gk := "new"
	if fileExists(goalFile) {
		gk = "overwrite"
	}
	plan = append([]planFile{{Path: goalFile, Kind: gk}, {Path: repPath, Kind: mapKind(repPath)}}, plan...)

	if format == "json" {
		payload := map[string]any{
			"mode": mode, "dir": dir, "goal": res.Goal, "changed": res.Changed,
			"self_check": ch, "plan": plan, "write": write, "existing": existing,
		}
		if !write {
			payload["status"] = "dry-run"
			return exitJSONCode(a, payload, exitOK)
		}
	} else {
		fmt.Fprintf(a.Stdout, "=== apply %s ===\n", mode)
		fmt.Fprintf(a.Stdout, "goal: %s  genre=%s  levels=%d\n", res.Goal.Name, res.Goal.Genre, res.Goal.Progress.MaxLevel)
		if len(res.Changed) > 0 {
			fmt.Fprintf(a.Stdout, "overrides: %s\n", strings.Join(res.Changed, ", "))
		}
		fmt.Fprintf(a.Stdout, "self_check: ok=%v  validate_err=%d warn=%d  combat_bad=%d pace_bad=%d\n",
			ch.OK, ch.ValidateErrs, ch.ValidateWarn, ch.CombatBad, ch.PaceBad)
		fmt.Fprintf(a.Stdout, "shape: heroes=%d skills=%d tasks=%d enemies=%d\n",
			ch.HeroRows, ch.SkillRows, ch.TaskRows, ch.EnemyRows)
		if len(ch.Warnings) > 0 {
			fmt.Fprintln(a.Stdout, "warnings:")
			for _, w := range ch.Warnings {
				fmt.Fprintf(a.Stdout, "  - %s\n", w)
			}
		}
		fmt.Fprintln(a.Stdout, "\nplan:")
		rows := make([][]string, len(plan))
		for i, p := range plan {
			rows[i] = []string{p.Kind, p.Path}
		}
		_ = report.Table(a.Stdout, []string{"op", "path"}, rows)
	}

	// hard fail self-check
	if !ch.OK {
		if format != "json" {
			fmt.Fprintln(a.Stdout, "\nFAIL 自校验未通过 — 未写盘")
		}
		return exitCheck
	}
	if strict && len(ch.Warnings) > 0 {
		if format != "json" {
			fmt.Fprintln(a.Stdout, "\nFAIL --strict 下存在告警 — 未写盘")
		}
		return exitCheck
	}

	if !write {
		if format != "json" {
			fmt.Fprintln(a.Stdout, "\ndry-run only — 加 --write 落盘")
			if mode == "modify" {
				fmt.Fprintln(a.Stdout, "检测到已有配置：--write 时将要求二次确认（或 --yes）")
			}
		}
		return exitOK
	}

	// 二次确认：改旧
	if mode == "modify" && !yes {
		if format != "json" {
			fmt.Fprintf(a.Stdout, "\n目标目录已有配置，将覆盖 %s\n", cfgDir)
		}
		ok, err := a.confirm("确认覆盖？输入 yes 继续: ")
		if err != nil {
			return a.fail(err)
		}
		if !ok {
			if format != "json" {
				fmt.Fprintln(a.Stdout, "已取消（需要 --yes 或交互确认）")
			}
			return exitConfirm
		}
	}

	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		return a.fail(err)
	}
	if err := writeGoalYAML(goalFile, res.Goal); err != nil {
		return a.fail(err)
	}
	written, err := res.Generated.WriteAll(cfgDir, xlsx)
	if err != nil {
		return a.fail(err)
	}
	if err := htmlreport.BuildDemoReport(res.Generated, res.Goal).WriteFile(repPath); err != nil {
		return a.fail(err)
	}

	// post-write verify: reload tables from disk
	if err := verifyWritten(cfgDir); err != nil {
		return a.fail(fmt.Errorf("post-write verify: %w", err))
	}

	if format == "json" {
		return exitJSONCode(a, map[string]any{
			"status": "written", "mode": mode, "dir": dir,
			"goal_file": goalFile, "report": repPath, "configs": written,
			"self_check": ch,
		}, exitOK)
	}
	fmt.Fprintf(a.Stdout, "\nwrote %d files\n  goal: %s\n  report: %s\n", len(written)+2, goalFile, repPath)

	if simN > 0 {
		if code := a.runSmokeSim(cfgDir, simN); code != 0 {
			return code
		}
	}
	if format != "json" {
		fmt.Fprintln(a.Stdout, "OK apply 完成")
	}
	return exitOK
}

func mapKind(p string) string {
	if fileExists(p) {
		return "overwrite"
	}
	return "new"
}

func destHasConfigs(cfgDir string) bool {
	for _, n := range []string{"HeroConfig.csv", "goal.yaml"} {
		if fileExists(filepath.Join(cfgDir, n)) {
			return true
		}
	}
	// goal may sit beside configs
	if fileExists(filepath.Join(filepath.Dir(cfgDir), "goal.yaml")) {
		return true
	}
	return fileExists(filepath.Join(cfgDir, "HeroConfig.csv"))
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

func (a *App) confirm(prompt string) (bool, error) {
	// non-TTY / CI without --yes should fail rather than hang
	fi, err := os.Stdin.Stat()
	if err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return false, nil
	}
	fmt.Fprint(a.Stdout, prompt)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		// EOF / cancel → not confirmed
		return false, nil
	}
	line = strings.TrimSpace(strings.ToLower(line))
	return line == "yes" || line == "y", nil
}

func verifyWritten(cfgDir string) error {
	for _, name := range []string{"HeroConfig", "SkillConfig", "EnemyConfig", "TaskConfig", "ItemConfig"} {
		p := filepath.Join(cfgDir, name+".csv")
		t, err := tablekit.Load(p, tablekit.Options{})
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if len(t.Headers) < 3 || len(t.Rows) < 1 {
			return fmt.Errorf("%s: empty table", name)
		}
	}
	return nil
}

func (a *App) runSmokeSim(cfgDir string, n int) int {
	// reuse sim from config via internal call path
	hero := firstID(filepath.Join(cfgDir, "HeroConfig.csv"))
	enemy := firstID(filepath.Join(cfgDir, "EnemyConfig.csv"))
	simRoot := filepath.Join(filepath.Dir(cfgDir), "sim-smoke")
	code := a.simRun([]string{
		"--n", fmt.Sprint(n), "--workers", "8", "--seed", "1",
		"--from-config", cfgDir, "--hero-id", hero, "--enemy-id", enemy,
		"--level", "10", "--root", simRoot, "--name", "apply-smoke",
	})
	return code
}

func firstID(csvPath string) string {
	data, err := os.ReadFile(csvPath)
	if err != nil {
		return "1001"
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) < 2 {
		return "1001"
	}
	parts := strings.Split(lines[1], ",")
	if len(parts) == 0 {
		return "1001"
	}
	return strings.TrimSpace(parts[0])
}

func exitJSONCode(a *App, v any, code int) int {
	enc := json.NewEncoder(a.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return a.fail(err)
	}
	return code
}
