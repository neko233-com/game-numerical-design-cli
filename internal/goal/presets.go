// Package goal presets: genre-specific starting goals (sanitized, fictional).
package goal

import "strings"

// PresetID identifies a built-in genre preset.
type PresetID string

const (
	PresetGenshinRPG  PresetID = "genshin" // 动作 RPG（原神式）
	PresetWhiteoutSLG PresetID = "slg"     // SLG 生存（无尽冬日式）
	PresetOnmyojiTurn PresetID = "onmyoji" // 回合制（阴阳师式）
	PresetDefault     PresetID = "default" // alias of genshin
)

// PresetIDs lists all presets in stable order.
func PresetIDs() []PresetID {
	return []PresetID{PresetGenshinRPG, PresetWhiteoutSLG, PresetOnmyojiTurn}
}

// PresetInfo describes a preset for CLI help.
type PresetInfo struct {
	ID          PresetID
	Title       string
	Description string
}

// PresetInfos is human-readable catalog.
func PresetInfos() []PresetInfo {
	return []PresetInfo{
		{PresetGenshinRPG, "原神式动作 RPG", "元素/武器角色、精英机兵、祈愿保底、摩拉经济"},
		{PresetWhiteoutSLG, "无尽冬日式 SLG", "兵种/统率、采集与行军、联盟任务、加速道具经济"},
		{PresetOnmyojiTurn, "阴阳师式回合制", "式神/御魂、速度条、觉醒材料、蓝符召唤"},
	}
}

// Preset returns a built-in goal by id. Unknown id falls back to Genshin.
func Preset(id PresetID) Goal {
	switch id {
	case PresetWhiteoutSLG:
		return presetWhiteoutSLG()
	case PresetOnmyojiTurn:
		return presetOnmyojiTurn()
	case PresetGenshinRPG, PresetDefault, "":
		return DefaultGoal()
	default:
		return DefaultGoal()
	}
}

// ParsePreset normalizes user input to PresetID.
func ParsePreset(s string) PresetID {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "slg", "whiteout", "无尽冬日", "winter":
		return PresetWhiteoutSLG
	case "onmyoji", "turn", "回合", "阴阳师":
		return PresetOnmyojiTurn
	case "genshin", "rpg", "原神", "gi", "default", "":
		return PresetGenshinRPG
	default:
		return PresetGenshinRPG
	}
}

// presetWhiteoutSLG — survival SLG: troops, march time, resource tiles.
// Combat units are troop squads; "Weapon" field holds troop type.
func presetWhiteoutSLG() Goal {
	return Goal{
		Name:        "极地远征数值Demo",
		Description: "简化无尽冬日式 SLG：兵种/统率/采集/行军/加速，用于数值验证（脱敏虚构）",
		Genre:       string(PresetWhiteoutSLG),
		Version:     1,
		Progress: ProgressGoal{
			// furnace / chief level pace
			MaxLevel:    30,
			DailyExp:    900,
			TargetDays:  map[string]float64{"5": 1.2, "10": 3.0, "15": 6.0, "20": 12.0, "30": 25.0},
			EarlySlope:  2.2,
			LateSlope:   0.45,
			BreakLevel:  12,
			StatGainPct: 10,
		},
		Combat: CombatGoal{
			TargetWinRate: 0.52,
			TargetTurns:   6,
			EnemyHPMult:   1.25,
			EnemyAtkMult:  0.6,
			EnemyDefMult:  1.0,
			Archetypes: []Archetype{
				// Weapon = 兵种; Element = 派系
				{ID: "2001", Name: "盾卫方阵", Weapon: "步兵", Element: "守备", Rarity: 5,
					HP: 1600, ATK: 380, DEF: 720, SPD: 70,
					CritRate: 0.05, CritDMG: 0.4,
					BasicMult: 0.9, SkillMult: 1.2, UltMult: 2.0},
				{ID: "2002", Name: "游侠骑射", Weapon: "骑兵", Element: "突击", Rarity: 5,
					HP: 1000, ATK: 780, DEF: 320, SPD: 120,
					CritRate: 0.1, CritDMG: 0.55,
					BasicMult: 1.15, SkillMult: 1.7, UltMult: 2.9},
				{ID: "2003", Name: "火炮连队", Weapon: "攻城", Element: "火力", Rarity: 5,
					HP: 900, ATK: 860, DEF: 280, SPD: 85,
					CritRate: 0.08, CritDMG: 0.5,
					BasicMult: 1.0, SkillMult: 1.9, UltMult: 3.4},
				{ID: "2004", Name: "斥候小队", Weapon: "侦察", Element: "机动", Rarity: 4,
					HP: 950, ATK: 520, DEF: 360, SPD: 130,
					CritRate: 0.07, CritDMG: 0.45,
					BasicMult: 1.0, SkillMult: 1.3, UltMult: 1.8},
				{ID: "2005", Name: "医师队", Weapon: "辅助", Element: "后勤", Rarity: 4,
					HP: 1200, ATK: 360, DEF: 520, SPD: 90,
					CritRate: 0.03, CritDMG: 0.3,
					BasicMult: 0.7, SkillMult: 0.9, UltMult: 1.2},
				{ID: "2006", Name: "联盟精锐", Weapon: "混合", Element: "联盟", Rarity: 4,
					HP: 1300, ATK: 560, DEF: 480, SPD: 100,
					CritRate: 0.06, CritDMG: 0.45,
					BasicMult: 0.95, SkillMult: 1.35, UltMult: 2.2},
			},
		},
		Tasks: TaskGoal{
			DailyCount: 5,
			DailyExp:   180,
			MainCount:  20,
			SideShare:  0.25,
		},
		Gacha: GachaGoal{
			// SLG often has lower hard pity / more shards
			BaseRate5: 0.012, HardPity: 50, SoftStartRatio: 0.7,
			FeaturedRate: 0.6, Guarantee: true,
			BaseRate4: 0.08, HardPity4: 8,
		},
		Economy: EconomyGoal{
			// tighter band: SLG inflation from gathering is a classic fail
			ProdConsMin: 0.95, ProdConsMax: 1.12,
			StarterGold: 20000, GoldCostMax: 800000,
		},
	}
}

