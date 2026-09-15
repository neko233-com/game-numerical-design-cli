package cli

import (
	"flag"
	"fmt"
	"sort"
	"strconv"

	"github.com/neko233-com/game-numerical-design-cli/internal/gacha"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
)

func (a *App) cmdGacha(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd gacha <sim|expect|suggest> [flags]

sim — Monte-Carlo distribution (not just EV)
  --rate 0.006          top rarity base rate
  --hard 90             hard pity (0 = none)
  --soft-start 59       soft pity start (1-based pull count)
  --soft-step 0.03      rate added per pull after soft start
  --pulls 80 --sessions 10000 --seed 1 --format table|json
  --featured-rate 0.5   rate-up among top rarity
  --guarantee           CN-style guarantee after losing 50/50

expect — analytical E[pulls] to top (single rarity)
  same rate/soft/hard flags

suggest — recommend soft pity schedule
  --rate 0.006 --hard 90
`)
		return 0
	}
	switch args[0] {
	case "sim":
		return a.gachaSim(args[1:])
	case "expect":
		return a.gachaExpect(args[1:])
	case "suggest":
		return a.gachaSuggest(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown gacha subcommand %q\n", args[0])
		return 2
	}
}

type gachaFlags struct {
	rate, softStep, featRate     float64
	hard, softStart, pulls, sess int
	seed                         int64
	format                       string
	guarantee                    bool
}

func (a *App) bindGachaFlags(fs *flag.FlagSet) *gachaFlags {
	g := &gachaFlags{}
	fs.Float64Var(&g.rate, "rate", 0.006, "top rarity base rate")
	fs.IntVar(&g.hard, "hard", 90, "hard pity")
	fs.IntVar(&g.softStart, "soft-start", 59, "soft pity start")
	fs.Float64Var(&g.softStep, "soft-step", 0.03, "soft pity step")
	fs.IntVar(&g.pulls, "pulls", 80, "pulls per session")
	fs.IntVar(&g.sess, "sessions", 10000, "sessions")
	fs.Int64Var(&g.seed, "seed", 1, "RNG seed")
	fs.StringVar(&g.format, "format", "table", "table|json")
	fs.Float64Var(&g.featRate, "featured-rate", 0, "featured rate among top")
	fs.BoolVar(&g.guarantee, "guarantee", false, "guarantee featured after off-banner")
	return g
}

func (a *App) gachaSim(args []string) int {
	fs := a.newFlagSet("gacha sim")
	g := a.bindGachaFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	b := gacha.Banner{
		Name: "cli-banner",
		Rarities: []gacha.Rarity{{
			Name: "top", Rate: g.rate,
			SoftPityStart: g.softStart, SoftPityStep: g.softStep,
			HardPity: g.hard, FeaturedRate: g.featRate,
		}, {
			Name: "low", Rate: 1 - g.rate,
		}},
		GuaranteeFeatured: g.guarantee,
	}
	rep, err := gacha.SimulateDistribution(b, g.pulls, g.sess, g.seed)
	if err != nil {
		return a.fail(err)
	}
	if g.format == "json" {
		return exitJSON(a, rep)
	}
	kv := [][2]string{
		{"banner", rep.Banner},
		{"sessions", strconv.Itoa(rep.Sessions)},
		{"pulls/session", strconv.Itoa(rep.PullsPerSession)},
		{"observed top rate", fmt.Sprintf("%.3f%%", rep.TopRateObserved*100)},
		{"observed featured", fmt.Sprintf("%.3f%%", rep.FeaturedRateObs*100)},
		{"avg first top (hit)", report.Ftoa(rep.AvgFirstTop)},
		{"P50 first top", report.Ftoa(rep.P50FirstTop)},
		{"P90 first top", report.Ftoa(rep.P90FirstTop)},
		{"P99 first top", report.Ftoa(rep.P99FirstTop)},
	}
	keys := make([]int, 0, len(rep.CumTopCurve))
	for k := range rep.CumTopCurve {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	for _, k := range keys {
		kv = append(kv, [2]string{
			fmt.Sprintf("P(top by %d)", k),
			fmt.Sprintf("%.1f%%", rep.CumTopCurve[k]*100),
		})
	}
	// top pity buckets
	var buckets [][2]string
	for pity, cnt := range rep.PityHistogram {
		buckets = append(buckets, [2]string{strconv.Itoa(pity), strconv.Itoa(cnt)})
	}
	sort.Slice(buckets, func(i, j int) bool {
		pi, _ := strconv.Atoi(buckets[i][0])
		pj, _ := strconv.Atoi(buckets[j][0])
		return pi < pj
	})
	if len(buckets) > 0 {
		kv = append(kv, [2]string{"--- pity hist (top) ---", ""})
		// show only every 10th + hard
		for _, b := range buckets {
			p, _ := strconv.Atoi(b[0])
			if p%10 == 0 || p == g.hard || p == 1 {
				kv = append(kv, [2]string{"  pity " + b[0], b[1]})
			}
		}
	}
	_ = report.KeyValue(a.Stdout, kv)
	return 0
}

func (a *App) gachaExpect(args []string) int {
	fs := a.newFlagSet("gacha expect")
	g := a.bindGachaFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	r := gacha.Rarity{
		Name: "top", Rate: g.rate,
		SoftPityStart: g.softStart, SoftPityStep: g.softStep, HardPity: g.hard,
	}
	exp, err := gacha.ExpectedPullsToTop(r)
	if err != nil {
		return a.fail(err)
	}
	if g.format == "json" {
		return exitJSON(a, map[string]float64{"expected_pulls": exp})
	}
	fmt.Fprintf(a.Stdout, "E[pulls to top] ≈ %s\n", report.Ftoa(exp))
	return 0
}

func (a *App) gachaSuggest(args []string) int {
	fs := a.newFlagSet("gacha suggest")
	rate := fs.Float64("rate", 0.006, "base rate")
	hard := fs.Int("hard", 90, "hard pity")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	start, step := gacha.SoftPitySuggest(*rate, *hard)
	_ = report.KeyValue(a.Stdout, [][2]string{
		{"base rate", report.Ftoa(*rate)},
		{"hard pity", strconv.Itoa(*hard)},
		{"suggest soft start", strconv.Itoa(start)},
		{"suggest soft step", report.Ftoa(step)},
		{"note", "start≈0.65×hard；step 使 hard-1 时 rate 接近 100%"},
	})
	return 0
}
