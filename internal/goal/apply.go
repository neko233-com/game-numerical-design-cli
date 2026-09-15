package goal

import (
	"fmt"
	"strconv"
	"strings"
)

// ApplyOptions is the production one-shot config request (配新 + 改旧).
type ApplyOptions struct {
	// Preset: genshin | slg | onmyoji
	Preset string `json:"preset"`
	// FromGoal loads an existing goal file as base (改旧).
	FromGoal string `json:"from_goal"`
	// Name overrides goal name.
	Name string `json:"name"`
	// OneLine is "k=v k=v" overrides (also used by apply CLI).
	OneLine string `json:"one_line"`
	// Structured overrides
	MaxLevel    int     `json:"max_level,omitempty"`
	DailyExp    float64 `json:"daily_exp,omitempty"`
	WinRate     float64 `json:"win_rate,omitempty"`
	TargetTurns float64 `json:"target_turns,omitempty"`
	HardPity    int     `json:"hard_pity,omitempty"`
	BaseRate5   float64 `json:"base_rate_5,omitempty"`
	StatGainPct float64 `json:"stat_gain_pct,omitempty"`
	// Extra raw k=v from OneLine after structured parse.
	KV map[string]string `json:"kv,omitempty"`
}

// ApplyResult is the outcome of ResolveApply + Build.
type ApplyResult struct {
	Goal      Goal       `json:"goal"`
	Generated *Generated `json:"-"`
	// Changed lists override keys applied to base.
	Changed []string `json:"changed"`
	// Sources of goal
	Source string `json:"source"` // preset | goal_file | mixed
}

// ResolveApply builds the final Goal from preset/file/overrides (no IO except FromGoal).
func ResolveApply(opt ApplyOptions) (*ApplyResult, error) {
	base := Preset(ParsePreset(opt.Preset))
	source := "preset"
	if strings.TrimSpace(opt.FromGoal) != "" {
		loaded, err := LoadGoal(opt.FromGoal)
		if err != nil {
			return nil, fmt.Errorf("load goal: %w", err)
		}
		base = loaded
		source = "goal_file"
		if opt.Preset != "" && ParsePreset(opt.Preset) != PresetGenshinRPG {
			source = "mixed"
		}
	}

	changed := []string{}
	set := func(key string, fn func()) {
		changed = append(changed, key)
		fn()
	}

	if strings.TrimSpace(opt.Name) != "" {
		set("name", func() { base.Name = opt.Name })
	}

	// parse one-line k=v pairs
	kv := map[string]string{}
	for _, pair := range strings.Fields(opt.OneLine) {
		if i := strings.IndexByte(pair, '='); i > 0 {
			kv[strings.ToLower(strings.TrimSpace(pair[:i]))] = strings.TrimSpace(pair[i+1:])
		}
	}
	for k, v := range opt.KV {
		kv[strings.ToLower(k)] = v
	}

	// one-line may switch preset when not loading from goal file
	if opt.FromGoal == "" {
		if p, ok := kv["preset"]; ok && strings.TrimSpace(p) != "" {
			base = Preset(ParsePreset(p))
			source = "preset"
			changed = append(changed, "preset")
		}
	}
	delete(kv, "preset")

	// structured flags win over one-line if set
	applyInt := func(key string, dst *int, v int) {
		if v != 0 {
			set(key, func() { *dst = v })
			return
		}
		if s, ok := kv[key]; ok {
			if n, err := strconv.Atoi(s); err == nil {
				set(key, func() { *dst = n })
			}
		}
	}
	applyFloat := func(key string, dst *float64, v float64) {
		if v != 0 {
			set(key, func() { *dst = v })
			return
		}
		if s, ok := kv[key]; ok {
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				set(key, func() { *dst = f })
			}
		}
	}

	applyInt("max_level", &base.Progress.MaxLevel, opt.MaxLevel)
	applyFloat("daily_exp", &base.Progress.DailyExp, opt.DailyExp)
	applyFloat("win_rate", &base.Combat.TargetWinRate, opt.WinRate)
	applyFloat("target_turns", &base.Combat.TargetTurns, opt.TargetTurns)
	applyInt("hard_pity", &base.Gacha.HardPity, opt.HardPity)
	applyFloat("base_rate_5", &base.Gacha.BaseRate5, opt.BaseRate5)
	applyFloat("stat_gain_pct", &base.Progress.StatGainPct, opt.StatGainPct)

	// extra numeric one-liners mapped to known goal fields
	for k, s := range kv {
		switch k {
		case "max_level", "daily_exp", "win_rate", "target_turns", "hard_pity", "base_rate_5", "stat_gain_pct":
			continue // already handled
		case "early_slope":
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				set(k, func() { base.Progress.EarlySlope = f })
			}
		case "late_slope":
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				set(k, func() { base.Progress.LateSlope = f })
			}
		case "break_level":
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				set(k, func() { base.Progress.BreakLevel = f })
			}
		case "enemy_hp_mult":
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				set(k, func() { base.Combat.EnemyHPMult = f })
			}
		case "enemy_atk_mult":
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				set(k, func() { base.Combat.EnemyAtkMult = f })
			}
		case "soft_start_ratio":
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				set(k, func() { base.Gacha.SoftStartRatio = f })
			}
		case "featured_rate":
			if f, err := strconv.ParseFloat(s, 64); err == nil {
				set(k, func() { base.Gacha.FeaturedRate = f })
			}
		}
	}

	if err := base.normalize(); err != nil {
		return nil, err
	}
	gen, err := Build(base)
	if err != nil {
		return nil, err
	}
	return &ApplyResult{Goal: base, Generated: gen, Changed: changed, Source: source}, nil
}

