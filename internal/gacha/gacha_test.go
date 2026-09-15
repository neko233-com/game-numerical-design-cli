package gacha

import (
	"math/rand"
	"testing"
)

func TestHardPityForces(t *testing.T) {
	b := Banner{
		Name: "t",
		Rarities: []Rarity{
			{Name: "top", Rate: 0.001, HardPity: 10},
			{Name: "low", Rate: 0.999},
		},
	}
	// force low rates; with hard 10, first top must appear by 10
	rng := rand.New(rand.NewSource(1))
	for i := 0; i < 50; i++ {
		sr, err := SimulateSession(b, 10, true, rng)
		if err != nil {
			t.Fatal(err)
		}
		if sr.FirstTopAt < 0 || sr.FirstTopAt > 10 {
			t.Fatalf("first top at %d, want 1..10", sr.FirstTopAt)
		}
	}
}

func TestSoftPityRaisesRate(t *testing.T) {
	r := Rarity{Name: "top", Rate: 0.006, SoftPityStart: 50, SoftPityStep: 0.05, HardPity: 90}
	r0 := rateAt(r, 1)
	r50 := rateAt(r, 50)
	r80 := rateAt(r, 80)
	if !(r0 < r50 && r50 < r80) {
		t.Fatalf("rates not increasing: %v %v %v", r0, r50, r80)
	}
	if rateAt(r, 90) != 1 {
		t.Fatalf("hard pity should force 1, got %v", rateAt(r, 90))
	}
}

func TestExpectedPullsReasonable(t *testing.T) {
	r := Rarity{Name: "top", Rate: 0.01, HardPity: 90}
	exp, err := ExpectedPullsToTop(r)
	if err != nil {
		t.Fatal(err)
	}
	// with 1% and hard 90, EV should be well below 90 and above 50-ish
	if exp < 40 || exp > 85 {
		t.Fatalf("E[pulls]=%v out of expected band", exp)
	}
}

func TestSoftPitySuggest(t *testing.T) {
	start, step := SoftPitySuggest(0.006, 90)
	if start < 40 || start > 80 {
		t.Fatalf("start=%d", start)
	}
	if step <= 0 {
		t.Fatalf("step=%v", step)
	}
}

func TestDistributionCumCurveMonotone(t *testing.T) {
	b := Banner{
		Name: "t",
		Rarities: []Rarity{
			{Name: "top", Rate: 0.02, SoftPityStart: 40, SoftPityStep: 0.04, HardPity: 80},
			{Name: "low", Rate: 0.98},
		},
	}
	rep, err := SimulateDistribution(b, 80, 2000, 3)
	if err != nil {
		t.Fatal(err)
	}
	prev := -1.0
	for _, n := range []int{10, 20, 30, 50, 74, 80} {
		p, ok := rep.CumTopCurve[n]
		if !ok {
			continue
		}
		if p < prev-1e-9 {
			t.Fatalf("cum curve not monotone at %d: %v < %v", n, p, prev)
		}
		prev = p
	}
	if rep.TopRateObserved <= 0 {
		t.Fatal("no tops observed")
	}
}