// presetOnmyojiTurn — turn-based: speed bar, skill points, souls.
func presetOnmyojiTurn() Goal {
	return Goal{
		Name:        "平安京式神数值Demo",
		Description: "简化阴阳师式回合制：式神/御魂/速度条/觉醒，用于数值验证（脱敏虚构）",
		Genre:       string(PresetOnmyojiTurn),
		Version:     1,
		Progress: ProgressGoal{
			MaxLevel:    40,
			DailyExp:    1500,
			TargetDays:  map[string]float64{"10": 0.8, "20": 2.0, "30": 4.5, "40": 10.0},
			EarlySlope:  2.0,
			LateSlope:   0.55,
			BreakLevel:  25,
			StatGainPct: 14,
		},
		Combat: CombatGoal{
			TargetWinRate: 0.58,
			TargetTurns:   10,
			EnemyHPMult:   1.1,
			EnemyAtkMult:  0.5,
			EnemyDefMult:  0.85,
			Archetypes: []Archetype{
				// Weapon = 定位; Element = 属性
				{ID: "3001", Name: "赤焰·输出", Weapon: "输出", Element: "火", Rarity: 5,
					HP: 1050, ATK: 820, DEF: 300, SPD: 110,
					CritRate: 0.1, CritDMG: 0.6,
					BasicMult: 1.05, SkillMult: 1.85, UltMult: 3.3},
				{ID: "3002", Name: "青岚·控制", Weapon: "控制", Element: "风", Rarity: 5,
					HP: 1100, ATK: 560, DEF: 400, SPD: 125,
					CritRate: 0.05, CritDMG: 0.4,
					BasicMult: 0.85, SkillMult: 1.25, UltMult: 2.0},
				{ID: "3003", Name: "玄水·治疗", Weapon: "治疗", Element: "水", Rarity: 5,
					HP: 1250, ATK: 420, DEF: 480, SPD: 100,
					CritRate: 0.03, CritDMG: 0.3,
					BasicMult: 0.7, SkillMult: 0.95, UltMult: 1.4},
				{ID: "3004", Name: "黄岩·护盾", Weapon: "护盾", Element: "土", Rarity: 4,
					HP: 1500, ATK: 350, DEF: 700, SPD: 85,
					CritRate: 0.03, CritDMG: 0.3,
					BasicMult: 0.75, SkillMult: 1.0, UltMult: 1.6},
				{ID: "3005", Name: "白藏·拉条", Weapon: "辅助", Element: "风", Rarity: 4,
					HP: 1000, ATK: 480, DEF: 380, SPD: 140,
					CritRate: 0.05, CritDMG: 0.35,
					BasicMult: 0.8, SkillMult: 1.1, UltMult: 1.5},
				{ID: "3006", Name: "紫电·单体", Weapon: "输出", Element: "雷", Rarity: 4,
					HP: 980, ATK: 760, DEF: 320, SPD: 115,
					CritRate: 0.12, CritDMG: 0.65,
					BasicMult: 1.1, SkillMult: 1.95, UltMult: 3.0},
			},
		},
		Tasks: TaskGoal{
			DailyCount: 4,
			DailyExp:   350,
			MainCount:  22,
			SideShare:  0.18,
		},
		Gacha: GachaGoal{
			// 阴阳师式：较低基础率 + 较紧保底
			BaseRate5: 0.008, HardPity: 60, SoftStartRatio: 0.68,
			FeaturedRate: 0.5, Guarantee: true,
			BaseRate4: 0.06, HardPity4: 10,
		},
		Economy: EconomyGoal{
			ProdConsMin: 0.9, ProdConsMax: 1.2,
			StarterGold: 8000, GoldCostMax: 200000,
		},
	}
}
