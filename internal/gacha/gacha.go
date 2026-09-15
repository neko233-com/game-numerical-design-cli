// Package gacha simulates drop tables, soft pity, hard pity, and multi-rarity
// banners. Focus is distribution experience, not just expected value.
package gacha

import (
	"fmt"
	"math"
	"math/rand"
	"sort"
)

// Rarity configures one rarity tier (e.g. SSR / SR / R).
type Rarity struct {
	Name string  `json:"name"`
	Rate float64 `json:"rate"` // base rate 0..1
	// SoftPityStart: pull index (1-based) after which rate increases each pull.
	// 0 disables soft pity.
	SoftPityStart int `json:"soft_pity_start"`
	// SoftPityStep added to rate per pull after start.
	SoftPityStep float64 `json:"soft_pity_step"`
	// HardPity: guaranteed at this pull count (inclusive). 0 = none.
	HardPity int `json:"hard_pity"`
	// FeaturedRate among this rarity (0..1) for rate-up.
	FeaturedRate float64 `json:"featured_rate"`
}

// Banner is a full pool: rarities should be listed high→low; rates need not sum to 1
// (remainder is "no high-rarity hit", typically the lowest tier absorbs via normalization).
type Banner struct {
	Name     string   `json:"name"`
	Rarities []Rarity `json:"rarities"`
	// GuaranteeFeatured: if true, losing 50/50 guarantees next featured (common CN-style).
	GuaranteeFeatured bool `json:"guarantee_featured"`
}

func (b Banner) validate() error {
	if len(b.Rarities) == 0 {
		return fmt.Errorf("banner has no rarities")
	}
	for i, r := range b.Rarities {
		if r.Rate < 0 || r.Rate > 1 {
			return fmt.Errorf("rarity[%d] %s rate out of range", i, r.Name)
		}
		if r.SoftPityStep < 0 {
			return fmt.Errorf("rarity[%d] soft_pity_step must be >= 0", i)
		}
	}
	return nil
}

// rateAt returns the effective drop rate for rarity r at pity counter `since`.
func rateAt(r Rarity, since int) float64 {
	rate := r.Rate
	if r.HardPity > 0 && since >= r.HardPity {
		return 1
	}
	if r.SoftPityStart > 0 && since+1 >= r.SoftPityStart {
		extra := r.SoftPityStep * float64(since+2-r.SoftPityStart)
		rate += extra
	}
	if rate > 1 {
		rate = 1
	}
	return rate
}

// PullResult of one pull.
type PullResult struct {
	Rarity    string `json:"rarity"`
	Featured  bool   `json:"featured"`
	PullIndex int    `json:"pull_index"`
	PityUsed  int    `json:"pity_used"` // pulls since last top rarity
}

// SessionResult is one player session of up to N pulls.
type SessionResult struct {
	Pulls         []PullResult `json:"pulls"`
	TopCount      int          `json:"top_count"`
	FeaturedCount int          `json:"featured_count"`
	FirstTopAt    int          `json:"first_top_at"`
}

// SimulateSession runs `maxPulls` pulls, stopping early if stopOnFirstTop.
func SimulateSession(b Banner, maxPulls int, stopOnFirstTop bool, rng *rand.Rand) (*SessionResult, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	if maxPulls <= 0 {
		return nil, fmt.Errorf("maxPulls must be > 0")
	}
	if rng == nil {
		rng = rand.New(rand.NewSource(1))
	}
	top := b.Rarities[0]
	sinceTop := 0
	// CN-style: featured guarantee flag
	lostFeatured := false
	res := &SessionResult{FirstTopAt: -1}

	for i := 1; i <= maxPulls; i++ {
		sinceTop++
		rate := rateAt(top, sinceTop)
		var hit PullResult
		if rng.Float64() < rate {
			// top rarity
			featured := false
			if top.FeaturedRate > 0 {
				if b.GuaranteeFeatured && lostFeatured {
					featured = true
					lostFeatured = false
				} else if rng.Float64() < top.FeaturedRate {
					featured = true
				} else {
					lostFeatured = true
				}
			}
			hit = PullResult{Rarity: top.Name, Featured: featured, PullIndex: i, PityUsed: sinceTop}
			sinceTop = 0
			res.TopCount++
			if featured {
				res.FeaturedCount++
			}
			if res.FirstTopAt < 0 {
				res.FirstTopAt = i
			}
		} else {
			// fall through lower rarities by remaining rate mass
			hit = rollLower(b.Rarities[1:], rng)
			hit.PullIndex = i
			hit.PityUsed = sinceTop
		}
		res.Pulls = append(res.Pulls, hit)
		if stopOnFirstTop && res.TopCount > 0 {
			break
		}
	}
	return res, nil
}

func rollLower(rs []Rarity, rng *rand.Rand) PullResult {
	if len(rs) == 0 {
		return PullResult{Rarity: "none"}
	}
	// weighted by Rate
	var sum float64
	for _, r := range rs {
		sum += r.Rate
	}
	if sum <= 0 {
		return PullResult{Rarity: rs[len(rs)-1].Name}
	}
	x := rng.Float64() * sum
	acc := 0.0
	for _, r := range rs {
		acc += r.Rate
		if x <= acc {
			return PullResult{Rarity: r.Name}
		}
	}
	return PullResult{Rarity: rs[len(rs)-1].Name}
}

