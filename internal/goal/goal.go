// Package goal turns numerical design goals into concrete config tables.
//
// Flow: Goal (YAML/JSON) → generate tables → optional sim/check → write csv/xlsx.
package goal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/neko233-com/game-numerical-design-cli/internal/economy"
)

// Goal is the high-level design intent. All numbers are design targets,
// not engine units — the generator expands them into rows.
type Goal struct {
	Name        string `json:"name" yaml:"name"`
	Description string `json:"description" yaml:"description"`
	// Version of goal schema.
	Version int `json:"version" yaml:"version"`

	Progress ProgressGoal `json:"progress" yaml:"progress"`
	Combat   CombatGoal   `json:"combat" yaml:"combat"`
	Tasks    TaskGoal     `json:"tasks" yaml:"tasks"`
	Gacha    GachaGoal    `json:"gacha" yaml:"gacha"`
	Economy  EconomyGoal  `json:"economy" yaml:"economy"`
}

type ProgressGoal struct {
	MaxLevel int     `json:"max_level" yaml:"max_level"`
	DailyExp float64 `json:"daily_exp" yaml:"daily_exp"`
	// TargetDaysToN maps level → design days from day0 (e.g. {10:1, 20:2.5, 30:4}).
	TargetDays map[string]float64 `json:"target_days" yaml:"target_days"`
	EarlySlope float64            `json:"early_slope" yaml:"early_slope"`
	LateSlope  float64            `json:"late_slope" yaml:"late_slope"`
	BreakLevel float64            `json:"break_level" yaml:"break_level"`
	// StatGainPct: key attribute gain % per level (feel threshold, default 8~20).
	StatGainPct float64 `json:"stat_gain_pct" yaml:"stat_gain_pct"`
}

type CombatGoal struct {
	// Target win rate of P50 hero vs same-level elite (0.4~0.6 healthy).
	TargetWinRate float64 `json:"target_win_rate" yaml:"target_win_rate"`
	// Battle length target in turns (actions).
	TargetTurns float64 `json:"target_turns" yaml:"target_turns"`
	// Archetypes define role stats at level 1 (base) and growth via Progress.
	Archetypes []Archetype `json:"archetypes" yaml:"archetypes"`
	// Enemy multiplier vs average hero HP at same level.
	EnemyHPMult  float64 `json:"enemy_hp_mult" yaml:"enemy_hp_mult"`
	EnemyAtkMult float64 `json:"enemy_atk_mult" yaml:"enemy_atk_mult"`
	EnemyDefMult float64 `json:"enemy_def_mult" yaml:"enemy_def_mult"`
}

// Archetype is a path/role (崩铁命途 simplified).
type Archetype struct {
	ID       string  `json:"id" yaml:"id"`
	Name     string  `json:"name" yaml:"name"`
	Path     string  `json:"path" yaml:"path"`       // 毁灭/巡猎/智识/同谐/虚无/存护/丰饶
	Element  string  `json:"element" yaml:"element"` // 物理/火/冰/雷/风/量子/虚数
	Rarity   int     `json:"rarity" yaml:"rarity"`   // 4 or 5
	HP       float64 `json:"hp" yaml:"hp"`
	ATK      float64 `json:"atk" yaml:"atk"`
	DEF      float64 `json:"def" yaml:"def"`
	SPD      float64 `json:"spd" yaml:"spd"`
	CritRate float64 `json:"crit_rate" yaml:"crit_rate"` // 0..1
	CritDMG  float64 `json:"crit_dmg" yaml:"crit_dmg"`   // 0.5 = 50%
	// Skill multipliers (coefficient on ATK).
	BasicMult float64 `json:"basic_mult" yaml:"basic_mult"`
	SkillMult float64 `json:"skill_mult" yaml:"skill_mult"`
	UltMult   float64 `json:"ult_mult" yaml:"ult_mult"`
}

type TaskGoal struct {
	// Daily tasks count and avg exp each.
	DailyCount int     `json:"daily_count" yaml:"daily_count"`
	DailyExp   float64 `json:"daily_exp" yaml:"daily_exp"`
	// Main-line tasks up to Progress.MaxLevel.
	MainCount int `json:"main_count" yaml:"main_count"`
	// Side tasks budget share of total exp (0~0.4).
	SideShare float64 `json:"side_share" yaml:"side_share"`
}

type GachaGoal struct {
	BaseRate5      float64 `json:"base_rate_5" yaml:"base_rate_5"`
	HardPity       int     `json:"hard_pity" yaml:"hard_pity"`
	SoftStartRatio float64 `json:"soft_start_ratio" yaml:"soft_start_ratio"`
	FeaturedRate   float64 `json:"featured_rate" yaml:"featured_rate"`
	Guarantee      bool    `json:"guarantee" yaml:"guarantee"`
	// 4-star
	BaseRate4 float64 `json:"base_rate_4" yaml:"base_rate_4"`
	HardPity4 int     `json:"hard_pity_4" yaml:"hard_pity_4"`
}

