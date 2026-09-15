// Package simulator runs many 1v1 battles in parallel with bounded memory,
// one log file per battle, cleanup of prior runs, and queryable history.
package simulator

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neko233-com/game-numerical-design-cli/internal/combat"
)

const (
	// MaxConcurrent is the hard cap on in-flight battles (memory guard).
	MaxConcurrent = 1000
	// DefaultDir is the on-disk root for simulation runs.
	DefaultDir = ".gnd/sim"
)

// Unit is a combat participant for the simulator CLI/scripts.
type Unit struct {
	Name      string             `json:"name"`
	ID        string             `json:"id,omitempty"`
	HP        float64            `json:"hp"`
	Attack    float64            `json:"atk"`
	Defense   float64            `json:"def"`
	CritRate  float64            `json:"crit_rate"`
	CritDMG   float64            `json:"crit_dmg"` // 0.5 = +50%
	Speed     float64            `json:"spd"`      // raw, 100 → 1.0 action/tick
	SkillMult float64            `json:"skill_mult"`
	Evasion   float64            `json:"evasion"`
	Accuracy  float64            `json:"accuracy"`
	Extra     map[string]float64 `json:"extra,omitempty"`
}

// Request configures one simulation run (N battles).
type Request struct {
	Name     string `json:"name"`
	Attacker Unit   `json:"attacker"`
	Defender Unit   `json:"defender"`
	N        int    `json:"n"`
	Seed     int64  `json:"seed"`
	Workers  int    `json:"workers"`
	// Root directory; empty → DefaultDir.
	Root string `json:"root,omitempty"`
	// ClearPrior removes previous runs under Root before starting (default true).
	ClearPrior *bool `json:"clear_prior,omitempty"`
	// LogEvery writes a log file every k battles (1 = every battle). Default 1.
	LogEvery int `json:"log_every,omitempty"`
	// Tags free-form labels for query.
	Tags map[string]string `json:"tags,omitempty"`
}

// BattleLog is the per-battle record written to disk.
type BattleLog struct {
	RunID      string  `json:"run_id"`
	BattleID   int     `json:"battle_id"`
	Seed       int64   `json:"seed"`
	Attacker   Unit    `json:"attacker"`
	Defender   Unit    `json:"defender"`
	Winner     string  `json:"winner"` // attacker | defender | draw
	Turns      int     `json:"turns"`
	DmgDealt   float64 `json:"dmg_dealt"`
	DmgTaken   float64 `json:"dmg_taken"`
	AttackerHP float64 `json:"attacker_hp_left"`
	DefenderHP float64 `json:"defender_hp_left"`
	DurationMS int64   `json:"duration_ms"`
	Error      string  `json:"error,omitempty"`
}

// RunManifest summarizes a run and is written once at the end.
type RunManifest struct {
	RunID         string            `json:"run_id"`
	Name          string            `json:"name"`
	CreatedAt     time.Time         `json:"created_at"`
	N             int               `json:"n"`
	Workers       int               `json:"workers"`
	Seed          int64             `json:"seed"`
	Attacker      Unit              `json:"attacker"`
	Defender      Unit              `json:"defender"`
	Tags          map[string]string `json:"tags,omitempty"`
	WinRate       float64           `json:"attacker_win_rate"`
	DrawRate      float64           `json:"draw_rate"`
	AvgTurns      float64           `json:"avg_turns"`
	P50Turns      float64           `json:"p50_turns"`
	P90Turns      float64           `json:"p90_turns"`
	AvgDmgDealt   float64           `json:"avg_dmg_dealt"`
	AvgDmgTaken   float64           `json:"avg_dmg_taken"`
	LogCount      int               `json:"log_count"`
	LogDir        string            `json:"log_dir"`
	DurationMS    int64             `json:"duration_ms"`
	TurnHistogram map[string]int    `json:"turn_histogram"`
}

// Runner executes requests against a root directory.
type Runner struct {
	Root string
}

