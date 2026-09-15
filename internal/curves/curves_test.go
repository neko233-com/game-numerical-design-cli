package curves

import (
	"math"
	"testing"
)

func TestLinear(t *testing.T) {
	y, err := Evaluate(Linear, 10, Params{A: 5, B: 2})
	if err != nil {
		t.Fatal(err)
	}
	if math.Abs(y-25) > 1e-9 {
		t.Fatalf("got %v want 25", y)
	}
}

func TestPiecewiseLogContinuity(t *testing.T) {
	p := Params{S1: 1.8, S2: 0.6, BreakX: 30}
	atBreak, _ := Evaluate(PiecewiseLog, 30, p)
	justAfter, _ := Evaluate(PiecewiseLog, 30.0001, p)
	if math.Abs(atBreak-justAfter) > 1e-2 {
		t.Fatalf("discontinuity at break: %v vs %v", atBreak, justAfter)
	}
	// early slope steeper: compare gain from 1→2 vs 50→51
	y1, _ := Evaluate(PiecewiseLog, 1, p)
	y2, _ := Evaluate(PiecewiseLog, 2, p)
	y50, _ := Evaluate(PiecewiseLog, 50, p)
	y51, _ := Evaluate(PiecewiseLog, 51, p)
	if y2-y1 <= y51-y50 {
		t.Fatalf("expected early absolute gain larger: early=%v late=%v", y2-y1, y51-y50)
	}
}

func TestSigmoidBounded(t *testing.T) {
	p := Params{L: 100, K: 0.3, X0: 20, Base: 10}
	y0, _ := Evaluate(Sigmoid, 0, p)
	yBig, _ := Evaluate(Sigmoid, 200, p)
	if y0 < 10 || y0 > 50 {
		t.Fatalf("y(0)=%v unexpected", y0)
	}
	if yBig < 105 || yBig > 110.1 {
		t.Fatalf("y(200)=%v should approach base+L=110", yBig)
	}
}

func TestSmoothnessCliff(t *testing.T) {
	// exponential with large b should flag cliff at high x
	rep, err := AnalyzeSmoothness(Exponential, Params{A: 1, B: 0.5}, 20, 20, 0.35)
	if err != nil {
		t.Fatal(err)
	}
	if !rep.HasCliff {
		t.Fatalf("expected cliff, maxRatio=%v", rep.MaxAbsDeltaRatio)
	}
}

func TestGrowthRate(t *testing.T) {
	r, err := GrowthRate(Linear, Params{A: 100, B: 10}, 10)
	if err != nil {
		t.Fatal(err)
	}
	// y(10)=200, y(11)=210 → 10/200 = 0.05
	if math.Abs(r-0.05) > 1e-9 {
		t.Fatalf("got %v", r)
	}
}
