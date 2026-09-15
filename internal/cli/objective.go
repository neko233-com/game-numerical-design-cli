package cli

import (
	"fmt"
	"math/rand"

	"github.com/neko233-com/game-numerical-design-cli/internal/balance"
	"github.com/neko233-com/game-numerical-design-cli/internal/combat"
)

// makeObjective builds a scalar objective. Higher is better.
// "winrate": negative |winRate - 0.5| so sensitivity ranks params that move balance.
func makeObjective(name string, opp map[string]float64, n int, seed int64) (balance.Objective, error) {
	switch name {
	case "winrate":
		if n <= 0 {
			n = 300
		}
		return func(params map[string]float64) float64 {
			get := func(k string, def float64) float64 {
				if v, ok := params[k]; ok {
					return v
				}
				return def
			}
			atk := combat.Unit{
				Name: "A", HP: get("hp", 5000), Attack: get("atk", 800),
				Defense: get("def", 200), CritRate: get("cr", 0.2),
				CritMult: get("cm", 1.5), Speed: get("speed", 1),
				SkillCoeff: get("skill", 1),
			}
			def := combat.Unit{
				Name: "D", HP: opp["d_hp"], Attack: opp["d_atk"],
				Defense: opp["d_def"], CritRate: opp["d_cr"],
				CritMult: opp["d_cm"], Speed: opp["d_speed"],
			}
			// fixed seed per call for comparability across nearby params
			rng := rand.New(rand.NewSource(seed))
			res, err := combat.Simulate(atk, def, n, rng)
			if err != nil {
				return 0
			}
			// score = -|wr - 0.5|  (closer to 50% is better)
			return -abs(res.AttackerWinRate - 0.5)
		}, nil
	default:
		return nil, fmt.Errorf("unknown objective %q (available: winrate)", name)
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
