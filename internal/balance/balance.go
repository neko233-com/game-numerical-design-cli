// Package balance provides anchor selection, sensitivity analysis, and
// dimension-based balance scoring frameworks.
package balance

import (
	"fmt"
	"math"
	"sort"
)

// Anchor is a benchmark value other numbers are designed around.
type Anchor struct {
	Name      string  `json:"name"`
	Value     float64 `json:"value"`
	Unit      string  `json:"unit"`
	Rationale string  `json:"rationale"` // 必须有依据，不能拍脑袋
}

// ValidateAnchors flags anchors without rationale or non-positive values.
func ValidateAnchors(anchors []Anchor) []string {
	var warnings []string
	for _, a := range anchors {
		if a.Value <= 0 {
			warnings = append(warnings, fmt.Sprintf("锚点 %q 值为 %v，通常应 > 0", a.Name, a.Value))
		}
		if a.Rationale == "" {
			warnings = append(warnings, fmt.Sprintf("锚点 %q 缺少依据（rationale）— 禁止拍脑袋", a.Name))
		}
	}
	return warnings
}

// Param is one tunable parameter for sensitivity analysis.
type Param struct {
	Name  string  `json:"name"`
	Value float64 `json:"value"`
	// Min/Max optional bounds for local probe.
	Min float64 `json:"min"`
	Max float64 `json:"max"`
}

// Objective evaluates a parameter vector into a scalar score (higher = better
// under your chosen metric, e.g. win-rate closeness to 0.5, or D7 retention proxy).
type Objective func(params map[string]float64) float64

// SensitivityResult ranks parameters by local influence.
type SensitivityResult struct {
	Param      string  `json:"param"`
	BaseValue  float64 `json:"base_value"`
	Delta      float64 `json:"delta"`
	ScoreBase  float64 `json:"score_base"`
	ScoreUp    float64 `json:"score_up"`
	ScoreDown  float64 `json:"score_down"`
	Derivative float64 `json:"derivative"` // dScore / dParam at base
	Elasticity float64 `json:"elasticity"` // (%ΔScore) / (%ΔParam)
	Influence  float64 `json:"influence"`  // abs(derivative) * param scale
}

// Sensitivity probes each parameter ±relativeDelta (default 10%) around base.
func Sensitivity(base map[string]float64, relDelta float64, obj Objective) ([]SensitivityResult, error) {
	if obj == nil {
		return nil, fmt.Errorf("objective is nil")
	}
	if relDelta <= 0 {
		relDelta = 0.1
	}
	baseScore := obj(base)
	var out []SensitivityResult
	names := make([]string, 0, len(base))
	for k := range base {
		names = append(names, k)
	}
	sort.Strings(names)

	for _, name := range names {
		v := base[name]
		d := math.Abs(v) * relDelta
		if d == 0 {
			d = relDelta
		}
		up := clone(base)
		up[name] = v + d
		down := clone(base)
		down[name] = v - d
		su, sd := obj(up), obj(down)
		deriv := (su - sd) / (2 * d)
		elast := 0.0
		if baseScore != 0 && v != 0 {
			// average of one-sided elasticities
			eu := ((su - baseScore) / baseScore) / (d / v)
			ed := ((sd - baseScore) / baseScore) / (-d / v)
			elast = (eu + ed) / 2
		}
		out = append(out, SensitivityResult{
			Param: name, BaseValue: v, Delta: d,
			ScoreBase: baseScore, ScoreUp: su, ScoreDown: sd,
			Derivative: deriv, Elasticity: elast,
			Influence: math.Abs(deriv) * math.Abs(v),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].Influence > out[j].Influence
	})
	return out, nil
}

func clone(m map[string]float64) map[string]float64 {
	c := make(map[string]float64, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}

// Dimension is one balance axis (DPS, survival, mobility, skill ceiling...).
type Dimension struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
	// Score 0..100 for the subject.
	Score float64 `json:"score"`
	// Target optional ideal score.
	Target float64 `json:"target"`
}