type EconomyGoal struct {
	// Currency daily prod/cons ratio band.
	ProdConsMin float64 `json:"prod_cons_min" yaml:"prod_cons_min"`
	ProdConsMax float64 `json:"prod_cons_max" yaml:"prod_cons_max"`
	// Starter currency stock for day-1 players.
	StarterGold float64 `json:"starter_gold" yaml:"starter_gold"`
	// Level-up gold cost at max level cumulative.
	GoldCostMax float64 `json:"gold_cost_max" yaml:"gold_cost_max"`
}

// DefaultGoal is a sensible 崩铁-like demo starting point (sanitized, fictional).
func DefaultGoal() Goal {
	return Goal{
		Name:        "星轨数值Demo",
		Description: "简化崩铁式：命途/属性/等级/技能/任务/抽卡，用于数值验证",
		Version:     1,
		Progress: ProgressGoal{
			MaxLevel:    40,
			DailyExp:    1200,
			TargetDays:  map[string]float64{"10": 1.0, "20": 2.2, "30": 4.0, "40": 8.0},
			EarlySlope:  1.8,
			LateSlope:   0.6,
			BreakLevel:  20,
			StatGainPct: 12,
		},
		Combat: CombatGoal{
			TargetWinRate: 0.55,
			TargetTurns:   8,
			EnemyHPMult:   1.15,
			EnemyAtkMult:  0.55,
			EnemyDefMult:  0.9,
			Archetypes: []Archetype{
				{ID: "1001", Name: "开拓者·毁灭", Path: "毁灭", Element: "物理", Rarity: 5,
					HP: 1200, ATK: 580, DEF: 460, SPD: 100,
					CritRate: 0.05, CritDMG: 0.50,
					BasicMult: 1.0, SkillMult: 1.4, UltMult: 2.8},
				{ID: "1002", Name: "巡星·猎手", Path: "巡猎", Element: "风", Rarity: 5,
					HP: 980, ATK: 720, DEF: 360, SPD: 115,
					CritRate: 0.08, CritDMG: 0.55,
					BasicMult: 1.1, SkillMult: 1.8, UltMult: 3.2},
				{ID: "1003", Name: "智识·星火", Path: "智识", Element: "火", Rarity: 5,
					HP: 1020, ATK: 700, DEF: 380, SPD: 96,
					CritRate: 0.05, CritDMG: 0.50,
					BasicMult: 0.9, SkillMult: 1.6, UltMult: 3.5},
				{ID: "1004", Name: "存护·壁垒", Path: "存护", Element: "冰", Rarity: 4,
					HP: 1400, ATK: 420, DEF: 620, SPD: 90,
					CritRate: 0.05, CritDMG: 0.50,
					BasicMult: 0.9, SkillMult: 1.1, UltMult: 1.8},
				{ID: "1005", Name: "丰饶·清泉", Path: "丰饶", Element: "雷", Rarity: 4,
					HP: 1100, ATK: 480, DEF: 480, SPD: 98,
					CritRate: 0.05, CritDMG: 0.50,
					BasicMult: 0.9, SkillMult: 1.0, UltMult: 1.5},
				{ID: "1006", Name: "同谐·回响", Path: "同谐", Element: "量子", Rarity: 4,
					HP: 1050, ATK: 500, DEF: 450, SPD: 105,
					CritRate: 0.05, CritDMG: 0.50,
					BasicMult: 0.9, SkillMult: 1.2, UltMult: 1.6},
			},
		},
		Tasks: TaskGoal{
			DailyCount: 4,
			DailyExp:   250,
			MainCount:  24,
			SideShare:  0.2,
		},
		Gacha: GachaGoal{
			BaseRate5: 0.006, HardPity: 90, SoftStartRatio: 0.65,
			FeaturedRate: 0.5, Guarantee: true,
			BaseRate4: 0.051, HardPity4: 10,
		},
		Economy: EconomyGoal{
			ProdConsMin: 0.9, ProdConsMax: 1.15,
			StarterGold: 5000, GoldCostMax: 120000,
		},
	}
}

// LoadGoal reads YAML or JSON goal file.
func LoadGoal(path string) (Goal, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Goal{}, err
	}
	g := DefaultGoal()
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &g); err != nil {
			return Goal{}, fmt.Errorf("parse goal yaml: %w", err)
		}
	case ".json":
		if err := json.Unmarshal(data, &g); err != nil {
			return Goal{}, fmt.Errorf("parse goal json: %w", err)
		}
	default:
		// try yaml then json
		if err := yaml.Unmarshal(data, &g); err != nil {
			if err2 := json.Unmarshal(data, &g); err2 != nil {
				return Goal{}, fmt.Errorf("parse goal: yaml=%v json=%v", err, err2)
			}
		}
	}
	if err := g.normalize(); err != nil {
		return Goal{}, err
	}
	return g, nil
}

