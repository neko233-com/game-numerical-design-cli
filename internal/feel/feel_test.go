package feel

import "testing"

func TestDecodeLevelGainLow(t *testing.T) {
	diags := Decode(map[string]float64{"level_stat_gain_pct": 5}, nil)
	found := false
	for _, d := range diags {
		if d.Metric == "level_stat_gain_pct" && !d.Healthy {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unhealthy level gain diagnosis, got %+v", diags)
	}
}

func TestDecodeEconomyBand(t *testing.T) {
	diags := Decode(map[string]float64{"daily_prod_cons_ratio": 1.5}, nil)
	ok := false
	for _, d := range diags {
		if d.Metric == "daily_prod_cons_ratio" && !d.Healthy {
			ok = true
		}
	}
	if !ok {
		t.Fatal("ratio 1.5 should be unhealthy")
	}
}

func TestEncodePhrase(t *testing.T) {
	hits := Encode("打击感太轻", nil)
	if len(hits) == 0 {
		t.Fatal("no hits for 打击感太轻")
	}
}

func TestCategories(t *testing.T) {
	cats := Categories(nil)
	if len(cats) < 3 {
		t.Fatalf("cats=%v", cats)
	}
}
