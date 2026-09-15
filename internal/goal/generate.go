package goal

import (
	"fmt"
	"math"
	"math/rand"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/neko233-com/game-numerical-design-cli/internal/combat"
	"github.com/neko233-com/game-numerical-design-cli/internal/curves"
	"github.com/neko233-com/game-numerical-design-cli/internal/economy"
	"github.com/neko233-com/game-numerical-design-cli/internal/gacha"
	"github.com/neko233-com/game-numerical-design-cli/internal/tablekit"
	"github.com/neko233-com/game-numerical-design-cli/internal/validate"
)

// Generated holds all tables ready to write.
type Generated struct {
	Hero   *tablekit.Table
	LvUp   *tablekit.Table
	Skill  *tablekit.Table
	Enemy  *tablekit.Table
	Task   *tablekit.Table
	Item   *tablekit.Table
	Gacha  *tablekit.Table
	Report Report
}

// Build expands Goal into tables + report (does not write files).
func Build(g Goal) (*Generated, error) {
	if err := g.normalize(); err != nil {
		return nil, err
	}
	gen := &Generated{}

	// --- Level curve ---
	levels, cumExp, err := buildLevels(g.Progress)
	if err != nil {
		return nil, err
	}
	gen.LvUp = levelTable(levels, g.Progress)

	// pace check
	var levelExps []float64
	var targets []float64
	for _, lv := range levels {
		if lv.Level == 1 {
			levelExps = append(levelExps, 0)
		} else {
			levelExps = append(levelExps, lv.CumExp)
		}
		if t, ok := g.Progress.TargetDays[strconv.Itoa(lv.Level)]; ok {
			targets = append(targets, t)
		} else {
			targets = append(targets, 0)
		}
	}
	pace, _ := economy.AnalyzePace(economy.PaceConfig{
		LevelExp: levelExps, DailyExp: g.Progress.DailyExp, TargetDays: targets,
	})

	// --- Heroes & skills ---
	skills, enemies := buildCombatTables(g, levels, cumExp)
	gen.Skill = skillTable(skills)
	gen.Enemy = enemyTable(enemies)
	gen.Hero = heroTable(g, levels)

	// combat sim checks at key levels
	gen.Report.CombatChecks = runCombatChecks(g, skills, enemies, levels)

	// --- Tasks ---
	gen.Task = taskTable(g, cumExp)

	// --- Items ---
	gen.Item = itemTable(g)

	// --- Gacha ---
	softStart, softStep := gacha.SoftPitySuggest(g.Gacha.BaseRate5, g.Gacha.HardPity)
	// respect ratio if provided
	if g.Gacha.SoftStartRatio > 0 {
		softStart = int(float64(g.Gacha.HardPity) * g.Gacha.SoftStartRatio)
		if softStart < 1 {
			softStart = 1
		}
		steps := g.Gacha.HardPity - softStart
		if steps > 0 {
			softStep = (1 - g.Gacha.BaseRate5) / float64(steps)
		}
	}
	gen.Gacha = gachaTable(g, softStart, softStep)
	gen.Report.Gacha = GachaPreview{
		BaseRate5: g.Gacha.BaseRate5, HardPity: g.Gacha.HardPity,
		SoftStart: softStart, SoftStep: softStep, Featured: g.Gacha.FeaturedRate,
	}

	// --- Report meta ---
	gen.Report.GoalName = g.Name
	gen.Report.MaxLevel = g.Progress.MaxLevel
	gen.Report.HeroCount = len(g.Combat.Archetypes)
	gen.Report.SkillCount = len(skills)
	gen.Report.TaskCount = len(gen.Task.Rows)
	gen.Report.EnemyCount = len(enemies)
	gen.Report.Pace = pace
	for _, lv := range levels {
		gen.Report.LevelExp = append(gen.Report.LevelExp, LevelPoint{
			Level: lv.Level, ExpToNext: lv.ExpToNext, CumExp: lv.CumExp,
			Days: lv.CumExp / g.Progress.DailyExp, StatMult: lv.StatMult,
		})
	}

	// warnings
	for _, p := range pace {
		if p.TargetDays > 0 {
			switch {
			case p.DeltaRatio > 0.4:
				gen.Report.Warnings = append(gen.Report.Warnings,
					fmt.Sprintf("Lv%d 达成需 %.1f 天，超出目标 %.1f 天 %.0f%%",
						p.Level, p.DaysNeeded, p.TargetDays, p.DeltaRatio*100))
			case p.DeltaRatio < -0.4:
				gen.Report.Warnings = append(gen.Report.Warnings,
					fmt.Sprintf("Lv%d 达成仅需 %.1f 天，显著快于目标 %.1f 天",
						p.Level, p.DaysNeeded, p.TargetDays))
			}
		}
	}
	for _, c := range gen.Report.CombatChecks {
		if !c.OK {
			gen.Report.Warnings = append(gen.Report.Warnings,
				fmt.Sprintf("战斗 %s vs %s: win=%.0f%% turns=%.1f — %s",
					c.HeroName, c.VsEnemy, c.WinRate*100, c.AvgTurns, c.Note))
		}
	}
	// validate tables
	gen.Report.OK = len(gen.Report.Warnings) == 0
	return gen, nil
}

