package economy

import "testing"

func TestAnalyzeBalanceInflation(t *testing.T) {
	r := AnalyzeBalance(Flow{Name: "gold", Production: 2000, Consumption: 1000})
	if r.Severity != "critical" {
		t.Fatalf("severity=%s want critical", r.Severity)
	}
	if r.Ratio != 2 {
		t.Fatalf("ratio=%v", r.Ratio)
	}
}

func TestAnalyzeBalanceHealthy(t *testing.T) {
	r := AnalyzeBalance(Flow{Name: "gold", Production: 1000, Consumption: 1000})
	if r.Severity != "ok" {
		t.Fatalf("severity=%s", r.Severity)
	}
}

func TestAnalyzeBalanceNoSink(t *testing.T) {
	r := AnalyzeBalance(Flow{Name: "token", Production: 100, Consumption: 0})
	if r.Severity != "critical" {
		t.Fatalf("want critical for pure faucet, got %s", r.Severity)
	}
}

func TestProjectStock(t *testing.T) {
	s, err := ProjectStock(100, Flow{Production: 10, Consumption: 5}, 3, 0)
	if err != nil {
		t.Fatal(err)
	}
	// 100,105,110,115
	if s[3] != 115 {
		t.Fatalf("got %v", s)
	}
}

func TestAnalyzePace(t *testing.T) {
	reps, err := AnalyzePace(PaceConfig{
		LevelExp:   []float64{800, 2000, 5000},
		DailyExp:   1000,
		TargetDays: []float64{1, 2, 4},
	})
	if err != nil {
		t.Fatal(err)
	}
	// level1: 0.8 days vs 1 → slightly fast; level3: 5 days vs 4 → slow but <40%
	if reps[0].DaysNeeded != 0.8 {
		t.Fatalf("lv1 days=%v", reps[0].DaysNeeded)
	}
	if reps[2].Verdict == "" {
		t.Fatal("missing verdict")
	}
}

func TestAnalyzeLoop(t *testing.T) {
	rep := AnalyzeLoop([]LoopStage{
		{Role: "source", Name: "quest", Outflow: 1000},
		{Role: "sink", Name: "shop", Inflow: 950},
	}, 0.1)
	if !rep.IsClosed {
		t.Fatalf("should be closed: %v", rep.Warnings)
	}
	rep2 := AnalyzeLoop([]LoopStage{
		{Role: "source", Name: "quest", Outflow: 1000},
		{Role: "sink", Name: "shop", Inflow: 400},
	}, 0.1)
	if rep2.IsClosed {
		t.Fatal("should flag large leak")
	}
}
