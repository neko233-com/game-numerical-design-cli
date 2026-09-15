// Package calllog records CLI invocations for agent troubleshooting.
// Ring/LRU cap is 5MB on disk; older entries drop automatically.
package calllog

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// MaxBytes is the hard on-disk budget for call logs (5 MiB).
	MaxBytes = 5 * 1024 * 1024
	// TrimTarget leaves headroom after rotation (4 MiB).
	TrimTarget = 4 * 1024 * 1024
	// DefaultDir relative to cwd.
	DefaultDir = ".gnd/logs"
	// FileName is the JSONL store.
	FileName = "calls.jsonl"
	// StdoutTailCap bounds captured output per side.
	StdoutTailCap = 4096
)

// Entry is one CLI invocation.
type Entry struct {
	ID         string            `json:"id"`
	TS         time.Time         `json:"ts"`
	Cmd        string            `json:"cmd"`
	Argv       []string          `json:"argv"`
	Cwd        string            `json:"cwd"`
	Exit       int               `json:"exit"`
	DurationMS int64             `json:"duration_ms"`
	StdoutTail string            `json:"stdout_tail,omitempty"`
	StderrTail string            `json:"stderr_tail,omitempty"`
	Env        map[string]string `json:"env,omitempty"`
	// Flags of interest extracted for quick search
	DirFlag  string `json:"dir_flag,omitempty"`
	GoalFlag string `json:"goal_flag,omitempty"`
	Preset   string `json:"preset,omitempty"`
}

// Options for the logger.
type Options struct {
	Dir      string
	MaxBytes int64
	Disabled bool
}

func (o Options) dir() string {
	if o.Dir != "" {
		return o.Dir
	}
	if v := os.Getenv("GND_LOG_DIR"); v != "" {
		return v
	}
	return DefaultDir
}

func (o Options) max() int64 {
	if o.MaxBytes > 0 {
		return o.MaxBytes
	}
	return MaxBytes
}

// Logger writes LRU-capped JSONL call logs.
type Logger struct {
	mu  sync.Mutex
	opt Options
}

// New creates a logger.
func New(opt Options) *Logger {
	if os.Getenv("GND_LOG") == "0" || os.Getenv("GND_LOG") == "off" {
		opt.Disabled = true
	}
	return &Logger{opt: opt}
}

// Path returns the jsonl path.
func (l *Logger) Path() string {
	return filepath.Join(l.opt.dir(), FileName)
}