// DistReport is the distribution experience summary over many sessions.
type DistReport struct {
	Banner          string  `json:"banner"`
	Sessions        int     `json:"sessions"`
	PullsPerSession int     `json:"pulls_per_session"`
	TopRateObserved float64 `json:"top_rate_observed"`
	FeaturedRateObs float64 `json:"featured_rate_observed"`
	AvgFirstTop     float64 `json:"avg_first_top"`
	P50FirstTop     float64 `json:"p50_first_top"`
	P90FirstTop     float64 `json:"p90_first_top"`
	P99FirstTop     float64 `json:"p99_first_top"`
	// Chance of at least one top within X pulls.
	CumTopCurve   map[int]float64 `json:"cum_top_curve"`  // key=pulls, value=P(top>=1 by N)
	PityHistogram map[int]int     `json:"pity_histogram"` // pity used on each top
}

// SimulateDistribution runs many sessions (no early stop) to characterize experience.
func SimulateDistribution(b Banner, pullsPerSession, sessions int, seed int64) (*DistReport, error) {
	if err := b.validate(); err != nil {
		return nil, err
	}
	if pullsPerSession <= 0 || sessions <= 0 {
		return nil, fmt.Errorf("pullsPerSession and sessions must be > 0")
	}
	rng := rand.New(rand.NewSource(seed))
	rep := &DistReport{
		Banner: b.Name, Sessions: sessions, PullsPerSession: pullsPerSession,
		CumTopCurve: map[int]float64{}, PityHistogram: map[int]int{},
	}
	firstTops := make([]int, 0, sessions)
	totalPulls, topHits, featHits := 0, 0, 0
	// cum: count sessions with first top <= k
	type firstAt struct{ first, total int }
	records := make([]firstAt, 0, sessions)

	for s := 0; s < sessions; s++ {
		sr, err := SimulateSession(b, pullsPerSession, false, rng)
		if err != nil {
			return nil, err
		}
		totalPulls += len(sr.Pulls)
		topHits += sr.TopCount
		featHits += sr.FeaturedCount
		for _, p := range sr.Pulls {
			if p.Rarity == b.Rarities[0].Name {
				rep.PityHistogram[p.PityUsed]++
			}
		}
		first := sr.FirstTopAt
		if first < 0 {
			first = 0 // no top
		}
		firstTops = append(firstTops, first)
		records = append(records, firstAt{first: first, total: len(sr.Pulls)})
	}

	if totalPulls > 0 {
		rep.TopRateObserved = float64(topHits) / float64(totalPulls)
		rep.FeaturedRateObs = float64(featHits) / float64(totalPulls)
	}

	// avg / percentiles among sessions that hit at least once
	var hitFirst []int
	var sumFirst float64
	for _, f := range firstTops {
		if f > 0 {
			hitFirst = append(hitFirst, f)
			sumFirst += float64(f)
		}
	}
	if len(hitFirst) > 0 {
		rep.AvgFirstTop = sumFirst / float64(len(hitFirst))
		sort.Ints(hitFirst)
		rep.P50FirstTop = percentile(hitFirst, 50)
		rep.P90FirstTop = percentile(hitFirst, 90)
		rep.P99FirstTop = percentile(hitFirst, 99)
	}

	// cumulative P(top>=1 by N)
	for _, n := range []int{10, 20, 30, 50, 74, 80, 90, 100, 160, 180, 200} {
		if n > pullsPerSession {
			continue
		}
		cnt := 0
		for _, r := range records {
			if r.first > 0 && r.first <= n {
				cnt++
			}
		}
		rep.CumTopCurve[n] = float64(cnt) / float64(sessions)
	}
	return rep, nil
}

func percentile(xs []int, p float64) float64 {
	if len(xs) == 0 {
		return 0
	}
	idx := int(math.Ceil(p/100*float64(len(xs)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(xs) {
		idx = len(xs) - 1
	}
	return float64(xs[idx])
}

// ExpectedPullsToTop computes analytical E[pulls] for a single rarity with soft/hard pity
// via absorbing Markov over pity counter 0..hardPity (or a cap if no hard pity).
func ExpectedPullsToTop(r Rarity) (float64, error) {
	cap := r.HardPity
	if cap <= 0 {
		// approximate with large cap; soft pity still applies
		cap = 1000
	}
	// dp[i] = expected remaining pulls from state i (i pulls since top)
	dp := make([]float64, cap+2)
	dp[cap] = 1 // hard pity guarantees next
	for i := cap - 1; i >= 0; i-- {
		p := rateAt(r, i+1) // next pull is attempt number i+1 since top... careful
		// At state i (i pulls since last top), next pull has rate rateAt(r, i) where since=i
		// rateAt uses since as "current pity count before this pull"? Let's use since=i meaning
		// the next pull is the (i+1)-th since top.
		p = rateAt(r, i)
		if p >= 1 {
			dp[i] = 1
			continue
		}
		if i+1 > cap {
			dp[i] = 1
			continue
		}
		dp[i] = 1 + (1-p)*dp[i+1]
	}
	return dp[0], nil
}

// SoftPitySuggest recommends a soft-pity schedule given base rate and hard pity.
func SoftPitySuggest(baseRate float64, hardPity int) (start int, step float64) {
	if hardPity <= 0 {
		hardPity = 90
	}
	// Common industry pattern: start soft pity ~0.6–0.7 of hard pity.
	start = int(float64(hardPity) * 0.65)
	if start < 1 {
		start = 1
	}
	// Step so that rate approaches 1 by hard pity-1.
	steps := hardPity - start
	if steps <= 0 {
		return start, 0
	}
	need := 1 - baseRate
	step = need / float64(steps)
	return start, step
}