func (g *Goal) normalize() error {
	if g.Version == 0 {
		g.Version = 1
	}
	if g.Progress.MaxLevel <= 0 {
		g.Progress.MaxLevel = 40
	}
	if g.Progress.DailyExp <= 0 {
		g.Progress.DailyExp = 1200
	}
	if g.Progress.EarlySlope <= 0 {
		g.Progress.EarlySlope = 1.8
	}
	if g.Progress.LateSlope <= 0 {
		g.Progress.LateSlope = 0.6
	}
	if g.Progress.BreakLevel <= 0 {
		g.Progress.BreakLevel = float64(g.Progress.MaxLevel) * 0.5
	}
	if g.Progress.StatGainPct <= 0 {
		g.Progress.StatGainPct = 12
	}
	if g.Combat.TargetWinRate <= 0 {
		g.Combat.TargetWinRate = 0.55
	}
	if g.Combat.TargetTurns <= 0 {
		g.Combat.TargetTurns = 8
	}
	if g.Combat.EnemyHPMult <= 0 {
		g.Combat.EnemyHPMult = 1.15
	}
	if g.Combat.EnemyAtkMult <= 0 {
		g.Combat.EnemyAtkMult = 0.55
	}
	if g.Combat.EnemyDefMult <= 0 {
		g.Combat.EnemyDefMult = 0.9
	}
	if len(g.Combat.Archetypes) == 0 {
		g.Combat = DefaultGoal().Combat
	}
	for i := range g.Combat.Archetypes {
		a := &g.Combat.Archetypes[i]
		if a.BasicMult <= 0 {
			a.BasicMult = 1
		}
		if a.SkillMult <= 0 {
			a.SkillMult = 1.4
		}
		if a.UltMult <= 0 {
			a.UltMult = 2.8
		}
		if a.SPD <= 0 {
			a.SPD = 100
		}
	}
	if g.Tasks.DailyCount <= 0 {
		g.Tasks.DailyCount = 4
	}
	if g.Tasks.DailyExp <= 0 {
		g.Tasks.DailyExp = 250
	}
	if g.Tasks.MainCount <= 0 {
		g.Tasks.MainCount = 24
	}
	if g.Gacha.BaseRate5 <= 0 {
		g.Gacha.BaseRate5 = 0.006
	}
	if g.Gacha.HardPity <= 0 {
		g.Gacha.HardPity = 90
	}
	if g.Gacha.SoftStartRatio <= 0 {
		g.Gacha.SoftStartRatio = 0.65
	}
	if g.Gacha.BaseRate4 <= 0 {
		g.Gacha.BaseRate4 = 0.051
	}
	if g.Gacha.HardPity4 <= 0 {
		g.Gacha.HardPity4 = 10
	}
	if g.Economy.ProdConsMax <= 0 {
		g.Economy.ProdConsMax = 1.15
	}
	if g.Economy.ProdConsMin <= 0 {
		g.Economy.ProdConsMin = 0.9
	}
	if g.Economy.StarterGold <= 0 {
		g.Economy.StarterGold = 5000
	}
	if g.Economy.GoldCostMax <= 0 {
		g.Economy.GoldCostMax = 120000
	}
	return nil
}

// Report summarizes what was generated and any goal-vs-result gaps.
type Report struct {
	GoalName     string               `json:"goal_name"`
	MaxLevel     int                  `json:"max_level"`
	HeroCount    int                  `json:"hero_count"`
	SkillCount   int                  `json:"skill_count"`
	TaskCount    int                  `json:"task_count"`
	EnemyCount   int                  `json:"enemy_count"`
	LevelExp     []LevelPoint         `json:"level_exp"`
	Pace         []economy.PaceReport `json:"pace"`
	CombatChecks []CombatCheck        `json:"combat_checks"`
	Gacha        GachaPreview         `json:"gacha"`
	Warnings     []string             `json:"warnings"`
	OK           bool                 `json:"ok"`
}

type LevelPoint struct {
	Level     int     `json:"level"`
	ExpToNext float64 `json:"exp_to_next"`
	CumExp    float64 `json:"cum_exp"`
	Days      float64 `json:"days"`
	StatMult  float64 `json:"stat_mult"` // cumulative base stat multiplier at this level
}

type CombatCheck struct {
	HeroID   string  `json:"hero_id"`
	HeroName string  `json:"hero_name"`
	VsEnemy  string  `json:"vs_enemy"`
	WinRate  float64 `json:"win_rate"`
	AvgTurns float64 `json:"avg_turns"`
	OK       bool    `json:"ok"`
	Note     string  `json:"note"`
}

type GachaPreview struct {
	BaseRate5 float64 `json:"base_rate_5"`
	HardPity  int     `json:"hard_pity"`
	SoftStart int     `json:"soft_start"`
	SoftStep  float64 `json:"soft_step"`
	Featured  float64 `json:"featured_rate"`
}
