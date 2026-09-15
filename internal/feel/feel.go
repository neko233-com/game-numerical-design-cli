// Package feel translates vague designer language into measurable thresholds,
// and reverse-maps metrics back to actionable feel diagnoses.
package feel

import (
	"fmt"
	"sort"
	"strings"
)

// Mapping is one 手感量化 rule: when metric is outside [Min,Max], the phrase applies.
type Mapping struct {
	Phrase    string  `json:"phrase"`   // e.g. 打击感太轻
	Category  string  `json:"category"` // combat | progress | economy | gacha
	Metric    string  `json:"metric"`   // machine-readable key
	Unit      string  `json:"unit"`
	Min       float64 `json:"min"` // inclusive healthy band; outside = issue
	Max       float64 `json:"max"`
	Direction string  `json:"direction"` // "low_is_bad" | "high_is_bad" | "band"
	Note      string  `json:"note"`
}

// DefaultMappings is the starter quantification table (extend over time).
func DefaultMappings() []Mapping {
	return []Mapping{
		{
			Phrase: "打击感太轻", Category: "combat", Metric: "hit_stop_frames",
			Unit: "frame@60fps", Min: 6, Max: 12, Direction: "low_is_bad",
			Note: "受击停顿帧 <6 帧时普遍反馈「软」；>12 帧易「拖」",
		},
		{
			Phrase: "伤害数字反馈弱", Category: "combat", Metric: "damage_popup_delay_ms",
			Unit: "ms", Min: 0, Max: 100, Direction: "high_is_bad",
			Note: "伤害弹出延迟 >100ms 会打断打击节奏",
		},
		{
			Phrase: "升级没爽感", Category: "progress", Metric: "level_stat_gain_pct",
			Unit: "%", Min: 8, Max: 35, Direction: "low_is_bad",
			Note: "相邻等级关键属性增幅 <8% 时「升了但没感觉」；>35% 中后期易数值爆炸",
		},
		{
			Phrase: "升级性价比崩了", Category: "progress", Metric: "stat_gain_per_cost_ratio",
			Unit: "ratio", Min: 0.7, Max: 1.3, Direction: "band",
			Note: "属性增幅/养成成本比值相对上一级快速下降 → 「越升越亏」",
		},
		{
			Phrase: "关卡太难", Category: "progress", Metric: "stage_clear_rate",
			Unit: "ratio", Min: 0.15, Max: 1.0, Direction: "low_is_bad",
			Note: "通关率 <15% 且重试 >3 次，或首通时长超预期 40%",
		},
		{
			Phrase: "关卡时长失控", Category: "progress", Metric: "first_clear_time_overrun",
			Unit: "ratio", Min: 0, Max: 0.4, Direction: "high_is_bad",
			Note: "(实际首通时长-设计时长)/设计时长 > 40%",
		},
		{
			Phrase: "经济崩了", Category: "economy", Metric: "daily_prod_cons_ratio",
			Unit: "ratio", Min: 0.9, Max: 1.3, Direction: "band",
			Note: "日产出/日消耗 >1.3 持续 3 天以上判定通胀崩盘",
		},
		{
			Phrase: "资源怎么都不够", Category: "economy", Metric: "daily_prod_cons_ratio",
			Unit: "ratio", Min: 0.9, Max: 1.3, Direction: "band",
			Note: "同指标下限：<0.7 强卡点",
		},
		{
			Phrase: "抽卡体感黑", Category: "gacha", Metric: "p90_pulls_to_top",
			Unit: "pulls", Min: 1, Max: 70, Direction: "high_is_bad",
			Note: "P90 出金抽数接近或超过硬保底会显著放大挫败感",
		},
		{
			Phrase: "保底形同虚设", Category: "gacha", Metric: "hard_pity_hit_rate",
			Unit: "ratio", Min: 0, Max: 0.25, Direction: "high_is_bad",
			Note: "大量玩家靠硬保底才出 → 软保底曲线太缓",
		},
	}
}

// Diagnosis is one decoded feel issue.
type Diagnosis struct {
	Phrase   string  `json:"phrase"`
	Category string  `json:"category"`
	Metric   string  `json:"metric"`
	Value    float64 `json:"value"`
	Healthy  bool    `json:"healthy"`
	Detail   string  `json:"detail"`
	Note     string  `json:"note"`
}

// Decode takes metric readings and returns matching diagnoses.
// readings: metricKey -> observed value (if a metric appears twice in mappings,
// both rules are evaluated).
func Decode(readings map[string]float64, mappings []Mapping) []Diagnosis {
	if mappings == nil {
		mappings = DefaultMappings()
	}
	var out []Diagnosis
	for _, m := range mappings {
		v, ok := readings[m.Metric]
		if !ok {
			continue
		}
		d := Diagnosis{
			Phrase: m.Phrase, Category: m.Category, Metric: m.Metric,
			Value: v, Note: m.Note,
		}
		switch m.Direction {
		case "low_is_bad":
			d.Healthy = v >= m.Min
			d.Detail = fmt.Sprintf("期望 ≥ %g %s，实测 %g", m.Min, m.Unit, v)
		case "high_is_bad":
			d.Healthy = v <= m.Max
			d.Detail = fmt.Sprintf("期望 ≤ %g %s，实测 %g", m.Max, m.Unit, v)
		default: // band
			d.Healthy = v >= m.Min && v <= m.Max
			d.Detail = fmt.Sprintf("期望 [%g, %g] %s，实测 %g", m.Min, m.Max, m.Unit, v)
		}
		if !d.Healthy {
			d.Detail = "【触发】" + d.Detail
		}
		out = append(out, d)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Healthy != out[j].Healthy {
			return !out[i].Healthy && out[j].Healthy
		}
		return out[i].Phrase < out[j].Phrase
	})
	return out
}

// Encode maps a free-text phrase to candidate mappings (fuzzy contains).
func Encode(phrase string, mappings []Mapping) []Mapping {
	if mappings == nil {
		mappings = DefaultMappings()
	}
	p := strings.TrimSpace(phrase)
	if p == "" {
		return nil
	}
	var hits []Mapping
	for _, m := range mappings {
		if strings.Contains(m.Phrase, p) || strings.Contains(p, m.Phrase) ||
			strings.Contains(m.Note, p) {
			hits = append(hits, m)
		}
	}
	return hits
}

// Categories lists distinct categories in the mapping table.
func Categories(mappings []Mapping) []string {
	if mappings == nil {
		mappings = DefaultMappings()
	}
	seen := map[string]bool{}
	var out []string
	for _, m := range mappings {
		if !seen[m.Category] {
			seen[m.Category] = true
			out = append(out, m.Category)
		}
	}
	sort.Strings(out)
	return out
}
