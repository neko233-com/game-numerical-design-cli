package cli

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/neko233-com/game-numerical-design-cli/internal/combat"
	"github.com/neko233-com/game-numerical-design-cli/internal/curves"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
	"github.com/neko233-com/game-numerical-design-cli/internal/script"
	"github.com/neko233-com/game-numerical-design-cli/internal/simulator"
	"github.com/neko233-com/game-numerical-design-cli/internal/tablekit"
)

// hostAPI implements script.HostAPI for the CLI.
type hostAPI struct {
	app *App
}

func (h *hostAPI) Curve(kind string, x float64, params map[string]float64) (float64, error) {
	return curves.Evaluate(curves.Kind(kind), x, curves.Params{
		A: params["a"], B: params["b"], C: params["c"],
		L: params["l"], K: params["k"], X0: params["x0"], Base: params["base"],
		S1: params["s1"], S2: params["s2"], BreakX: params["break"],
	})
}

func (h *hostAPI) Damage(in map[string]float64, typ string) (float64, error) {
	skill := in["skill"]
	if skill == 0 {
		skill = 1
	}
	cm := in["crit_mult"]
	if cm == 0 {
		cm = 1.5
	}
	return combat.ComputeDamage(combat.DamageInput{
		Attack: in["atk"], Defense: in["def"], SkillCoeff: skill,
		CritRate: in["crit_rate"], CritMult: cm, Mitigation: in["mitigation"],
		Type: combat.AttackType(typ),
	})
}

func (h *hostAPI) SimBattle(attacker, defender map[string]any, n int, seed int64) (map[string]any, error) {
	if n <= 0 {
		n = 200
	}
	if n > 10000 {
		n = 10000
	}
	a := toCombatUnit(simulator.FromMap(attacker))
	d := toCombatUnit(simulator.FromMap(defender))
	if seed == 0 {
		seed = 1
	}
	res, err := combat.Simulate(a, d, n, rand.New(rand.NewSource(seed)))
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"n": res.N, "win_rate": res.AttackerWinRate,
		"avg_turns": res.AvgTurns, "p50_turns": res.P50Turns, "p90_turns": res.P90Turns,
		"avg_dmg_dealt": res.AvgDamageDealt, "avg_dmg_taken": res.AvgDamageTaken,
	}, nil
}

func toCombatUnit(u simulator.Unit) combat.Unit {
	speed := u.Speed
	if speed <= 0 {
		speed = 100
	}
	cm := 1 + u.CritDMG
	if u.CritDMG == 0 {
		cm = 1.5
	}
	sm := u.SkillMult
	if sm == 0 {
		sm = 1
	}
	acc := u.Accuracy
	if acc == 0 {
		acc = 1
	}
	return combat.Unit{
		Name: u.Name, HP: u.HP, Attack: u.Attack, Defense: u.Defense,
		CritRate: u.CritRate, CritMult: cm, Speed: speed / 100,
		SkillCoeff: sm, Acc: acc, Evasion: u.Evasion,
	}
}

func (h *hostAPI) LoadTable(path string, sheet string) (map[string]any, error) {
	t, err := tablekit.Load(path, tablekit.Options{Sheet: sheet})
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"name": t.Name, "headers": t.Headers, "rows": t.Rows, "count": len(t.Rows),
	}, nil
}

func (h *hostAPI) ReadJSON(path string) (any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return v, nil
}