// ApplyCheck is the production self-check summary.
type ApplyCheck struct {
	OK            bool     `json:"ok"`
	Warnings      []string `json:"warnings"`
	ValidateErrs  int      `json:"validate_errors"`
	ValidateWarn  int      `json:"validate_warns"`
	CombatBad     int      `json:"combat_bad"`
	PaceBad       int      `json:"pace_bad"`
	HeroRows      int      `json:"hero_rows"`
	SkillRows     int      `json:"skill_rows"`
	TaskRows      int      `json:"task_rows"`
	EnemyRows     int      `json:"enemy_rows"`
	MaxLevel      int      `json:"max_level"`
	WinRateTarget float64  `json:"win_rate_target"`
	HardPity      int      `json:"hard_pity"`
}

// SelfCheck runs production gates on a generated result.
func (r *ApplyResult) SelfCheck() ApplyCheck {
	g := r.Generated
	ch := ApplyCheck{
		MaxLevel:      r.Goal.Progress.MaxLevel,
		WinRateTarget: r.Goal.Combat.TargetWinRate,
		HardPity:      r.Goal.Gacha.HardPity,
		HeroRows:      len(g.Hero.Rows),
		SkillRows:     len(g.Skill.Rows),
		TaskRows:      len(g.Task.Rows),
		EnemyRows:     len(g.Enemy.Rows),
	}
	for _, is := range g.CheckTables() {
		switch is.Severity {
		case "error":
			ch.ValidateErrs++
			ch.Warnings = append(ch.Warnings, "validate: "+is.Message)
		case "warn":
			ch.ValidateWarn++
		}
	}
	for _, c := range g.Report.CombatChecks {
		if !c.OK {
			ch.CombatBad++
		}
	}
	for _, p := range g.Report.Pace {
		if p.TargetDays > 0 && (p.DeltaRatio > 0.4 || p.DeltaRatio < -0.5) {
			ch.PaceBad++
		}
	}
	// production hard gates
	ch.Warnings = append(ch.Warnings, g.Report.Warnings...)
	if ch.HeroRows < 1 {
		ch.Warnings = append(ch.Warnings, "无英雄配置")
	}
	if ch.SkillRows < ch.HeroRows*2 {
		ch.Warnings = append(ch.Warnings, "技能行数不足")
	}
	if r.Goal.Gacha.HardPity <= 0 {
		ch.Warnings = append(ch.Warnings, "硬保底未设置")
	}
	if r.Goal.Progress.MaxLevel < 10 {
		ch.Warnings = append(ch.Warnings, "max_level 过低")
	}
	// OK if no validate errors and table shape ok.
	// Combat/pace warnings are soft (design iteration), listed but not blocking.
	ch.OK = ch.ValidateErrs == 0 && ch.HeroRows > 0 && ch.SkillRows > 0
	return ch
}

// ParseApplyLine is a convenience for CLI one-liner.
func ParseApplyLine(s string) map[string]string {
	out := map[string]string{}
	for _, pair := range strings.Fields(s) {
		if i := strings.IndexByte(pair, '='); i > 0 {
			out[strings.ToLower(strings.TrimSpace(pair[:i]))] = strings.TrimSpace(pair[i+1:])
		}
	}
	return out
}