// DimensionReport scores weighted balance across dimensions.
type DimensionReport struct {
	WeightedScore float64        `json:"weighted_score"`
	Gaps          []DimensionGap `json:"gaps"`
	Verdict       string         `json:"verdict"`
}

// DimensionGap is |score-target| for one dimension.
type DimensionGap struct {
	Name     string  `json:"name"`
	Score    float64 `json:"score"`
	Target   float64 `json:"target"`
	Gap      float64 `json:"gap"`
	Weighted float64 `json:"weighted"`
}

// AnalyzeDimensions computes weighted score and largest gaps vs target.
func AnalyzeDimensions(dims []Dimension, gapWarn float64) (*DimensionReport, error) {
	if len(dims) == 0 {
		return nil, fmt.Errorf("no dimensions")
	}
	if gapWarn <= 0 {
		gapWarn = 15
	}
	var wsum, score float64
	rep := &DimensionReport{}
	for _, d := range dims {
		if d.Weight < 0 {
			return nil, fmt.Errorf("dimension %s weight must be >= 0", d.Name)
		}
		wsum += d.Weight
		score += d.Weight * d.Score
		if d.Target > 0 {
			gap := d.Score - d.Target
			rep.Gaps = append(rep.Gaps, DimensionGap{
				Name: d.Name, Score: d.Score, Target: d.Target,
				Gap: gap, Weighted: gap * d.Weight,
			})
		}
	}
	if wsum == 0 {
		return nil, fmt.Errorf("total weight is 0")
	}
	rep.WeightedScore = score / wsum
	sort.Slice(rep.Gaps, func(i, j int) bool {
		return math.Abs(rep.Gaps[i].Weighted) > math.Abs(rep.Gaps[j].Weighted)
	})
	bad := 0
	for _, g := range rep.Gaps {
		if math.Abs(g.Gap) > gapWarn {
			bad++
		}
	}
	switch {
	case bad == 0:
		rep.Verdict = "各维度贴近目标"
	case bad <= 2:
		rep.Verdict = fmt.Sprintf("%d 个维度偏离目标超过 %.0f 分，优先修加权偏差最大的", bad, gapWarn)
	default:
		rep.Verdict = fmt.Sprintf("%d 个维度失衡，可能需要重设锚点而非只调参", bad)
	}
	return rep, nil
}

// ExtremeCase checks full-max / full-zero safety.
type ExtremeCase struct {
	Name   string             `json:"name"`
	Params map[string]float64 `json:"params"`
}

// ExtremeResult reports whether extreme vectors produce safe scores.
type ExtremeResult struct {
	Name   string  `json:"name"`
	Score  float64 `json:"score"`
	Safe   bool    `json:"safe"`
	Reason string  `json:"reason"`
}

// CheckExtremes evaluates named extreme parameter vectors under a safety predicate.
func CheckExtremes(cases []ExtremeCase, obj Objective, safe func(score float64) bool) ([]ExtremeResult, error) {
	if obj == nil {
		return nil, fmt.Errorf("objective is nil")
	}
	var out []ExtremeResult
	for _, c := range cases {
		s := obj(c.Params)
		ok := safe(s)
		reason := "ok"
		if !ok {
			reason = "极端属性下得分越界 — 检查上限/下限保护"
		}
		out = append(out, ExtremeResult{Name: c.Name, Score: s, Safe: ok, Reason: reason})
	}
	return out, nil
}

// WinRateTarget reports how far an observed win rate is from design target.
func WinRateTarget(observed, target float64) (delta float64, verdict string) {
	if target <= 0 {
		target = 0.5
	}
	delta = observed - target
	switch {
	case math.Abs(delta) <= 0.02:
		verdict = "胜率在 ±2pp 内 — 可接受"
	case math.Abs(delta) <= 0.05:
		verdict = "胜率偏离 2~5pp — 轻微失衡，观察高分段/低分段拆分"
	default:
		verdict = fmt.Sprintf("胜率偏离 %.1fpp — 需公式或参数层修复，不是微调能救", delta*100)
	}
	return delta, verdict
}