// WriteAll saves generated tables under dir as csv (and optional xlsx).
func (gen *Generated) WriteAll(dir string, writeXLSX bool) ([]string, error) {
	tables := map[string]*tablekit.Table{
		"HeroConfig":      gen.Hero,
		"HeroLvUpConfig":  gen.LvUp,
		"SkillConfig":     gen.Skill,
		"EnemyConfig":     gen.Enemy,
		"TaskConfig":      gen.Task,
		"ItemConfig":      gen.Item,
		"GachaPoolConfig": gen.Gacha,
	}
	var written []string
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	for name, t := range tables {
		if t == nil {
			continue
		}
		csvPath := filepath.Join(dir, name+".csv")
		if err := t.Save(csvPath, tablekit.Options{}); err != nil {
			return written, err
		}
		written = append(written, csvPath)
		if writeXLSX {
			xlsxPath := filepath.Join(dir, name+".xlsx")
			if err := t.Save(xlsxPath, tablekit.Options{Sheet: "Config"}); err != nil {
				return written, err
			}
			written = append(written, xlsxPath)
		}
	}
	return written, nil
}

// CheckTables runs validate on generated numeric tables.
func (gen *Generated) CheckTables() []validate.Issue {
	var issues []validate.Issue
	check := func(name string, t *tablekit.Table) {
		if t == nil {
			return
		}
		rows := tableToValidate(t)
		opt := validate.Options{
			GrowthCliff: 0.6,
			MaxSafe:     2_147_483_647,
			Monotonic:   []string{"cum_exp"},
		}
		for _, is := range validate.Table(rows, opt) {
			is.Message = name + ": " + is.Message
			issues = append(issues, is)
		}
	}
	check("Hero", gen.Hero)
	check("LvUp", gen.LvUp)
	check("Enemy", gen.Enemy)
	check("Task", gen.Task)
	check("Item", gen.Item)
	return issues
}

func tableToValidate(t *tablekit.Table) []validate.Row {
	idField, _, _ := t.ResolveIDField("")
	var rows []validate.Row
	for i, r := range t.Rows {
		fields := map[string]float64{}
		var id int64
		for ci, h := range t.Headers {
			val := ""
			if ci < len(r) {
				val = r[ci]
			}
			if h == idField {
				if v, err := strconv.ParseFloat(val, 64); err == nil {
					id = int64(v)
				} else {
					id = int64(i + 1)
				}
				continue
			}
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				fields[h] = v
			}
		}
		rows = append(rows, validate.Row{ID: id, Fields: fields})
	}
	return rows
}

// ---- level curve ----

type levelRow struct {
	Level     int
	ExpToNext float64
	CumExp    float64
	StatMult  float64 // cumulative (1+gain)^ (level-1) style
}