// NewRunner creates a runner. root empty → DefaultDir.
func NewRunner(root string) *Runner {
	if root == "" {
		root = DefaultDir
	}
	return &Runner{Root: root}
}

func (r *Runner) clearPrior() error {
	entries, err := os.ReadDir(r.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(r.Root, 0o755)
		}
		return err
	}
	for _, e := range entries {
		// only remove run_* dirs and index files we own
		name := e.Name()
		if strings.HasPrefix(name, "run_") || name == "index.json" {
			if err := os.RemoveAll(filepath.Join(r.Root, name)); err != nil {
				return err
			}
		}
	}
	return os.MkdirAll(r.Root, 0o755)
}

// Run executes req.N battles with bounded workers. Always clears prior runs first
// unless ClearPrior is explicitly false.
func (r *Runner) Run(req Request) (*RunManifest, error) {
	if req.N <= 0 {
		return nil, fmt.Errorf("n must be > 0")
	}
	if req.N > 1_000_000 {
		return nil, fmt.Errorf("n=%d exceeds hard cap 1e6", req.N)
	}
	clear := true
	if req.ClearPrior != nil {
		clear = *req.ClearPrior
	}
	if clear {
		if err := r.clearPrior(); err != nil {
			return nil, fmt.Errorf("clear prior: %w", err)
		}
	} else if err := os.MkdirAll(r.Root, 0o755); err != nil {
		return nil, err
	}

	workers := req.Workers
	if workers <= 0 {
		workers = runtime.NumCPU() * 4
	}
	if workers > MaxConcurrent {
		workers = MaxConcurrent
	}
	if workers > req.N {
		workers = req.N
	}

	logEvery := req.LogEvery
	if logEvery <= 0 {
		logEvery = 1
	}

	runID := fmt.Sprintf("run_%s_%d", sanitize(req.Name), time.Now().UnixMilli())
	if req.Name == "" {
		runID = fmt.Sprintf("run_%d", time.Now().UnixMilli())
	}
	logDir := filepath.Join(r.Root, runID, "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}

	started := time.Now()
	aUnit := toCombatUnit(req.Attacker)
	dUnit := toCombatUnit(req.Defender)

	jobs := make(chan int)
	var wg sync.WaitGroup
	// Stream aggregates only — do NOT keep all BattleLogs in RAM.
	var (
		winCount, drawCount atomic.Int64
		turnSum             atomic.Int64
		logCount            atomic.Int64
	)
	var (
		mu       sync.Mutex
		dmgDealt float64
		dmgTaken float64
		turns    []int
		hist     = map[int]int{}
	)

	// Bound in-flight battles to workers (≤ MaxConcurrent).
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func(_ int) {
			defer wg.Done()
			rngBase := req.Seed
			if rngBase == 0 {
				rngBase = time.Now().UnixNano()
			}
			for id := range jobs {
				bStart := time.Now()
				seed := rngBase + int64(id)*9973
				rng := rand.New(rand.NewSource(seed))
				fr, err := combat.SingleFight(aUnit, dUnit, rng)
				bl := BattleLog{
					RunID: runID, BattleID: id, Seed: seed,
					Attacker: req.Attacker, Defender: req.Defender,
					DurationMS: time.Since(bStart).Milliseconds(),
				}
				win := false
				draw := false
				if err != nil {
					bl.Error = err.Error()
					bl.Winner = "error"
				} else {
					bl.Winner = fr.Winner
					bl.Turns = fr.Turns
					bl.DmgDealt = fr.DmgDealt
					bl.DmgTaken = fr.DmgTaken
					bl.AttackerHP = fr.AttackerHP
					bl.DefenderHP = fr.DefenderHP
					win = fr.Winner == "attacker"
					draw = fr.Winner == "draw"
				}
				if bl.Winner == "" {
					bl.Winner = "defender"
				}

				// write log file (every logEvery battles, always write 1 and last)
				if id%logEvery == 0 || id == req.N {
					if err := writeBattleLog(logDir, bl); err == nil {
						logCount.Add(1)
					}
				}

				if win {
					winCount.Add(1)
				}
				if draw {
					drawCount.Add(1)
				}
				turnSum.Add(int64(bl.Turns))
				mu.Lock()
				dmgDealt += bl.DmgDealt
				dmgTaken += bl.DmgTaken
				turns = append(turns, bl.Turns)
				hist[bl.Turns]++
				mu.Unlock()
			}
		}(w)
	}

	for i := 1; i <= req.N; i++ {
		jobs <- i
	}
	close(jobs)
	wg.Wait()

	n := float64(req.N)
	man := &RunManifest{
		RunID: runID, Name: req.Name, CreatedAt: started,
		N: req.N, Workers: workers, Seed: req.Seed,
		Attacker: req.Attacker, Defender: req.Defender, Tags: req.Tags,
		WinRate:       float64(winCount.Load()) / n,
		DrawRate:      float64(drawCount.Load()) / n,
		AvgTurns:      float64(turnSum.Load()) / n,
		AvgDmgDealt:   dmgDealt / n,
		AvgDmgTaken:   dmgTaken / n,
		LogCount:      int(logCount.Load()),
		LogDir:        logDir,
		DurationMS:    time.Since(started).Milliseconds(),
		TurnHistogram: map[string]int{},
	}
	sort.Ints(turns)
	if len(turns) > 0 {
		man.P50Turns = float64(percentile(turns, 50))
		man.P90Turns = float64(percentile(turns, 90))
	}
	for t, c := range hist {
		man.TurnHistogram[fmt.Sprint(t)] = c
	}

	if err := writeManifest(filepath.Join(r.Root, runID, "manifest.json"), man); err != nil {
		return nil, err
	}
	if err := r.appendIndex(man); err != nil {
		return nil, err
	}
	return man, nil
}

