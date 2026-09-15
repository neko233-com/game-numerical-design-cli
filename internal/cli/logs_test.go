package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/game-numerical-design-cli/internal/calllog"
)

func TestCallLogCapturesInvocations(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GND_LOG_DIR", filepath.Join(dir, "logs"))
	// real command via App.Run
	app := &App{Args: []string{"version"}, NoCallLog: false}
	var out strings.Builder
	app.Stdout = &out
	app.Stderr = &out
	if code := app.Run(); code != 0 {
		t.Fatalf("code=%d", code)
	}
	lg := calllog.New(calllog.Options{Dir: filepath.Join(dir, "logs")})
	entries, err := lg.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) == 0 {
		t.Fatal("no call logged")
	}
	found := false
	for _, e := range entries {
		if e.Cmd == "version" {
			found = true
			if e.Exit != 0 {
				t.Fatalf("exit=%d", e.Exit)
			}
			if !strings.Contains(e.StdoutTail, "gnd") {
				t.Fatalf("stdout tail=%q", e.StdoutTail)
			}
		}
	}
	if !found {
		t.Fatalf("version not in log: %+v", entries)
	}
}

func TestLogsListAndShow(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	t.Setenv("GND_LOG_DIR", logDir)
	// seed via Run
	app := &App{Args: []string{"recipe", "list"}}
	var buf strings.Builder
	app.Stdout, app.Stderr = &buf, &buf
	app.Run()

	out, errS, code := run(t, "logs", "list", "--limit", "5")
	if code != 0 {
		t.Fatalf("list code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "recipe") && !strings.Contains(out, "argv") {
		t.Fatalf("list=%s", out)
	}
	// stats
	out, errS, code = run(t, "logs", "stats")
	if code != 0 {
		t.Fatalf("stats code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "bytes") {
		t.Fatalf("stats=%s", out)
	}
	// path
	out, _, _ = run(t, "logs", "path")
	if !strings.Contains(out, "calls.jsonl") {
		t.Fatalf("path=%s", out)
	}
}

func TestLogsNotSelfLogged(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("GND_LOG_DIR", filepath.Join(dir, "logs"))
	app := &App{Args: []string{"logs", "list"}}
	var buf strings.Builder
	app.Stdout, app.Stderr = &buf, &buf
	app.Run()
	// logs command itself should not append
	lg := calllog.New(calllog.Options{Dir: filepath.Join(dir, "logs")})
	entries, _ := lg.Load()
	for _, e := range entries {
		if e.Cmd == "logs" {
			t.Fatal("logs should not self-log")
		}
	}
}

func TestGitignoreCoversLogs(t *testing.T) {
	// ensure repo root ignore still lists .gnd
	// path relative to module root when tests run from internal/cli
	p := filepath.Join("..", "..", ".gitignore")
	data, err := os.ReadFile(p)
	if err != nil {
		t.Skip("no gitignore")
	}
	if !strings.Contains(string(data), ".gnd") {
		t.Fatal(".gitignore missing .gnd")
	}
}
