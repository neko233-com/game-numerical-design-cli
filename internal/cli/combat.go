package cli

import (
	"fmt"
	"math/rand"

	"github.com/neko233-com/game-numerical-design-cli/internal/combat"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
)

func (a *App) cmdCombat(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd combat <dmg|simulate|ehp> [flags]

dmg — expected damage
  --atk 1000 --def 300 --skill 1 --crit-rate 0 --crit-mult 1.5
  --type physical|true|flat --mitigation 1

simulate — Monte-Carlo 1v1
  attacker: --a-hp --a-atk --a-def --a-crit-rate --a-crit-mult --a-speed --a-skill --a-acc --a-evasion
  defender: same with d- prefix
  --n 2000 --seed 1 --format table|json

ehp — effective HP proxy
  --hp 5000 --def 400
`)
		return 0
	}
	switch args[0] {
	case "dmg":
		return a.combatDmg(args[1:])
	case "simulate":
		return a.combatSim(args[1:])
	case "ehp":
		return a.combatEHP(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown combat subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) combatDmg(args []string) int {
	fs := a.newFlagSet("combat dmg")
	var (
		in     combat.DamageInput
		format = fs.String("format", "table", "table|json")
	)
	fs.Float64Var(&in.Attack, "atk", 1000, "attack")
	fs.Float64Var(&in.Defense, "def", 300, "defense")
	fs.Float64Var(&in.SkillCoeff, "skill", 1, "skill coefficient")
	fs.Float64Var(&in.CritRate, "crit-rate", 0, "crit rate 0..1")
	fs.Float64Var(&in.CritMult, "crit-mult", 1.5, "crit multiplier")
	fs.Float64Var(&in.Mitigation, "mitigation", 1, "extra mitigation multiplier")
	typ := fs.String("type", "physical", "physical|true|flat")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	in.Type = combat.AttackType(*typ)
	dmg, err := combat.ComputeDamage(in)
	if err != nil {
		return a.fail(err)
	}
	// also show non-crit base for reference
	noCrit := in
	noCrit.CritRate = 0
	base, _ := combat.ComputeDamage(noCrit)
	if *format == "json" {
		return exitJSON(a, map[string]float64{"expected": dmg, "base_no_crit": base})
	}
	_ = report.KeyValue(a.Stdout, [][2]string{
		{"type", string(in.Type)},
		{"atk", report.Ftoa(in.Attack)},
		{"def", report.Ftoa(in.Defense)},
		{"skill", report.Ftoa(in.SkillCoeff)},
		{"crit", fmt.Sprintf("%.0f%% × %.2f", in.CritRate*100, in.CritMult)},
		{"base(no crit)", report.Ftoa(base)},
		{"expected dmg", report.Ftoa(dmg)},
	})
	return 0
}

func (a *App) combatSim(args []string) int {
	fs := a.newFlagSet("combat simulate")
	var (
		ahp, aatk, adef, acr, acm, aspd, askill, aacc, aeva float64
		dhp, datk, ddef, dcr, dcm, dspd, dskill, dacc, deva float64
		n                                                   int
		seed                                                int64
		format                                              string
	)
	fs.Float64Var(&ahp, "a-hp", 5000, "attacker HP")
	fs.Float64Var(&aatk, "a-atk", 800, "attacker ATK")
	fs.Float64Var(&adef, "a-def", 200, "attacker DEF")
	fs.Float64Var(&acr, "a-crit-rate", 0.2, "attacker crit rate")
	fs.Float64Var(&acm, "a-crit-mult", 1.5, "attacker crit mult")
	fs.Float64Var(&aspd, "a-speed", 1, "attacker speed")
	fs.Float64Var(&askill, "a-skill", 1, "attacker skill coeff")
	fs.Float64Var(&aacc, "a-acc", 1, "attacker accuracy")
	fs.Float64Var(&aeva, "a-evasion", 0, "attacker evasion")

	fs.Float64Var(&dhp, "d-hp", 4000, "defender HP")
	fs.Float64Var(&datk, "d-atk", 600, "defender ATK")
	fs.Float64Var(&ddef, "d-def", 250, "defender DEF")
	fs.Float64Var(&dcr, "d-crit-rate", 0.1, "defender crit rate")
	fs.Float64Var(&dcm, "d-crit-mult", 1.5, "defender crit mult")
	fs.Float64Var(&dspd, "d-speed", 1, "defender speed")
	fs.Float64Var(&dskill, "d-skill", 1, "defender skill coeff")
	fs.Float64Var(&dacc, "d-acc", 1, "defender accuracy")
	fs.Float64Var(&deva, "d-evasion", 0, "defender evasion")

	fs.IntVar(&n, "n", 2000, "simulations")
	fs.Int64Var(&seed, "seed", 1, "RNG seed")
	fs.StringVar(&format, "format", "table", "table|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	atk := combat.Unit{
		Name: "attacker", HP: ahp, Attack: aatk, Defense: adef,
		CritRate: acr, CritMult: acm, Speed: aspd, SkillCoeff: askill,
		Acc: aacc, Evasion: aeva,
	}
	def := combat.Unit{
		Name: "defender", HP: dhp, Attack: datk, Defense: ddef,
		CritRate: dcr, CritMult: dcm, Speed: dspd, SkillCoeff: dskill,
		Acc: dacc, Evasion: deva,
	}
	rng := rand.New(rand.NewSource(seed))
	res, err := combat.Simulate(atk, def, n, rng)
	if err != nil {
		return a.fail(err)
	}
	if format == "json" {
		return exitJSON(a, res)
	}
	_ = report.KeyValue(a.Stdout, [][2]string{
		{"n", fmt.Sprint(res.N)},
		{"attacker win rate", fmt.Sprintf("%.1f%%", res.AttackerWinRate*100)},
		{"avg turns", report.Ftoa(res.AvgTurns)},
		{"p50 turns", report.Ftoa(res.P50Turns)},
		{"p90 turns", report.Ftoa(res.P90Turns)},
		{"avg dmg dealt", report.Ftoa(res.AvgDamageDealt)},
		{"avg dmg taken", report.Ftoa(res.AvgDamageTaken)},
	})
	return 0
}

func (a *App) combatEHP(args []string) int {
	fs := a.newFlagSet("combat ehp")
	hp := fs.Float64("hp", 5000, "HP")
	def := fs.Float64("def", 400, "DEF")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	fmt.Fprintf(a.Stdout, "EHP ≈ %s  (HP * (1 + DEF/100))\n",
		report.Ftoa(combat.EffectiveHP(*hp, *def)))
	return 0
}