func toCombatUnit(u Unit) combat.Unit {
	speed := u.Speed
	if speed <= 0 {
		speed = 100
	}
	cu := combat.Unit{
		Name: u.Name, HP: u.HP, Attack: u.Attack, Defense: u.Defense,
		CritRate: u.CritRate, CritMult: 1 + u.CritDMG,
		Speed: speed / 100, SkillCoeff: u.SkillMult,
		Acc: u.Accuracy, Evasion: u.Evasion,
	}
	if cu.SkillCoeff == 0 {
		cu.SkillCoeff = 1
	}
	if cu.CritMult == 1 {
		cu.CritMult = 1.5
		if u.CritDMG == 0 {
			cu.CritMult = 1.5
		}
	}
	if u.CritDMG > 0 {
		cu.CritMult = 1 + u.CritDMG
	}
	return cu
}

func sanitize(s string) string {
	if s == "" {
		return "sim"
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

func writeBattleLog(dir string, bl BattleLog) error {
	path := filepath.Join(dir, fmt.Sprintf("battle_%05d.json", bl.BattleID))
	data, err := json.MarshalIndent(bl, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

func writeManifest(path string, m *RunManifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// IndexEntry is one line in the queryable history index.
type IndexEntry struct {
	RunID     string            `json:"run_id"`
	Name      string            `json:"name"`
	CreatedAt time.Time         `json:"created_at"`
	N         int               `json:"n"`
	WinRate   float64           `json:"attacker_win_rate"`
	AvgTurns  float64           `json:"avg_turns"`
	Tags      map[string]string `json:"tags,omitempty"`
	Manifest  string            `json:"manifest"`
	LogCount  int               `json:"log_count"`
}

func (r *Runner) indexPath() string {
	return filepath.Join(r.Root, "index.json")
}

func (r *Runner) appendIndex(m *RunManifest) error {
	list, err := r.LoadIndex()
	if err != nil {
		list = nil
	}
	list = append(list, IndexEntry{
		RunID: m.RunID, Name: m.Name, CreatedAt: m.CreatedAt,
		N: m.N, WinRate: m.WinRate, AvgTurns: m.AvgTurns,
		Tags: m.Tags, Manifest: filepath.Join(m.RunID, "manifest.json"),
		LogCount: m.LogCount,
	})
	// keep last 200
	if len(list) > 200 {
		list = list[len(list)-200:]
	}
	data, err := json.MarshalIndent(list, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(r.indexPath(), append(data, '\n'), 0o644)
}

// LoadIndex returns run history (oldest→newest).
func (r *Runner) LoadIndex() ([]IndexEntry, error) {
	data, err := os.ReadFile(r.indexPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var list []IndexEntry
	if err := json.Unmarshal(data, &list); err != nil {
		return nil, err
	}
	return list, nil
}

// GetManifest loads one run's manifest.
func (r *Runner) GetManifest(runID string) (*RunManifest, error) {
	data, err := os.ReadFile(filepath.Join(r.Root, runID, "manifest.json"))
	if err != nil {
		return nil, err
	}
	var m RunManifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}

// ListBattles returns log filenames for a run (sorted).
func (r *Runner) ListBattles(runID string) ([]string, error) {
	dir := filepath.Join(r.Root, runID, "logs")
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".json") {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// GetBattle loads one battle log.
func (r *Runner) GetBattle(runID string, battleID int) (*BattleLog, error) {
	path := filepath.Join(r.Root, runID, "logs", fmt.Sprintf("battle_%05d.json", battleID))
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var bl BattleLog
	if err := json.Unmarshal(data, &bl); err != nil {
		return nil, err
	}
	return &bl, nil
}

// Clear removes all run_* dirs and index.
func (r *Runner) Clear() error {
	return r.clearPrior()
}

// Query filters history.
type Query struct {
	NameContains string
	TagKey       string
	TagValue     string
	MinWinRate   float64
	Limit        int
}

// Search filters the index newest-first.
func (r *Runner) Search(q Query) ([]IndexEntry, error) {
	list, err := r.LoadIndex()
	if err != nil {
		return nil, err
	}
	// newest first
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
	var out []IndexEntry
	for _, e := range list {
		if q.NameContains != "" && !strings.Contains(strings.ToLower(e.Name), strings.ToLower(q.NameContains)) {
			continue
		}
		if q.TagKey != "" {
			if e.Tags == nil || e.Tags[q.TagKey] != q.TagValue && q.TagValue != "" {
				if q.TagValue == "" {
					if _, ok := e.Tags[q.TagKey]; !ok {
						continue
					}
				} else {
					continue
				}
			}
		}
		if q.MinWinRate > 0 && e.WinRate < q.MinWinRate {
			continue
		}
		out = append(out, e)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

func percentile(sorted []int, p float64) int {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(p/100*float64(len(sorted))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

// FromMap builds a Unit from a JS/JSON object.
func FromMap(m map[string]any) Unit {
	get := func(k string) float64 {
		if v, ok := m[k]; ok {
			switch x := v.(type) {
			case float64:
				return x
			case int:
				return float64(x)
			case int64:
				return float64(x)
			case string:
				var f float64
				fmt.Sscanf(x, "%g", &f)
				return f
			}
		}
		return 0
	}
	getS := func(k string) string {
		if v, ok := m[k]; ok {
			return fmt.Sprint(v)
		}
		return ""
	}
	u := Unit{
		Name: getS("name"), ID: getS("id"),
		HP: get("hp"), Attack: get("atk"), Defense: get("def"),
		CritRate: get("crit_rate"), CritDMG: get("crit_dmg"),
		Speed: get("spd"), SkillMult: get("skill_mult"),
		Evasion: get("evasion"), Accuracy: get("accuracy"),
	}
	if u.Accuracy == 0 {
		u.Accuracy = 1
	}
	if u.Speed == 0 {
		u.Speed = 100
	}
	return u
}