// Append records one entry and trims to budget.
func (l *Logger) Append(e Entry) error {
	if l == nil || l.opt.Disabled {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if e.TS.IsZero() {
		e.TS = time.Now().UTC()
	}
	if e.ID == "" {
		e.ID = NewID(e)
	}
	dir := l.opt.dir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := l.Path()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	line, err := json.Marshal(e)
	if err != nil {
		f.Close()
		return err
	}
	if _, err := f.Write(append(line, '\n')); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return l.trimLocked(path)
}

func (l *Logger) trimLocked(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	max := l.opt.max()
	if st.Size() <= max {
		return nil
	}
	target := max * 4 / 5
	if target > TrimTarget {
		target = TrimTarget
	}
	if target < 1024 {
		target = 1024
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := splitNonEmpty(string(data))
	keep := make([]string, 0, len(lines))
	var size int64
	for i := len(lines) - 1; i >= 0; i-- {
		n := int64(len(lines[i]) + 1)
		if size+n > target {
			break
		}
		keep = append(keep, lines[i])
		size += n
	}
	for i, j := 0, len(keep)-1; i < j; i, j = i+1, j-1 {
		keep[i], keep[j] = keep[j], keep[i]
	}
	body := ""
	if len(keep) > 0 {
		body = strings.Join(keep, "\n") + "\n"
	}
	return os.WriteFile(path, []byte(body), 0o644)
}

// Query filters entries.
type Query struct {
	Cmd      string
	Limit    int
	Since    time.Time
	Contains string // search in argv/stdout/stderr
	Exit     *int
}

// Load reads all entries (newest last).
func (l *Logger) Load() ([]Entry, error) {
	if l == nil {
		return nil, nil
	}
	data, err := os.ReadFile(l.Path())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Entry
	for _, line := range splitNonEmpty(string(data)) {
		var e Entry
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		out = append(out, e)
	}
	return out, nil
}

// Search returns matching entries newest-first.
func (l *Logger) Search(q Query) ([]Entry, error) {
	all, err := l.Load()
	if err != nil {
		return nil, err
	}
	// newest first
	for i, j := 0, len(all)-1; i < j; i, j = i+1, j-1 {
		all[i], all[j] = all[j], all[i]
	}
	var out []Entry
	for _, e := range all {
		if q.Cmd != "" && e.Cmd != q.Cmd {
			continue
		}
		if !q.Since.IsZero() && e.TS.Before(q.Since) {
			continue
		}
		if q.Exit != nil && e.Exit != *q.Exit {
			continue
		}
		if q.Contains != "" {
			blob := strings.ToLower(strings.Join(e.Argv, " ") + " " + e.StdoutTail + " " + e.StderrTail)
			if !strings.Contains(blob, strings.ToLower(q.Contains)) {
				continue
			}
		}
		out = append(out, e)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, nil
}

// Get by id prefix.
func (l *Logger) Get(idPrefix string) (*Entry, error) {
	all, err := l.Load()
	if err != nil {
		return nil, err
	}
	for i := len(all) - 1; i >= 0; i-- {
		if strings.HasPrefix(all[i].ID, idPrefix) {
			e := all[i]
			return &e, nil
		}
	}
	return nil, fmt.Errorf("call id %q not found", idPrefix)
}

// Stats summarizes the store.
type Stats struct {
	Path      string `json:"path"`
	Bytes     int64  `json:"bytes"`
	MaxBytes  int64  `json:"max_bytes"`
	Entries   int    `json:"entries"`
	OldestTS  string `json:"oldest_ts,omitempty"`
	NewestTS  string `json:"newest_ts,omitempty"`
	FailCount int    `json:"fail_count"`
}

func (l *Logger) Stats() (Stats, error) {
	st := Stats{Path: l.Path(), MaxBytes: l.opt.max()}
	fi, err := os.Stat(l.Path())
	if err == nil {
		st.Bytes = fi.Size()
	}
	entries, err := l.Load()
	if err != nil {
		return st, err
	}
	st.Entries = len(entries)
	if len(entries) > 0 {
		st.OldestTS = entries[0].TS.Format(time.RFC3339)
		st.NewestTS = entries[len(entries)-1].TS.Format(time.RFC3339)
	}
	for _, e := range entries {
		if e.Exit != 0 {
			st.FailCount++
		}
	}
	return st, nil
}

// Clear removes the log file.
func (l *Logger) Clear() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	err := os.Remove(l.Path())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// ExtractMeta pulls quick fields from argv.
func ExtractMeta(argv []string) (cmd, dirFlag, goalFlag, preset string) {
	if len(argv) > 0 {
		cmd = argv[0]
	}
	for i := 0; i < len(argv); i++ {
		a := argv[i]
		next := func() string {
			if i+1 < len(argv) {
				return argv[i+1]
			}
			return ""
		}
		switch a {
		case "--dir":
			dirFlag = next()
		case "--goal":
			goalFlag = next()
		case "--preset":
			preset = next()
		case "--root":
			// sim root — still useful
			if dirFlag == "" {
				dirFlag = next()
			}
		}
		if strings.HasPrefix(a, "preset=") || strings.Contains(a, "preset=") {
			for _, p := range strings.Fields(a) {
				if strings.HasPrefix(p, "preset=") {
					preset = strings.TrimPrefix(p, "preset=")
				}
			}
		}
	}
	// one-liner first token (e.g. "preset=slg max_level=30")
	if len(argv) > 0 && strings.Contains(argv[0], "=") {
		for _, p := range strings.Fields(argv[0]) {
			if strings.HasPrefix(p, "preset=") {
				preset = strings.TrimPrefix(p, "preset=")
			}
		}
	}
	return
}

// NewID is a stable-ish short id from content+time.
func NewID(e Entry) string {
	h := sha1.Sum([]byte(e.TS.String() + "|" + strings.Join(e.Argv, " ") + "|" + e.Cwd))
	return fmt.Sprintf("%s-%s", e.TS.UTC().Format("20060102T150405.000"), hex.EncodeToString(h[:4]))
}

// Tail returns last n runes of s.
func Tail(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

func splitNonEmpty(s string) []string {
	raw := strings.Split(s, "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// FormatEntry human table row.
func FormatEntry(e Entry) string {
	return fmt.Sprintf("%s  exit=%d  %6dms  %s",
		e.TS.Local().Format("15:04:05"), e.Exit, e.DurationMS, strings.Join(e.Argv, " "))
}

// SortByTime helper.
func SortByTime(entries []Entry) {
	sort.SliceStable(entries, func(i, j int) bool {
		return entries[i].TS.Before(entries[j].TS)
	})
}
