package combat

import (
	"math/rand"
	"testing"
)

func TestComputeDamagePhysical(t *testing.T) {
	d, err := ComputeDamage(DamageInput{
		Attack: 1000, Defense: 1000, SkillCoeff: 1, Type: Physical,
	})
	if err != nil {
		t.Fatal(err)
	}
	// 1000 * 1 * 1000/2000 = 500
	if d < 499 || d > 501 {
		t.Fatalf("got %v want ~500", d)
	}
}

func TestComputeDamageCritExpectation(t *testing.T) {
	d, err := ComputeDamage(DamageInput{
		Attack: 1000, Defense: 0, SkillCoeff: 1, Type: True,
		CritRate: 0.5, CritMult: 2,
	})
	if err != nil {
		t.Fatal(err)
	}
	// base 1000, E = 1000*(1+0.5*1)=1500
	if d < 1499 || d > 1501 {
		t.Fatalf("got %v want 1500", d)
	}
}

func TestSimulateReproducible(t *testing.T) {
	a := Unit{Name: "a", HP: 5000, Attack: 500, Defense: 100, CritRate: 0.1, Speed: 1}
	d := Unit{Name: "d", HP: 5000, Attack: 500, Defense: 100, CritRate: 0.1, Speed: 1}
	r1, err := Simulate(a, d, 400, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatal(err)
	}
	r2, err := Simulate(a, d, 400, rand.New(rand.NewSource(7)))
	if err != nil {
		t.Fatal(err)
	}
	if r1.AttackerWinRate != r2.AttackerWinRate {
		t.Fatalf("not reproducible: %v vs %v", r1.AttackerWinRate, r2.AttackerWinRate)
	}
	if r1.AttackerWinRate < 0.25 || r1.AttackerWinRate > 0.75 {
		t.Fatalf("mirror match win rate should be near 0.5, got %v", r1.AttackerWinRate)
	}
}

func TestEffectiveHP(t *testing.T) {
	ehp := EffectiveHP(1000, 100)
	if ehp != 2000 {
		t.Fatalf("got %v", ehp)
	}
}
