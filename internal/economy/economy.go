// Package economy models resource production/consumption, inflation risk,
// and closed-loop integrity for game economic systems.
package economy

import (
	"fmt"
	"math"
)

// Flow is a daily (or per-period) production/consumption pair for one currency.
type Flow struct {
	Name        string  `json:"name"`
	Production  float64 `json:"production"` // per period, total server or cohort
	Consumption float64 `json:"consumption"`
	Stock       float64 `json:"stock"` // current circulating stock (optional, for velocity)
}

// BalanceReport describes production vs consumption health.
type BalanceReport struct {
	Name            string   `json:"name"`
	Ratio           float64  `json:"ratio"` // production / consumption
	Net             float64  `json:"net"`
	Verdict         string   `json:"verdict"`
	Severity        string   `json:"severity"` // ok | warn | critical
	InflationRisk   string   `json:"inflation_risk"`
	Recommendations []string `json:"recommendations"`
}

// AnalyzeBalance classifies a single flow.
// Rules of thumb (tunable via thresholds):
//
//	ratio > 1.3 sustained → inflation / 经济崩了
//	ratio < 0.7           → 通缩 / 卡进度
//	0.9..1.1              → healthy tight loop
func AnalyzeBalance(f Flow) BalanceReport {
	r := BalanceReport{Name: f.Name}
	if f.Consumption == 0 {
		if f.Production > 0 {
			r.Ratio = math.Inf(1)
			r.Net = f.Production
			r.Verdict = "有产出无消耗出口 — 纯通胀源"
			r.Severity = "critical"
			r.InflationRisk = "critical"
			r.Recommendations = append(r.Recommendations,
				"必须补充消耗出口（强化/商店/活动/回收）")
			return r
		}
		r.Ratio = 0
		r.Verdict = "无产出无消耗 — 死资源"
		r.Severity = "warn"
		r.Recommendations = append(r.Recommendations, "确认该货币是否废弃；若保留需定义用途")
		return r
	}
	r.Ratio = f.Production / f.Consumption
	r.Net = f.Production - f.Consumption

	switch {
	case r.Ratio > 1.3:
		r.Verdict = "产出显著大于消耗 — 通胀风险"
		r.Severity = "critical"
		r.InflationRisk = "high"
		r.Recommendations = append(r.Recommendations,
			"压产出或加消耗；检查是否有无底洞式产出（挂机/扫荡）",
			fmt.Sprintf("目标把 ratio 压到 1.05~1.20（当前 %.2f）", r.Ratio))
	case r.Ratio > 1.15:
		r.Verdict = "产出略偏高 — 轻度通胀"
		r.Severity = "warn"
		r.InflationRisk = "medium"
		r.Recommendations = append(r.Recommendations, "观察 3 日存量曲线；考虑限时消耗活动")
	case r.Ratio >= 0.9:
		r.Verdict = "产出消耗基本平衡"
		r.Severity = "ok"
		r.InflationRisk = "low"
	case r.Ratio >= 0.7:
		r.Verdict = "消耗略偏高 — 轻度通缩/卡感"
		r.Severity = "warn"
		r.InflationRisk = "deflation"
		r.Recommendations = append(r.Recommendations, "检查新手/回流是否卡在该资源；适当补产出")
	default:
		r.Verdict = "消耗显著大于产出 — 通缩/强卡点"
		r.Severity = "critical"
		r.InflationRisk = "deflation"
		r.Recommendations = append(r.Recommendations,
			"玩家会感到「怎么都不够」；补产出或下调消耗锚点")
	}
	return r
}

// ProjectStock projects stock over n periods with constant flows and optional sink rate
// (fraction of stock removed per period, e.g. decay/tax).
func ProjectStock(initialStock float64, f Flow, periods int, decayRate float64) ([]float64, error) {
	if periods < 1 {
		return nil, fmt.Errorf("periods must be >= 1")
	}
	if decayRate < 0 || decayRate >= 1 {
		return nil, fmt.Errorf("decayRate must be in [0,1)")
	}
	out := make([]float64, periods+1)
	s := initialStock
	out[0] = s
	for i := 1; i <= periods; i++ {
		s = s*(1-decayRate) + f.Production - f.Consumption
		if s < 0 {
			s = 0
		}
		out[i] = s
	}
	return out, nil
}