func (h *hostAPI) WriteJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	if dir := filepath.Dir(path); dir != "" && dir != "." {
		_ = os.MkdirAll(dir, 0o755)
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func (h *hostAPI) Log(args ...any) {
	fmt.Fprintln(h.app.Stdout, args...)
}

func (a *App) cmdScript(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd script run <file.ts|file.js>
       gnd script eval '<inline ts>'

TypeScript/JavaScript runs in-process (esbuild transpile + goja).
No Node.js install required. Host API on global "gnd":

  gnd.curve(kind, x, params)
  gnd.damage({atk,def,skill,...}, type)
  gnd.simBattle(attacker, defender, n, seed)
  gnd.loadTable(path, sheet)  → {headers, rows}
  gnd.readJSON(path) / gnd.writeJSON(path, v)
  gnd.log(...) / console.log(...)

Example (balance.ts):
  const t = gnd.loadTable("demo/configs/HeroConfig.csv", "")
  const row = t.rows[0]
  const hero = Object.fromEntries(t.headers.map((h, i) => [h, row[i]]))
  const res = gnd.simBattle({
    name: hero.name, hp: +hero.base_hp, atk: +hero.base_atk,
    def: +hero.base_def, spd: +hero.base_spd,
    crit_rate: +hero.base_crit_rate / 100, skill_mult: 1.4
  }, { name: "dummy", hp: 5000, atk: 400, def: 200, spd: 90 }, 200, 1)
  gnd.log("win_rate", res.win_rate)
`)
		return 0
	}
	switch args[0] {
	case "run":
		return a.scriptRun(args[1:])
	case "eval":
		return a.scriptEval(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown script subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) scriptRun(args []string) int {
	fs := a.newFlagSet("script run")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		return a.fail(fmt.Errorf("need script file path"))
	}
	path := fs.Arg(0)
	wd := filepath.Dir(path)
	if abs, err := filepath.Abs(wd); err == nil {
		wd = abs
	}
	res, err := script.RunFile(path, script.Options{
		Host: &hostAPI{app: a}, WorkingDir: wd, TimeoutMS: 60000,
	})
	if err != nil {
		return a.fail(err)
	}
	return a.finishScript(res, *format)
}

func (a *App) scriptEval(args []string) int {
	fs := a.newFlagSet("script eval")
	format := fs.String("format", "table", "table|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) < 1 {
		return a.fail(fmt.Errorf("need inline script"))
	}
	res, err := script.Run("eval.ts", rest[0], script.Options{
		Host: &hostAPI{app: a}, TimeoutMS: 60000,
	})
	if err != nil {
		return a.fail(err)
	}
	return a.finishScript(res, *format)
}

func (a *App) finishScript(res *script.Result, format string) int {
	if format == "json" {
		return exitJSON(a, map[string]any{"value": res.Value, "logs": res.Logs})
	}
	if res.Value != nil {
		b, _ := json.MarshalIndent(res.Value, "", "  ")
		fmt.Fprintf(a.Stdout, "result:\n%s\n", b)
	}
	return 0
}

func (a *App) cmdSim(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd sim <run|list|show|battle|logs|clear>

run     并行模拟 N 场 1v1（默认清空历史；每场独立日志；并发≤1000）
  --n 1000 --workers 16 --seed 1 --name my-battle
  --a-hp --a-atk --a-def --a-spd --a-crit-rate --a-crit-dmg --a-skill
  --d-hp --d-atk ...
  --from-config demo/configs --hero-id 1001 --enemy-id 2020 --level 20
  --root .gnd/sim --log-every 1 --keep-prior

list / show / battle / logs / clear  查询与清理历史
`)
		return 0
	}
	switch args[0] {
	case "run":
		return a.simRun(args[1:])
	case "list":
		return a.simList(args[1:])
	case "show":
		return a.simShow(args[1:])
	case "battle":
		return a.simBattleOne(args[1:])
	case "logs":
		return a.simLogs(args[1:])
	case "clear":
		return a.simClear(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown sim subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) simRun(args []string) int {
	fs := a.newFlagSet("sim run")
	var (
		n, workers, logEvery, level     int
		seed                            int64
		name, root, format              string
		keepPrior                       bool
		aHP, aATK, aDEF, aSPD, aCR, aCD float64
		aSk                             float64
		dHP, dATK, dDEF, dSPD, dCR, dCD float64
		dSk                             float64
		fromCfg, heroID, enemyID        string
	)
	fs.IntVar(&n, "n", 100, "battle count")
	fs.IntVar(&workers, "workers", 0, "parallel workers (default CPU*4, cap 1000)")
	fs.Int64Var(&seed, "seed", 1, "RNG base seed")
	fs.StringVar(&name, "name", "sim", "run name")
	fs.StringVar(&root, "root", simulator.DefaultDir, "sim root dir")
	fs.StringVar(&format, "format", "table", "table|json")
	fs.BoolVar(&keepPrior, "keep-prior", false, "do not clear prior runs")
	fs.IntVar(&logEvery, "log-every", 1, "write log every k battles")
	fs.IntVar(&level, "level", 0, "level for --from-config stat mult")
	fs.StringVar(&fromCfg, "from-config", "", "config dir with HeroConfig/EnemyConfig")
	fs.StringVar(&heroID, "hero-id", "", "hero id")
	fs.StringVar(&enemyID, "enemy-id", "", "enemy id")

	fs.Float64Var(&aHP, "a-hp", 5000, "attacker HP")
	fs.Float64Var(&aATK, "a-atk", 800, "attacker ATK")
	fs.Float64Var(&aDEF, "a-def", 200, "attacker DEF")
	fs.Float64Var(&aSPD, "a-spd", 100, "attacker SPD")
	fs.Float64Var(&aCR, "a-crit-rate", 0.1, "attacker crit rate")
	fs.Float64Var(&aCD, "a-crit-dmg", 0.5, "attacker crit dmg bonus")
	fs.Float64Var(&aSk, "a-skill", 1.4, "attacker skill mult")

	fs.Float64Var(&dHP, "d-hp", 5000, "defender HP")
	fs.Float64Var(&dATK, "d-atk", 600, "defender ATK")
	fs.Float64Var(&dDEF, "d-def", 250, "defender DEF")
	fs.Float64Var(&dSPD, "d-spd", 95, "defender SPD")
	fs.Float64Var(&dCR, "d-crit-rate", 0.05, "defender crit rate")
	fs.Float64Var(&dCD, "d-crit-dmg", 0.5, "defender crit dmg bonus")
	fs.Float64Var(&dSk, "d-skill", 1.0, "defender skill mult")

	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}

	req := simulator.Request{
		Name: name, N: n, Seed: seed, Workers: workers,
		Root: root, LogEvery: logEvery,
		Attacker: simulator.Unit{
			Name: "attacker", HP: aHP, Attack: aATK, Defense: aDEF,
			Speed: aSPD, CritRate: aCR, CritDMG: aCD, SkillMult: aSk, Accuracy: 1,
		},
		Defender: simulator.Unit{
			Name: "defender", HP: dHP, Attack: dATK, Defense: dDEF,
			Speed: dSPD, CritRate: dCR, CritDMG: dCD, SkillMult: dSk, Accuracy: 1,
		},
	}
	if keepPrior {
		f := false
		req.ClearPrior = &f
	}
	if fromCfg != "" && heroID != "" {
		loaded, err := a.loadPairFromConfig(fromCfg, heroID, enemyID, level)
		if err != nil {
			return a.fail(err)
		}
		req.Attacker = loaded[0]
		req.Defender = loaded[1]
	}

	man, err := simulator.NewRunner(root).Run(req)
	if err != nil {
		return a.fail(err)
	}
	if format == "json" {
		return exitJSON(a, man)
	}
	_ = report.KeyValue(a.Stdout, [][2]string{
		{"run_id", man.RunID},
		{"name", man.Name},
		{"n", strconv.Itoa(man.N)},
		{"workers", strconv.Itoa(man.Workers)},
		{"win_rate", fmt.Sprintf("%.1f%%", man.WinRate*100)},
		{"draw_rate", fmt.Sprintf("%.1f%%", man.DrawRate*100)},
		{"avg_turns", fmt.Sprintf("%.2f", man.AvgTurns)},
		{"p50_turns", fmt.Sprintf("%.0f", man.P50Turns)},
		{"p90_turns", fmt.Sprintf("%.0f", man.P90Turns)},
		{"logs", strconv.Itoa(man.LogCount)},
		{"log_dir", man.LogDir},
		{"duration_ms", strconv.FormatInt(man.DurationMS, 10)},
	})
	return 0
}