func buildLevels(p ProgressGoal) ([]levelRow, float64, error) {
	max := p.MaxLevel
	// Build cum_exp so that at each target_days milestone, days = cum/daily_exp hits target.
	// Between milestones use piecewise-log shape (early fast, late slow).
	anchors := targetAnchors(p)
	if len(anchors) == 0 {
		anchors = []struct {
			level int
			days  float64
		}{{1, 0}, {max, 8}}
	}

	gainRate := p.StatGainPct / 100
	if gainRate <= 0 {
		gainRate = 0.12
	}

	// shape weights via piecewise-log on level
	params := curves.Params{S1: p.EarlySlope, S2: p.LateSlope, BreakX: p.BreakLevel}
	raw := make([]float64, max+1)
	for lv := 0; lv <= max; lv++ {
		y, err := curves.Evaluate(curves.PiecewiseLog, float64(lv), params)
		if err != nil {
			return nil, 0, err
		}
		raw[lv] = y
	}

	// piecewise scale between anchors
	cum := make([]float64, max+1)
	for i := 0; i+1 < len(anchors); i++ {
		a, b := anchors[i], anchors[i+1]
		daysA, daysB := a.days, b.days
		rawA, rawB := raw[a.level], raw[b.level]
		if rawB <= rawA {
			rawB = rawA + 1e-9
		}
		for lv := a.level; lv <= b.level && lv <= max; lv++ {
			t := (raw[lv] - rawA) / (rawB - rawA)
			days := daysA + t*(daysB-daysA)
			cum[lv] = days * p.DailyExp
		}
	}
	// fill before first / after last
	first, last := anchors[0], anchors[len(anchors)-1]
	for lv := 0; lv < first.level && lv <= max; lv++ {
		cum[lv] = first.days * p.DailyExp * float64(lv) / float64(maxInt(first.level, 1))
	}
	for lv := last.level + 1; lv <= max; lv++ {
		cum[lv] = last.days * p.DailyExp
	}
	// enforce non-decreasing
	for lv := 1; lv <= max; lv++ {
		if cum[lv] < cum[lv-1] {
			cum[lv] = cum[lv-1]
		}
	}

	var rows []levelRow
	stat := 1.0
	for lv := 1; lv <= max; lv++ {
		toNext := cum[lv] - cum[lv-1]
		if lv == max {
			toNext = 0
		}
		if toNext < 0 {
			toNext = 0
		}
		if lv > 1 {
			g := gainRate
			if float64(lv) > p.BreakLevel {
				g = gainRate * 0.75
			}
			if g < 0.08 {
				g = 0.08
			}
			stat *= (1 + g)
		}
		rows = append(rows, levelRow{
			Level: lv, ExpToNext: math.Round(toNext), CumExp: math.Round(cum[lv]), StatMult: stat,
		})
	}
	return rows, cum[max], nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func targetAnchors(p ProgressGoal) []struct {
	level int
	days  float64
} {
	type anchor struct {
		level int
		days  float64
	}
	var list []anchor
	list = append(list, anchor{1, 0})
	keys := make([]int, 0, len(p.TargetDays))
	for k, d := range p.TargetDays {
		if d <= 0 {
			continue
		}
		lv, err := strconv.Atoi(k)
		if err != nil || lv < 1 || lv > p.MaxLevel {
			continue
		}
		keys = append(keys, lv)
	}
	sort.Ints(keys)
	for _, lv := range keys {
		list = append(list, anchor{lv, p.TargetDays[strconv.Itoa(lv)]})
	}
	if list[len(list)-1].level < p.MaxLevel {
		last := list[len(list)-1]
		list = append(list, anchor{p.MaxLevel, last.days * float64(p.MaxLevel) / float64(maxInt(last.level, 1))})
	}
	out := make([]struct {
		level int
		days  float64
	}, len(list))
	for i, a := range list {
		out[i].level = a.level
		out[i].days = a.days
	}
	return out
}

func levelTable(rows []levelRow, p ProgressGoal) *tablekit.Table {
	t := &tablekit.Table{
		Headers: []string{"id", "level", "exp_to_next", "cum_exp", "stat_gain_pct", "note"},
	}
	gain := p.StatGainPct
	for _, r := range rows {
		g := gain
		if r.Level > int(p.BreakLevel) {
			g = gain * 0.75
		}
		if g < 8 {
			g = 8
		}
		t.Rows = append(t.Rows, []string{
			strconv.Itoa(r.Level),
			strconv.Itoa(r.Level),
			fmt.Sprintf("%.0f", r.ExpToNext),
			fmt.Sprintf("%.0f", r.CumExp),
			fmt.Sprintf("%.1f", g),
			fmt.Sprintf("mult=%.3f", r.StatMult),
		})
	}
	return t
}

// ---- combat ----

type skillRow struct {
	ID     string
	HeroID string
	Name   string
	Kind   string // basic / skill / ultimate
	Mult   float64
	Hits   int
	Energy float64
	Target string
	Desc   string
}

type enemyRow struct {
	ID    string
	Name  string
	Level int
	HP    float64
	ATK   float64
	DEF   float64
	SPD   float64
	Kind  string // mob / elite / boss
}

func statAt(base, mult float64) float64 {
	return base * mult
}

func buildCombatTables(g Goal, levels []levelRow, _ float64) ([]skillRow, []enemyRow) {
	var skills []skillRow
	var enemies []enemyRow
	sk := skillNamesForGenre(g.Genre)
	// 3 skills per archetype
	for _, a := range g.Combat.Archetypes {
		elem := a.Element
		if elem == "" {
			elem = "物理"
		}
		skills = append(skills,
			skillRow{ID: a.ID + "01", HeroID: a.ID, Name: a.Name + "-" + sk.basic, Kind: "basic",
				Mult: a.BasicMult, Hits: 1, Energy: 20, Target: "single",
				Desc: fmt.Sprintf("对敌方单体造成等同于{Attack}%d%%的%s伤害", int(a.BasicMult*100), elem)},
			skillRow{ID: a.ID + "02", HeroID: a.ID, Name: a.Name + "-" + sk.skill, Kind: "skill",
				Mult: a.SkillMult, Hits: 2, Energy: 30, Target: "single",
				Desc: fmt.Sprintf("对敌方单体造成2段，每段{Attack}%d%%的%s伤害", int(a.SkillMult*50), elem)},
			skillRow{ID: a.ID + "03", HeroID: a.ID, Name: a.Name + "-" + sk.ult, Kind: "ultimate",
				Mult: a.UltMult, Hits: 1, Energy: 0, Target: "aoe",
				Desc: fmt.Sprintf("对敌方全体造成{Attack}%d%%的%s伤害", int(a.UltMult*100), elem)},
		)
	}

	levelMult := map[int]float64{}
	for _, lv := range levels {
		levelMult[lv.Level] = lv.StatMult
	}
	sampleLevels := []int{}
	for _, s := range []int{5, 10, 15, 20, 25, 30, 35, 40} {
		if s <= g.Progress.MaxLevel {
			sampleLevels = append(sampleLevels, s)
		}
	}
	if len(sampleLevels) == 0 {
		sampleLevels = []int{g.Progress.MaxLevel}
	}

	var avgHP, avgATK, avgDEF, avgSPD float64
	for _, a := range g.Combat.Archetypes {
		avgHP += a.HP
		avgATK += a.ATK
		avgDEF += a.DEF
		avgSPD += a.SPD
	}
	n := float64(len(g.Combat.Archetypes))
	avgHP, avgATK, avgDEF, avgSPD = avgHP/n, avgATK/n, avgDEF/n, avgSPD/n

	// Auto-tune elite HP multiplier toward target win rate.
	hpMult := g.Combat.EnemyHPMult
	if hpMult <= 0 {
		hpMult = 1.15
	}
	midLevel := sampleLevels[len(sampleLevels)/2]
	multMid := levelMult[midLevel]
	if multMid == 0 {
		multMid = 1
	}
	// Tune against average DPS win rate so all output roles land near the target.
	var dps []Archetype
	for _, a := range g.Combat.Archetypes {
		if a.ATK >= avgArchetypeATK(g)*0.95 {
			dps = append(dps, a)
		}
	}
	if len(dps) == 0 {
		dps = g.Combat.Archetypes
	}
	var atkMult float64
	hpMult, atkMult = tuneEnemyForDPS(g, dps, avgHP, avgATK, avgDEF, avgSPD, multMid, hpMult)
	enemyAtk := atkMult

	idx := 2000
	for _, lv := range sampleLevels {
		mult := levelMult[lv]
		if mult == 0 {
			mult = 1
		}
		idx++
		enemies = append(enemies, enemyRow{
			ID: strconv.Itoa(idx), Name: fmt.Sprintf("%s·Lv%d", eliteNameForGenre(g.Genre), lv), Level: lv,
			HP:   math.Round(avgHP * mult * hpMult),
			ATK:  math.Round(avgATK * mult * enemyAtk),
			DEF:  math.Round(avgDEF * mult * g.Combat.EnemyDefMult),
			SPD:  math.Round(avgSPD * 0.95),
			Kind: "elite",
		})
	}
	// boss at max level — slightly above elite, still in playable band for DPS
	multMax := levelMult[g.Progress.MaxLevel]
	if multMax == 0 {
		multMax = 1
	}
	enemies = append(enemies, enemyRow{
		ID: "2901", Name: bossNameForGenre(g.Genre) + "·演示", Level: g.Progress.MaxLevel,
		HP:   math.Round(avgHP * multMax * hpMult * 1.55),
		ATK:  math.Round(avgATK * multMax * enemyAtk * 1.05),
		DEF:  math.Round(avgDEF * multMax * g.Combat.EnemyDefMult),
		SPD:  math.Round(avgSPD),
		Kind: "boss",
	})
	return skills, enemies
}

// tuneEnemyForDPS searches elite HP/ATK so the average DPS win rate ≈ target.
func tuneEnemyForDPS(g Goal, dps []Archetype, avgHP, avgATK, avgDEF, avgSPD, statMult, startMult float64) (float64, float64) {
	atkMult := g.Combat.EnemyAtkMult
	defMult := g.Combat.EnemyDefMult

	simDPS := func(hpM, atkM float64) float64 {
		var sum float64
		var n float64
		for i, a := range dps {
			hero := combat.Unit{
				Name: a.Name,
				HP:   a.HP * statMult, Attack: a.ATK * statMult, Defense: a.DEF * statMult,
				CritRate: a.CritRate, CritMult: 1 + a.CritDMG,
				Speed: a.SPD / 100, SkillCoeff: a.SkillMult,
			}
			enemy := combat.Unit{
				Name: "elite",
				HP:   avgHP * statMult * hpM, Attack: avgATK * statMult * atkM,
				Defense:  avgDEF * statMult * defMult,
				CritRate: 0.05, CritMult: 1.5, Speed: avgSPD * 0.95 / 100, SkillCoeff: 1,
			}
			res, err := combat.Simulate(hero, enemy, 300, rand.New(rand.NewSource(int64(11+i))))
			if err != nil {
				continue
			}
			sum += res.AttackerWinRate
			n++
		}
		if n == 0 {
			return 0
		}
		return sum / n
	}

	// Soften ATK if DPS cannot win even vs low-HP elite.
	for i := 0; i < 8; i++ {
		if simDPS(0.5, atkMult) >= 0.25 {
			break
		}
		atkMult *= 0.8
		if atkMult < 0.12 {
			atkMult = 0.12
			break
		}
	}

	lo, hi := 0.5, 6.0
	best := startMult
	for i := 0; i < 12; i++ {
		mid := (lo + hi) / 2
		best = mid
		wr := simDPS(mid, atkMult)
		if wr > g.Combat.TargetWinRate+0.04 {
			lo = mid
		} else if wr < g.Combat.TargetWinRate-0.04 {
			hi = mid
		} else {
			break
		}
	}
	return math.Round(best*100) / 100, atkMult
}

func runCombatChecks(g Goal, skills []skillRow, enemies []enemyRow, levels []levelRow) []CombatCheck {
	skillByHero := map[string]map[string]skillRow{}
	for _, s := range skills {
		if skillByHero[s.HeroID] == nil {
			skillByHero[s.HeroID] = map[string]skillRow{}
		}
		skillByHero[s.HeroID][s.Kind] = s
	}
	levelMult := map[int]float64{}
	for _, lv := range levels {
		levelMult[lv.Level] = lv.StatMult
	}

	var checks []CombatCheck
	midLevel := g.Progress.MaxLevel / 2
	var midEnemy, bossEnemy *enemyRow
	for i := range enemies {
		if enemies[i].Kind == "elite" {
			if midEnemy == nil || absf(float64(enemies[i].Level-midLevel)) < absf(float64(midEnemy.Level-midLevel)) {
				midEnemy = &enemies[i]
			}
		}
		if enemies[i].Kind == "boss" {
			bossEnemy = &enemies[i]
		}
	}

	rng := rand.New(rand.NewSource(42))
	for _, a := range g.Combat.Archetypes {
		sk := skillByHero[a.ID]
		coeff := sk["skill"].Mult
		if coeff == 0 {
			coeff = a.SkillMult
		}

		type tgt struct {
			label string
			e     *enemyRow
		}
		var targets []tgt
		if midEnemy != nil {
			targets = append(targets, tgt{"精英", midEnemy})
		}
		if bossEnemy != nil {
			targets = append(targets, tgt{"Boss", bossEnemy})
		}

		for _, tg := range targets {
			// SAME-LEVEL hero vs enemy
			mult := levelMult[tg.e.Level]
			if mult == 0 {
				mult = 1
			}
			hero := combat.Unit{
				Name: a.Name,
				HP:   a.HP * mult, Attack: a.ATK * mult, Defense: a.DEF * mult,
				CritRate: a.CritRate, CritMult: 1 + a.CritDMG,
				Speed: a.SPD / 100, SkillCoeff: coeff,
			}
			enemy := combat.Unit{
				Name: tg.e.Name, HP: tg.e.HP, Attack: tg.e.ATK, Defense: tg.e.DEF,
				CritRate: 0.05, CritMult: 1.5, Speed: tg.e.SPD / 100, SkillCoeff: 1,
			}
			res, err := combat.Simulate(hero, enemy, 600, rng)
			if err != nil {
				continue
			}
			ok := true
			note := "ok"
			isDPS := a.ATK >= avgArchetypeATK(g)*0.95
			if tg.e.Kind == "elite" {
				if isDPS {
					if res.AttackerWinRate < g.Combat.TargetWinRate-0.18 || res.AttackerWinRate > g.Combat.TargetWinRate+0.25 {
						ok = false
						note = fmt.Sprintf("输出位胜率偏离目标 %.0f%%", g.Combat.TargetWinRate*100)
					}
				} else {
					// tank/support: require survivability, allow lower win
					if res.AvgTurns < 4 {
						ok = false
						note = "生存位过脆，站不住"
					}
				}
				if res.AvgTurns > g.Combat.TargetTurns*2.2 {
					ok = false
					note = "对局过长"
				}
			} else {
				if isDPS {
					if res.AttackerWinRate < 0.1 || res.AttackerWinRate > 0.75 {
						ok = false
						note = "Boss 胜率超出可玩带 10%~75%"
					}
				} else if res.AvgTurns < 5 {
					ok = false
					note = "Boss 战生存位站不住"
				}
			}
			checks = append(checks, CombatCheck{
				HeroID: a.ID, HeroName: a.Name,
				VsEnemy:  fmt.Sprintf("%s/Lv%d/%s", tg.label, tg.e.Level, tg.e.Name),
				WinRate:  res.AttackerWinRate,
				AvgTurns: res.AvgTurns, OK: ok, Note: note,
			})
		}
	}
	return checks
}

func absf(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func avgArchetypeATK(g Goal) float64 {
	if len(g.Combat.Archetypes) == 0 {
		return 0
	}
	var s float64
	for _, a := range g.Combat.Archetypes {
		s += a.ATK
	}
	return s / float64(len(g.Combat.Archetypes))
}

// isDPSWeapon marks primary damage roles across genres.
func isDPSWeapon(w string) bool {
	switch w {
	// genshin weapons
	case "单手剑", "双手剑", "长柄武器", "弓":
		return true
	// slg troops
	case "骑兵", "攻城", "混合":
		return true
	// onmyoji roles
	case "输出":
		return true
	default:
		return false
	}
}

func heroTable(g Goal, levels []levelRow) *tablekit.Table {
	t := &tablekit.Table{
		Headers: []string{
			"id", "name", "weapon", "element", "rarity",
			"base_hp", "base_atk", "base_def", "base_spd",
			"base_crit_rate", "base_crit_dmg",
			"skill_basic", "skill_skill", "skill_ultimate",
			"note",
		},
	}
	for _, a := range g.Combat.Archetypes {
		t.Rows = append(t.Rows, []string{
			a.ID, a.Name, a.Weapon, a.Element, strconv.Itoa(a.Rarity),
			fmt.Sprintf("%.0f", a.HP), fmt.Sprintf("%.0f", a.ATK),
			fmt.Sprintf("%.0f", a.DEF), fmt.Sprintf("%.0f", a.SPD),
			fmt.Sprintf("%.0f", a.CritRate*100), fmt.Sprintf("%.0f", a.CritDMG*100),
			a.ID + "01", a.ID + "02", a.ID + "03",
			"demo-sanitized",
		})
	}
	return t
}

func skillTable(skills []skillRow) *tablekit.Table {
	t := &tablekit.Table{
		Headers: []string{
			"id", "hero_id", "name", "type", "target", "hits",
			"atk_mult_pct", "energy_gain", "desc",
		},
	}
	for _, s := range skills {
		t.Rows = append(t.Rows, []string{
			s.ID, s.HeroID, s.Name, s.Kind, s.Target, strconv.Itoa(s.Hits),
			fmt.Sprintf("%.0f", s.Mult*100), fmt.Sprintf("%.0f", s.Energy), s.Desc,
		})
	}
	return t
}

func enemyTable(enemies []enemyRow) *tablekit.Table {
	t := &tablekit.Table{
		Headers: []string{"id", "name", "level", "kind", "hp", "atk", "def", "spd"},
	}
	for _, e := range enemies {
		t.Rows = append(t.Rows, []string{
			e.ID, e.Name, strconv.Itoa(e.Level), e.Kind,
			fmt.Sprintf("%.0f", e.HP), fmt.Sprintf("%.0f", e.ATK),
			fmt.Sprintf("%.0f", e.DEF), fmt.Sprintf("%.0f", e.SPD),
		})
	}
	return t
}

func taskTable(g Goal, totalExp float64) *tablekit.Table {
	t := &tablekit.Table{
		Headers: []string{
			"id", "name", "type", "unlock_level", "target_type", "target_count",
			"reward_exp", "reward_gold", "note",
		},
	}
	// main tasks: divide cum exp (minus daily share approx)
	sideShare := g.Tasks.SideShare
	if sideShare < 0 {
		sideShare = 0
	}
	if sideShare > 0.4 {
		sideShare = 0.4
	}
	mainBudget := totalExp * (1 - sideShare)
	perMain := mainBudget / float64(g.Tasks.MainCount)

	targetTypes := []string{"clear_stage", "upgrade_hero", "win_battle", "collect_item", "gacha_once"}
	for i := 0; i < g.Tasks.MainCount; i++ {
		id := 30000 + i + 1
		unlock := 1 + (i * g.Progress.MaxLevel / g.Tasks.MainCount)
		if unlock > g.Progress.MaxLevel {
			unlock = g.Progress.MaxLevel
		}
		t.Rows = append(t.Rows, []string{
			strconv.Itoa(id),
			fmt.Sprintf("主线·第%d章", i+1),
			"main",
			strconv.Itoa(unlock),
			targetTypes[i%len(targetTypes)],
			strconv.Itoa(1 + i%3),
			fmt.Sprintf("%.0f", perMain),
			fmt.Sprintf("%.0f", perMain*0.8),
			"generated",
		})
	}
	// daily tasks
	for i := 0; i < g.Tasks.DailyCount; i++ {
		id := 31000 + i + 1
		t.Rows = append(t.Rows, []string{
			strconv.Itoa(id),
			fmt.Sprintf("每日·活跃%d", i+1),
			"daily",
			"1",
			[]string{"login", "win_battle", "clear_stage", "consume_stamina"}[i%4],
			strconv.Itoa(1 + i),
			fmt.Sprintf("%.0f", g.Tasks.DailyExp),
			fmt.Sprintf("%.0f", g.Tasks.DailyExp*0.5),
			"generated",
		})
	}
	// side tasks
	sideCount := g.Tasks.MainCount / 3
	sideBudget := totalExp * sideShare
	perSide := 0.0
	if sideCount > 0 {
		perSide = sideBudget / float64(sideCount)
	}
	for i := 0; i < sideCount; i++ {
		id := 32000 + i + 1
		t.Rows = append(t.Rows, []string{
			strconv.Itoa(id),
			fmt.Sprintf("支线·探索%d", i+1),
			"side",
			strconv.Itoa(1 + i*2),
			"explore",
			strconv.Itoa(1),
			fmt.Sprintf("%.0f", perSide),
			fmt.Sprintf("%.0f", perSide*0.6),
			"generated",
		})
	}
	return t
}

func itemTable(g Goal) *tablekit.Table {
	t := &tablekit.Table{
		Headers: []string{
			"id", "name", "type", "quality", "stack", "sell_gold", "desc",
		},
	}
	for _, it := range itemsForGenre(g.Genre) {
		t.Rows = append(t.Rows, []string{it[0], it[1], it[2], it[3], it[4], it[5], it[6]})
	}
	// gold sink preview item price scaled to economy
	t.Rows = append(t.Rows, []string{
		"5001", "升级消耗·演示", "sink", "1", "1",
		fmt.Sprintf("%.0f", g.Economy.GoldCostMax/float64(g.Progress.MaxLevel)),
		"满级累计消耗锚点",
	})
	return t
}

type skillFlavor struct{ basic, skill, ult string }

func skillNamesForGenre(genre string) skillFlavor {
	switch genre {
	case "slg":
		return skillFlavor{"普攻", "战术指令", "统率技"}
	case "onmyoji":
		return skillFlavor{"普攻", "主动技能", "大招"}
	default:
		return skillFlavor{"普通攻击", "元素战技", "元素爆发"}
	}
}

func eliteNameForGenre(genre string) string {
	switch genre {
	case "slg":
		return "雪原掠夺者"
	case "onmyoji":
		return "觉醒妖灵"
	default:
		return "遗迹机兵"
	}
}

func bossNameForGenre(genre string) string {
	switch genre {
	case "slg":
		return "冰原巨兽"
	case "onmyoji":
		return "八岐幻影"
	default:
		return "风蚀之核"
	}
}

func itemsForGenre(genre string) [][7]string {
	switch genre {
	case "slg":
		return [][7]string{
			{"1001", "生肉", "currency", "1", "999999", "0", "基础资源"},
			{"1002", "木材", "currency", "1", "999999", "0", "基础资源"},
			{"1003", "钻石", "currency", "5", "999999", "0", "稀有货币（演示）"},
			{"2001", "统率经验", "exp", "2", "999999", "0", "建筑/统率经验"},
			{"2002", "加速券·5分", "material", "3", "999", "0", "建造/研究加速"},
			{"3001", "英雄招募券", "gacha_ticket", "4", "999", "0", "英雄抽取（演示）"},
			{"4001", "英雄碎片", "hero_shard", "4", "999", "0", "英雄升星材料"},
		}
	case "onmyoji":
		return [][7]string{
			{"1001", "金币", "currency", "1", "999999", "0", "基础货币"},
			{"1002", "勾玉", "currency", "5", "999999", "0", "稀有货币（演示）"},
			{"2001", "式神经验", "exp", "2", "999999", "0", "升级材料"},
			{"2002", "觉醒材料", "material", "3", "999", "100", "式神觉醒（演示）"},
			{"2003", "御魂强化石", "material", "3", "999", "80", "御魂强化（演示）"},
			{"3001", "神秘的符咒", "gacha_ticket", "5", "999", "0", "召唤券（演示）"},
			{"4001", "式神碎片", "hero_shard", "4", "999", "0", "合成式神（演示）"},
		}
	default: // genshin
		return [][7]string{
			{"1001", "摩拉", "currency", "1", "999999", "0", "基础货币"},
			{"1002", "原石", "currency", "5", "999999", "0", "稀有货币（演示）"},
			{"2001", "冒险阅历", "exp", "2", "999999", "0", "冒险等级经验（演示）"},
			{"2002", "大英雄的经验", "material", "3", "9999", "0", "角色经验书（演示）"},
			{"2003", "武器突破矿石", "material", "3", "999", "50", "武器培养材料（演示）"},
			{"3001", "相遇之缘", "gacha_ticket", "4", "999", "0", "常驻祈愿券（演示）"},
			{"4001", "命星·演示", "hero_shard", "5", "999", "0", "角色命星（演示）"},
		}
	}
}

func gachaTable(g Goal, softStart int, softStep float64) *tablekit.Table {
	t := &tablekit.Table{
		Headers: []string{
			"id", "name", "rarity", "base_rate_pct", "soft_pity_start", "soft_pity_step_pct",
			"hard_pity", "featured_rate_pct", "guarantee_featured", "note",
		},
	}
	// 5-star
	t.Rows = append(t.Rows, []string{
		"1", "角色活动祈愿", "5",
		fmt.Sprintf("%.3f", g.Gacha.BaseRate5*100),
		strconv.Itoa(softStart),
		fmt.Sprintf("%.2f", softStep*100),
		strconv.Itoa(g.Gacha.HardPity),
		fmt.Sprintf("%.1f", g.Gacha.FeaturedRate*100),
		strconv.FormatBool(g.Gacha.Guarantee),
		"sanitized demo",
	})
	// 4-star simple
	t.Rows = append(t.Rows, []string{
		"2", "4星保底", "4",
		fmt.Sprintf("%.3f", g.Gacha.BaseRate4*100),
		"0", "0",
		strconv.Itoa(g.Gacha.HardPity4),
		"0", "false",
		"4星硬保底",
	})
	// 3-star filler
	t.Rows = append(t.Rows, []string{
		"3", "3星光锥", "3",
		fmt.Sprintf("%.2f", (1-g.Gacha.BaseRate5-g.Gacha.BaseRate4)*100),
		"0", "0", "0", "0", "false", "filler",
	})
	return t
}
