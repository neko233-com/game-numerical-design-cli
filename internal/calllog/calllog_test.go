package calllog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAppendLoadSearchLRU(t *testing.T) {
	dir := t.TempDir()
	lg := New(Options{Dir: dir, MaxBytes: 8 * 1024}) // tiny cap to force trim
	for i := 0; i < 50; i++ {
		if err := lg.Append(Entry{
			Cmd: "apply", Argv: []string{"apply", "preset=slg", strings.Repeat("x", 200)},
			Exit: i % 5, StdoutTail: strings.Repeat("out", 100),
		}); err != nil {
			t.Fatal(err)
		}
	}
	st, err := lg.Stats()
	if err != nil {
		t.Fatal(err)
	}
	if st.Bytes > MaxBytes {
		t.Fatalf("bytes=%d cap=%d", st.Bytes, MaxBytes)
	}
	if st.Entries == 0 {
		t.Fatal("no entries after trim")
	}
	if st.Entries >= 50 {
		t.Fatalf("trim did not drop old entries: %d", st.Entries)
	}
	entries, err := lg.Load()
	if err != nil {
		t.Fatal(err)
	}
	// newest should be last
	if entries[len(entries)-1].Cmd != "apply" {
		t.Fatal("order")
	}
	// search
	fails, err := lg.Search(Query{Exit: ptrInt(1), Limit: 5})
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range fails {
		if e.Exit != 1 {
			t.Fatalf("filter broken: %+v", e)
		}
	}
	// contains
	hits, err := lg.Search(Query{Contains: "preset=slg", Limit: 3})
	if err != nil || len(hits) == 0 {
		t.Fatalf("contains hits=%d err=%v", len(hits), err)
	}
	// get by prefix
	e, err := lg.Get(entries[len(entries)-1].ID[:10])
	if err != nil {
		t.Fatal(err)
	}
	if e.Cmd != "apply" {
		t.Fatal("get")
	}
}

func TestDisabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GND_LOG", "0")
	lg := New(Options{Dir: dir})
	if err := lg.Append(Entry{Cmd: "x", Argv: []string{"x"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, FileName)); !os.IsNotExist(err) {
		t.Fatal("disabled logger should not write")
	}
}

func TestExtractMeta(t *testing.T) {
	cmd, dir, goal, preset := ExtractMeta([]string{
		"apply", "preset=slg", "--dir", "out", "--goal", "g.yaml",
	})
	if cmd != "apply" || dir != "out" || goal != "g.yaml" || preset != "slg" {
		t.Fatalf("cmd=%s dir=%s goal=%s preset=%s", cmd, dir, goal, preset)
	}
}

func TestClear(t *testing.T) {
	dir := t.TempDir()
	lg := New(Options{Dir: dir})
	_ = lg.Append(Entry{Cmd: "v", Argv: []string{"version"}, TS: time.Now()})
	if err := lg.Clear(); err != nil {
		t.Fatal(err)
	}
	st, _ := lg.Stats()
	if st.Entries != 0 {
		t.Fatalf("entries=%d", st.Entries)
	}
}

func TestExtractMetaOneLiner(t *testing.T) {
	_, _, _, preset := ExtractMeta([]string{"apply", "preset=genshin max_level=20", "--dir", "x"})
	if preset != "genshin" {
		t.Fatalf("preset=%q", preset)
	}
}

func ptrInt(n int) *int { return &n }