func (a *App) loadPairFromConfig(dir, heroID, enemyID string, level int) ([2]simulator.Unit, error) {
	var out [2]simulator.Unit
	heroPath := filepath.Join(dir, "HeroConfig.csv")
	if _, err := os.Stat(heroPath); err != nil {
		heroPath = filepath.Join(dir, "HeroConfig.xlsx")
	}
	ht, err := tablekit.Load(heroPath, tablekit.Options{})
	if err != nil {
		return out, fmt.Errorf("load hero config: %w", err)
	}
	hi, err := ht.FindRow(heroID, tablekit.Options{})
	if err != nil {
		return out, err
	}
	get := func(t *tablekit.Table, row int, field string) string {
		ci := t.ColIndex(field)
		if ci < 0 || row < 0 || row >= len(t.Rows) || ci >= len(t.Rows[row]) {
			return ""
		}
		return t.Rows[row][ci]
	}
	f := func(s string) float64 {
		v, _ := strconv.ParseFloat(s, 64)
		return v
	}
	mult := 1.0
	if level > 0 {
		lvPath := filepath.Join(dir, "HeroLvUpConfig.csv")
		if lt, err := tablekit.Load(lvPath, tablekit.Options{}); err == nil {
			if ri, err := lt.FindRow(strconv.Itoa(level), tablekit.Options{IDField: "level"}); err == nil {
				note := get(lt, ri, "note")
				if strings.HasPrefix(note, "mult=") {
					fmt.Sscanf(note, "mult=%g", &mult)
				}
			}
		}
	}
	if mult <= 0 {
		mult = 1
	}
	out[0] = simulator.Unit{
		Name: get(ht, hi, "name"), ID: heroID,
		HP: f(get(ht, hi, "base_hp")) * mult, Attack: f(get(ht, hi, "base_atk")) * mult,
		Defense: f(get(ht, hi, "base_def")) * mult, Speed: f(get(ht, hi, "base_spd")),
		CritRate:  f(get(ht, hi, "base_crit_rate")) / 100,
		CritDMG:   f(get(ht, hi, "base_crit_dmg")) / 100,
		SkillMult: 1.4, Accuracy: 1,
	}
	if enemyID != "" {
		ePath := filepath.Join(dir, "EnemyConfig.csv")
		et, err := tablekit.Load(ePath, tablekit.Options{})
		if err != nil {
			return out, err
		}
		ei, err := et.FindRow(enemyID, tablekit.Options{})
		if err != nil {
			return out, err
		}
		out[1] = simulator.Unit{
			Name: get(et, ei, "name"), ID: enemyID,
			HP: f(get(et, ei, "hp")), Attack: f(get(et, ei, "atk")),
			Defense: f(get(et, ei, "def")), Speed: f(get(et, ei, "spd")),
			CritRate: 0.05, CritDMG: 0.5, SkillMult: 1, Accuracy: 1,
		}
	}
	return out, nil
}

