package balance

import "testing"

func TestValidateAnchors(t *testing.T) {
	w := ValidateAnchors([]Anchor{
		{Name: "BossATK", Value: 1500, Rationale: "sim"},
		{Name: "Bad", Value: 0},
		{Name: "NoWhy", Value: 100},
	})
	if len(w) != 3 {
		t.Fatalf("want 3 warnings, got %v", w)
	}
}

func TestSensitivityRanks(t *testing.T) {
	base := map[string]float64{"atk": 100, "noise": 1}
	obj := func(p map[string]float64) float64 {
		return p["atk"] * 2 // clearly depends on atk only
	}
	res, err := Sensitivity(base, 0.1, obj)
	if err != nil {
		t.Fatal(err)
	}
	if res[0].Param != "atk" {
		t.Fatalf("top influence should be atk, got %v", res[0].Param)
	}
}

func TestAnalyzeDimensions(t *testing.T) {
	rep, err := AnalyzeDimensions([]Dimension{
		{Name: "DPS", Weight: 0.4, Score: 90, Target: 70},
		{Name: "Survival", Weight: 0.3, Score: 70, Target: 70},
		{Name: "Mobility", Weight: 0.3, Score: 40, Target: 70},
	}, 15)
	if err != nil {
		t.Fatal(err)
	}
	if rep.WeightedScore <= 0 {
		t.Fatal("bad score")
	}
	// largest weighted gap should be DPS or Mobility
	if len(rep.Gaps) != 3 {
		t.Fatalf("gaps=%d", len(rep.Gaps))
	}
}

func TestWinRateTarget(t *testing.T) {
	_, v := WinRateTarget(0.51, 0.5)
	if v == "" {
		t.Fatal("empty verdict")
	}
	d, _ := WinRateTarget(0.7, 0.5)
	if d < 0.199 || d > 0.201 {
		t.Fatalf("delta=%v", d)
	}
}
