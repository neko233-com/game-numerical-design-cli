package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDemoInitAndSim(t *testing.T) {
	dir := t.TempDir()
	out, errS, code := run(t, "demo", "init", "--dir", dir)
	if code != 0 {
		t.Fatalf("init code=%d err=%s out=%s", code, errS, out)
	}
	if !strings.Contains(out, "demo ready") {
		t.Fatalf("out=%s", out)
	}
	goalPath := filepath.Join(dir, "goal.yaml")
	if _, err := os.Stat(goalPath); err != nil {
		t.Fatal("goal.yaml missing")
	}
	hero := filepath.Join(dir, "configs", "HeroConfig.csv")
	if _, err := os.Stat(hero); err != nil {
		t.Fatal("HeroConfig.csv missing")
	}
	out, errS, code = run(t, "demo", "sim", "--goal", goalPath)
	if code != 0 {
		t.Fatalf("sim code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "combat") || !strings.Contains(out, "gacha") {
		t.Fatalf("sim out=%s", out)
	}
	out, errS, code = run(t, "demo", "check", "--goal", goalPath)
	if code != 0 {
		t.Fatalf("check code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "report:") {
		t.Fatalf("check out=%s", out)
	}
}

func TestDemoGoalPrint(t *testing.T) {
	out, errS, code := run(t, "demo", "goal")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "星轨数值Demo") && !strings.Contains(out, "progress") {
		t.Fatalf("out=%s", out)
	}
}
