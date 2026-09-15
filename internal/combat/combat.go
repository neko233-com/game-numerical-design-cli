// Package combat implements attribute formulas, damage models, and a lightweight
// Monte-Carlo combat simulator for balance checks.
package combat

import (
	"fmt"
	"math"
	"math/rand"
)

// AttackType selects the damage formula family.
type AttackType string

const (
	// Physical: atk * skill * (atk / (atk + def))
	Physical AttackType = "physical"
	// Magical uses mdef similarly.
	Magical AttackType = "magical"
	// True ignores defense.
	True AttackType = "true"
	// Flat: skill * atk - def (floored)
	Flat AttackType = "flat"
)

// DamageInput is one damage roll configuration.
type DamageInput struct {
	Attack     float64
	Defense    float64
	SkillCoeff float64 // skill multiplier, default 1
	CritRate   float64 // 0..1
	CritMult   float64 // e.g. 1.5
	Type       AttackType
	// Mitigation is extra multiplier after formula (0.8 = 20% reduction).
	Mitigation float64
}

// ComputeDamage returns expected (mean) damage for the input.
func ComputeDamage(in DamageInput) (float64, error) {
	if in.Attack < 0 || in.Defense < 0 {
		return 0, fmt.Errorf("attack/defense must be >= 0")
	}
	if in.CritRate < 0 || in.CritRate > 1 {
		return 0, fmt.Errorf("critRate must be in [0,1]")
	}
	if in.SkillCoeff == 0 {
		in.SkillCoeff = 1
	}
	if in.CritMult == 0 {
		in.CritMult = 1.5
	}
	if in.Mitigation == 0 {
		in.Mitigation = 1
	}

	var base float64
	switch in.Type {
	case "", Physical:
		base = in.Attack * in.SkillCoeff * (in.Attack / (in.Attack + in.Defense + 1e-9))
	case Magical:
		base = in.Attack * in.SkillCoeff * (in.Attack / (in.Attack + in.Defense + 1e-9))
	case True:
		base = in.Attack * in.SkillCoeff
	case Flat:
		base = in.Attack*in.SkillCoeff - in.Defense
		if base < 0 {
			base = 0
		}
	default:
		return 0, fmt.Errorf("unknown attack type %q", in.Type)
	}
	// Expected value with crit: (1-cr)*base + cr*base*critMult
	exp := base * (1 + in.CritRate*(in.CritMult-1))
	return exp * in.Mitigation, nil
}

// Unit is a combat participant for the simulator.
type Unit struct {
	Name       string
	HP         float64
	Attack     float64
	Defense    float64
	CritRate   float64
	CritMult   float64
	Speed      float64 // turns per "tick window"; higher acts more often
	SkillCoeff float64
	Type       AttackType
	// Acc (accuracy 0..1) vs target Evasion for hit roll.
	Acc     float64
	Evasion float64
}

func (u Unit) normalized() Unit {
	if u.CritMult == 0 {
		u.CritMult = 1.5
	}
	if u.SkillCoeff == 0 {
		u.SkillCoeff = 1
	}
	if u.Speed == 0 {
		u.Speed = 1
	}
	if u.Acc == 0 {
		u.Acc = 1
	}
	if u.Type == "" {
		u.Type = Physical
	}
	return u
}

// SimResult aggregates Monte-Carlo outcomes.
type SimResult struct {
	N               int         `json:"n"`
	AttackerWinRate float64     `json:"attacker_win_rate"`
	AvgTurns        float64     `json:"avg_turns"`
	AvgDamageDealt  float64     `json:"avg_damage_dealt"`
	AvgDamageTaken  float64     `json:"avg_damage_taken"`
	P50Turns        float64     `json:"p50_turns"`
	P90Turns        float64     `json:"p90_turns"`
	TurnHistogram   map[int]int `json:"turn_histogram"`
}

// Simulate runs n independent 1v1 fights attacker-first, returns stats.
// Random source is provided for reproducibility (pass rand.New(rand.NewSource(seed))).
func Simulate(attacker, defender Unit, n int, rng *rand.Rand) (*SimResult, error) {
	if n <= 0 {
		return nil, fmt.Errorf("n must be > 0")
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	a := attacker.normalized()
	d := defender.normalized()

	res := &SimResult{N: n, TurnHistogram: map[int]int{}}
	turnsList := make([]int, 0, n)
	var winSum, dmgDealtSum, dmgTakenSum float64

	for i := 0; i < n; i++ {
		ahp, dhp := a.HP, d.HP
		aAcc, dAcc := 0.0, 0.0 // action charge
		turns := 0
		var dealt, taken float64
		maxTurns := 1000
		for ahp > 0 && dhp > 0 && turns < maxTurns {
			turns++
			aAcc += a.Speed
			dAcc += d.Speed
			// attacker acts if charge >= 1
			if aAcc >= 1 && ahp > 0 && dhp > 0 {
				aAcc--
				hit := rng.Float64() < a.Acc*(1-d.Evasion)
				if hit {
					crit := rng.Float64() < a.CritRate
					mult := 1.0
					if crit {
						mult = a.CritMult
					}
					base, _ := ComputeDamage(DamageInput{
						Attack: a.Attack, Defense: d.Defense,
						SkillCoeff: a.SkillCoeff, Type: a.Type,
						CritRate: 0, // crit already applied via mult
					})
					dmg := base * mult
					dhp -= dmg
					dealt += dmg
				}
			}
			if dAcc >= 1 && ahp > 0 && dhp > 0 {
				dAcc--
				hit := rng.Float64() < d.Acc*(1-a.Evasion)
				if hit {
					crit := rng.Float64() < d.CritRate
					mult := 1.0
					if crit {
						mult = d.CritMult
					}
					base, _ := ComputeDamage(DamageInput{
						Attack: d.Attack, Defense: a.Defense,
						SkillCoeff: d.SkillCoeff, Type: d.Type,
					})
					dmg := base * mult
					ahp -= dmg
					taken += dmg
				}
			}
		}
		if dhp <= 0 && ahp > 0 {
			winSum++
		}
		dmgDealtSum += dealt
		dmgTakenSum += taken
		turnsList = append(turnsList, turns)
		res.TurnHistogram[turns]++
	}

	res.AttackerWinRate = winSum / float64(n)
	res.AvgDamageDealt = dmgDealtSum / float64(n)
	res.AvgDamageTaken = dmgTakenSum / float64(n)
	var sum float64
	for _, t := range turnsList {
		sum += float64(t)
	}
	res.AvgTurns = sum / float64(n)
	res.P50Turns = percentileInt(turnsList, 50)
	res.P90Turns = percentileInt(turnsList, 90)
	return res, nil
}

func percentileInt(xs []int, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	sorted := append([]int(nil), xs...)
	// simple insertion for small n; use sort for generality
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j] < sorted[j-1]; j-- {
			sorted[j], sorted[j-1] = sorted[j-1], sorted[j]
		}
	}
	idx := int(math.Ceil(p/100*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return float64(sorted[idx])
}

// EffectiveHP returns HP / (1 - mitigation) style tankiness proxy using physical formula inverse.
func EffectiveHP(hp, def float64) float64 {
	// For physical formula, damage multiplier vs pure true is atk/(atk+def).
	// Effective HP in "true-damage units" is roughly HP * (1 + def/atk_ref).
	// We expose a def-independent form: HP * (1 + def/K) with K as reference atk.
	const k = 100
	return hp * (1 + def/k)
}
