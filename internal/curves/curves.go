// Package curves provides reusable growth-curve models for game numerical design.
//
// Each model is a pure function f(x) with named parameters, plus a Fit helper
// that samples the curve into a table for plotting / export.
package curves

import (
	"fmt"
	"math"
)

// Kind enumerates supported growth models.
type Kind string

const (
	Linear       Kind = "linear"
	Exponential  Kind = "exponential"
	Logarithmic  Kind = "logarithmic"
	Power        Kind = "power"
	Sigmoid      Kind = "sigmoid"
	Quadratic    Kind = "quadratic"
	PiecewiseLog Kind = "piecewise-log"
)

// Kinds lists all supported curve kinds in a stable order.
func Kinds() []Kind {
	return []Kind{Linear, Exponential, Logarithmic, Power, Sigmoid, Quadratic, PiecewiseLog}
}

// Params holds curve parameters. Unused fields are ignored per kind.
type Params struct {
	// Linear: y = a + b*x
	A float64
	B float64

	// Exponential: y = a * e^(b*x)
	// Power:        y = a * x^b
	// Quadratic:    y = a + b*x + c*x^2
	C float64

	// Sigmoid: y = L / (1 + e^(-k*(x-x0))) + base
	L    float64 // capacity / max amplitude
	K    float64 // steepness
	X0   float64 // midpoint
	Base float64

	// PiecewiseLog: two-phase log with slope break at BreakX
	// y = s1*ln(1+x) for x<=BreakX; then continuous continuation with slope s2
	S1     float64
	S2     float64
	BreakX float64
}

// Describe returns a one-line formula description for the kind.
func Describe(k Kind) string {
	switch k {
	case Linear:
		return "y = a + b·x  — 等差成长，适合短周期或手动校准段"
	case Exponential:
		return "y = a·e^(b·x) — 指数膨胀，仅限超短周期或「爆炸性爽点」，极易失控"
	case Logarithmic:
		return "y = a·ln(1+b·x) — 对数成长，前快后缓，新手期爽感常用"
	case Power:
		return "y = a·x^b — 幂律成长；b<1 边际递减，b>1 越滚越大"
	case Sigmoid:
		return "y = L/(1+e^(-k(x-x0))) + base — S 曲线，有上限的平滑成长"
	case Quadratic:
		return "y = a + b·x + c·x² — 二次加速，中后期拉开差距"
	case PiecewiseLog:
		return "分段对数：前期斜率 s1，x>break 后斜率 s2（如 1.8→0.6）"
	default:
		return string(k)
	}
}

// Evaluate computes y = f(x) for the given kind and params.
func Evaluate(k Kind, x float64, p Params) (float64, error) {
	if x < 0 {
		return 0, fmt.Errorf("x must be >= 0, got %v", x)
	}
	switch k {
	case Linear:
		return p.A + p.B*x, nil
	case Exponential:
		return p.A * math.Exp(p.B*x), nil
	case Logarithmic:
		return p.A * math.Log(1+p.B*x), nil
	case Power:
		if x == 0 {
			if p.B > 0 {
				return 0, nil
			}
			return 0, fmt.Errorf("power curve undefined at x=0 when b<=0")
		}
		return p.A * math.Pow(x, p.B), nil
	case Sigmoid:
		return p.Base + p.L/(1+math.Exp(-p.K*(x-p.X0))), nil
	case Quadratic:
		return p.A + p.B*x + p.C*x*x, nil
	case PiecewiseLog:
		if p.BreakX <= 0 {
			return 0, fmt.Errorf("piecewise-log requires breakX > 0")
		}
		if x <= p.BreakX {
			return p.S1 * math.Log(1+x), nil
		}
		// continuous: at break, y = s1*ln(1+break); after break add s2*ln(x/break)
		yBreak := p.S1 * math.Log(1+p.BreakX)
		return yBreak + p.S2*math.Log(x/p.BreakX), nil
	default:
		return 0, fmt.Errorf("unknown curve kind %q", k)
	}
}

// SamplePoint is one sampled point on a curve.
type SamplePoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// Sample generates n+1 points from x=0..maxX inclusive.
func Sample(k Kind, p Params, maxX float64, n int) ([]SamplePoint, error) {
	if n < 1 {
		return nil, fmt.Errorf("n must be >= 1")
	}
	if maxX < 0 {
		return nil, fmt.Errorf("maxX must be >= 0")
	}
	out := make([]SamplePoint, 0, n+1)
	for i := 0; i <= n; i++ {
		x := maxX * float64(i) / float64(n)
		y, err := Evaluate(k, x, p)
		if err != nil {
			return nil, err
		}
		out = append(out, SamplePoint{X: x, Y: y})
	}
	return out, nil
}

// GrowthRate returns relative growth from x to x+1: (y(x+1)-y(x))/y(x).
// Returns 0 when y(x)==0.
func GrowthRate(k Kind, p Params, x float64) (float64, error) {
	y0, err := Evaluate(k, x, p)
	if err != nil {
		return 0, err
	}
	y1, err := Evaluate(k, x+1, p)
	if err != nil {
		return 0, err
	}
	if y0 == 0 {
		return 0, nil
	}
	return (y1 - y0) / y0, nil
}

// SmoothnessReport inspects a sampled curve for cliff jumps / flat stretches.
type SmoothnessReport struct {
	Points           []SamplePoint `json:"points"`
	MaxAbsDeltaRatio float64       `json:"max_abs_delta_ratio"`
	AtX              float64       `json:"at_x"`
	HasCliff         bool          `json:"has_cliff"`
	HasLongFlat      bool          `json:"has_long_flat"`
	Warnings         []string      `json:"warnings"`
}

// AnalyzeSmoothness flags cliffs (relative step > cliffThreshold) and long flat runs.
func AnalyzeSmoothness(k Kind, p Params, maxX float64, n int, cliffThreshold float64) (*SmoothnessReport, error) {
	pts, err := Sample(k, p, maxX, n)
	if err != nil {
		return nil, err
	}
	rep := &SmoothnessReport{Points: pts}
	if cliffThreshold <= 0 {
		cliffThreshold = 0.35
	}
	flatCount := 0
	for i := 1; i < len(pts); i++ {
		prev, cur := pts[i-1], pts[i]
		// Relative change is undefined at y=0; skip cliff/flat so 0→positive
		// starting from origin is not a false cliff.
		if prev.Y == 0 {
			flatCount = 0
			continue
		}
		ratio := math.Abs(cur.Y-prev.Y) / math.Abs(prev.Y)
		if ratio > rep.MaxAbsDeltaRatio {
			rep.MaxAbsDeltaRatio = ratio
			rep.AtX = cur.X
		}
		if ratio > cliffThreshold {
			rep.HasCliff = true
		}
		if ratio < 0.001 {
			flatCount++
			if flatCount >= 5 {
				rep.HasLongFlat = true
			}
		} else {
			flatCount = 0
		}
	}
	if rep.HasCliff {
		rep.Warnings = append(rep.Warnings,
			fmt.Sprintf("存在断崖式跳跃：x≈%.2f 处相邻点相对变化 %.1f%% > 阈值 %.0f%%",
				rep.AtX, rep.MaxAbsDeltaRatio*100, cliffThreshold*100))
	}
	if rep.HasLongFlat {
		rep.Warnings = append(rep.Warnings, "存在较长平坦段（≥5 步相对变化 <0.1%），成长感可能停滞")
	}
	return rep, nil
}