func (a *App) simList(args []string) int {
	fs := a.newFlagSet("sim list")
	root := fs.String("root", simulator.DefaultDir, "sim root")
	q := fs.String("name", "", "name contains")
	tag := fs.String("tag", "", "tag key=value")
	limit := fs.Int("limit", 20, "max rows")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	query := simulator.Query{NameContains: *q, Limit: *limit}
	if *tag != "" {
		if i := strings.IndexByte(*tag, '='); i >= 0 {
			query.TagKey, query.TagValue = (*tag)[:i], (*tag)[i+1:]
		}
	}
	list, err := simulator.NewRunner(*root).Search(query)
	if err != nil {
		return a.fail(err)
	}
	if *format == "json" {
		return exitJSON(a, list)
	}
	if len(list) == 0 {
		fmt.Fprintln(a.Stdout, "no runs")
		return 0
	}
	rows := make([][]string, len(list))
	for i, e := range list {
		rows[i] = []string{
			e.RunID, e.Name, e.CreatedAt.Format("01-02 15:04:04"),
			strconv.Itoa(e.N), fmt.Sprintf("%.1f%%", e.WinRate*100),
			fmt.Sprintf("%.1f", e.AvgTurns), strconv.Itoa(e.LogCount),
		}
	}
	_ = report.Table(a.Stdout, []string{"run_id", "name", "time", "n", "win", "turns", "logs"}, rows)
	return 0
}