// LoopStage is one node in a production→consumption loop.
type LoopStage struct {
	Role    string  `json:"role"` // source | sink | convert
	Name    string  `json:"name"`
	Inflow  float64 `json:"inflow"`
	Outflow float64 `json:"outflow"`
	Notes   string  `json:"notes"`
}

// LoopReport checks closed-loop integrity.
type LoopReport struct {
	TotalIn  float64  `json:"total_in"`
	TotalOut float64  `json:"total_out"`
	Leak     float64  `json:"leak"`    // in - out, positive = money printing
	Orphans  []string `json:"orphans"` // sinks with no matching source notes
	IsClosed bool     `json:"is_closed"`
	Warnings []string `json:"warnings"`
}

// AnalyzeLoop verifies that major sources have sinks and net leak is bounded.
func AnalyzeLoop(stages []LoopStage, maxLeakRatio float64) LoopReport {
	if maxLeakRatio <= 0 {
		maxLeakRatio = 0.1
	}
	rep := LoopReport{IsClosed: true}
	var sources, sinks float64
	for _, s := range stages {
		switch s.Role {
		case "source":
			sources += s.Outflow
			rep.TotalIn += s.Outflow
		case "sink":
			sinks += s.Inflow
			rep.TotalOut += s.Inflow
			if s.Inflow == 0 {
				rep.Orphans = append(rep.Orphans, s.Name)
			}
		case "convert":
			rep.TotalIn += s.Inflow
			rep.TotalOut += s.Outflow
		}
	}
	rep.Leak = rep.TotalIn - rep.TotalOut
	if sources > 0 && math.Abs(rep.Leak)/sources > maxLeakRatio {
		rep.IsClosed = false
		rep.Warnings = append(rep.Warnings,
			fmt.Sprintf("净泄漏 %.1f 占总产出 %.1f%%，超过阈值 %.0f%% — 闭环不完整",
				rep.Leak, 100*math.Abs(rep.Leak)/sources, maxLeakRatio*100))
	}
	if len(rep.Orphans) > 0 {
		rep.IsClosed = false
		rep.Warnings = append(rep.Warnings, "存在无流入的 sink："+fmt.Sprint(rep.Orphans))
	}
	return rep
}

// PaceConfig models player progression pacing for early retention.
type PaceConfig struct {
	// Cumulative EXP required curve sample (index = level-1).
	LevelExp []float64
	// Daily exp production for the target cohort.
	DailyExp float64
	// Target days to reach level i (same length as LevelExp) — optional benchmarks.
	TargetDays []float64
}

// PaceReport tells whether early leveling pace matches design intent.
type PaceReport struct {
	Level         int     `json:"level"`
	RequiredTotal float64 `json:"required_total"`
	DaysNeeded    float64 `json:"days_needed"`
	TargetDays    float64 `json:"target_days"`
	DeltaRatio    float64 `json:"delta_ratio"` // (needed-target)/target
	Verdict       string  `json:"verdict"`
}

// AnalyzePace compares days-needed vs target for each level.
func AnalyzePace(cfg PaceConfig) ([]PaceReport, error) {
	if cfg.DailyExp <= 0 {
		return nil, fmt.Errorf("dailyExp must be > 0")
	}
	if len(cfg.LevelExp) == 0 {
		return nil, fmt.Errorf("levelExp is empty")
	}
	var out []PaceReport
	for i, req := range cfg.LevelExp {
		days := req / cfg.DailyExp
		r := PaceReport{
			Level: i + 1, RequiredTotal: req, DaysNeeded: days,
		}
		if i < len(cfg.TargetDays) {
			r.TargetDays = cfg.TargetDays[i]
			if r.TargetDays > 0 {
				r.DeltaRatio = (days - r.TargetDays) / r.TargetDays
			}
		}
		switch {
		case r.TargetDays == 0:
			r.Verdict = "无目标"
		case r.DeltaRatio > 0.4:
			r.Verdict = "显著慢于预期（超目标 40%+）— 可能卡关/流失点"
		case r.DeltaRatio > 0.15:
			r.Verdict = "偏慢"
		case r.DeltaRatio >= -0.15:
			r.Verdict = "符合预期"
		case r.DeltaRatio >= -0.4:
			r.Verdict = "偏快"
		default:
			r.Verdict = "显著快于预期 — 前 3 天成长爽感可能足够，但中后期易空窗"
		}
		out = append(out, r)
	}
	return out, nil
}