func (a *App) simShow(args []string) int {
	fs := a.newFlagSet("sim show")
	root := fs.String("root", simulator.DefaultDir, "sim root")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		return a.fail(fmt.Errorf("need run_id"))
	}
	m, err := simulator.NewRunner(*root).GetManifest(fs.Arg(0))
	if err != nil {
		return a.fail(err)
	}
	if *format == "json" {
		return exitJSON(a, m)
	}
	_ = report.KeyValue(a.Stdout, [][2]string{
		{"run_id", m.RunID}, {"name", m.Name}, {"n", strconv.Itoa(m.N)},
		{"win_rate", fmt.Sprintf("%.2f%%", m.WinRate*100)},
		{"avg_turns", fmt.Sprintf("%.2f", m.AvgTurns)},
		{"p50", fmt.Sprintf("%.0f", m.P50Turns)}, {"p90", fmt.Sprintf("%.0f", m.P90Turns)},
		{"log_dir", m.LogDir},
	})
	return 0
}

func (a *App) simBattleOne(args []string) int {
	fs := a.newFlagSet("sim battle")
	root := fs.String("root", simulator.DefaultDir, "sim root")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 2 {
		return a.fail(fmt.Errorf("need run_id battle_id"))
	}
	id, err := strconv.Atoi(fs.Arg(1))
	if err != nil {
		return a.fail(fmt.Errorf("battle_id must be int"))
	}
	bl, err := simulator.NewRunner(*root).GetBattle(fs.Arg(0), id)
	if err != nil {
		return a.fail(err)
	}
	if *format == "json" {
		return exitJSON(a, bl)
	}
	_ = report.KeyValue(a.Stdout, [][2]string{
		{"run_id", bl.RunID}, {"battle_id", strconv.Itoa(bl.BattleID)},
		{"seed", strconv.FormatInt(bl.Seed, 10)}, {"winner", bl.Winner},
		{"turns", strconv.Itoa(bl.Turns)},
		{"dmg_dealt", fmt.Sprintf("%.0f", bl.DmgDealt)},
		{"dmg_taken", fmt.Sprintf("%.0f", bl.DmgTaken)},
	})
	return 0
}

func (a *App) simLogs(args []string) int {
	fs := a.newFlagSet("sim logs")
	root := fs.String("root", simulator.DefaultDir, "sim root")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		return a.fail(fmt.Errorf("need run_id"))
	}
	names, err := simulator.NewRunner(*root).ListBattles(fs.Arg(0))
	if err != nil {
		return a.fail(err)
	}
	for _, n := range names {
		fmt.Fprintln(a.Stdout, n)
	}
	fmt.Fprintf(a.Stdout, "(%d files)\n", len(names))
	return 0
}

func (a *App) simClear(args []string) int {
	fs := a.newFlagSet("sim clear")
	root := fs.String("root", simulator.DefaultDir, "sim root")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if err := simulator.NewRunner(*root).Clear(); err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "cleared %s\n", *root)
	return 0
}
