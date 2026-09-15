package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/game-numerical-design-cli/internal/goal"
)

// TestDeepPipelineAllPresets runs init → check → report → sim → table → script
// for every genre preset (genshin / slg / onmyoji).
func TestDeepPipelineAllPresets(t *testing.T) {
	for _, preset := range goal.PresetIDs() {
		preset := preset
		t.Run(string(preset), func(t *testing.T) {
			dir := t.TempDir()
			goalPath := filepath.Join(dir, "goal.yaml")
			cfgDir := filepath.Join(dir, "configs")
			repPath := filepath.Join(dir, "report.html")

			// 1) init
			out, errS, code := run(t, "demo", "init", "--dir", dir, "--preset", string(preset), "--xlsx")
			if code != 0 {
				t.Fatalf("init code=%d err=%s out=%s", code, errS, out)
			}
			if _, err := os.Stat(goalPath); err != nil {
				t.Fatal("goal.yaml missing")
			}
			heroCSV := filepath.Join(cfgDir, "HeroConfig.csv")
			if _, err := os.Stat(heroCSV); err != nil {
				t.Fatal("HeroConfig.csv missing")
			}

			// goal yaml carries genre
			gdata, _ := os.ReadFile(goalPath)
			if !strings.Contains(string(gdata), "genre:") {
				t.Fatalf("goal missing genre:\n%s", gdata)
			}

			// 2) check + html
			out, errS, code = run(t, "demo", "check", "--goal", goalPath, "--html", repPath)
			if code != 0 {
				t.Fatalf("check code=%d err=%s", code, errS)
			}
			if !strings.Contains(out, "report:") {
				t.Fatalf("check out=%s", out)
			}
			hdata, err := os.ReadFile(repPath)
			if err != nil {
				t.Fatal("report html missing")
			}
			hs := string(hdata)
			for _, want := range []string{"成长曲线", "战斗平衡", "<svg", "抽卡参数"} {
				if !strings.Contains(hs, want) {
					t.Fatalf("report missing %q", want)
				}
			}

			// 3) table schema + convert roundtrip
			out, errS, code = run(t, "table", "schema", heroCSV)
			if code != 0 {
				t.Fatalf("schema code=%d err=%s", code, errS)
			}
			if !strings.Contains(out, "rows:") {
				t.Fatalf("schema=%s", out)
			}
			xlsxOut := filepath.Join(dir, "hero_rt.xlsx")
			out, errS, code = run(t, "table", "convert", heroCSV, xlsxOut)
			if code != 0 {
				t.Fatalf("convert code=%d err=%s", code, errS)
			}
			if _, err := os.Stat(xlsxOut); err != nil {
				t.Fatal("xlsx convert missing")
			}
			jsonOut := filepath.Join(dir, "hero_rt.json")
			if _, errS, code = run(t, "table", "convert", xlsxOut, jsonOut); code != 0 {
				t.Fatalf("xlsx→json code=%d err=%s", code, errS)
			}

			// 4) validate multi-format
			out, errS, code = run(t, "validate", "table", "--file", filepath.Join(cfgDir, "EnemyConfig.csv"))
			if code != 0 {
				t.Fatalf("validate code=%d err=%s out=%s", code, errS, out)
			}

			// 5) sim from config
			// pick first elite enemy id from csv
			edata, _ := os.ReadFile(filepath.Join(cfgDir, "EnemyConfig.csv"))
			enemyID := firstDataID(string(edata))
			simRoot := filepath.Join(dir, "sim")
			out, errS, code = run(t, "sim", "run",
				"--n", "40", "--workers", "4", "--seed", "9",
				"--from-config", cfgDir, "--hero-id", "1001", "--enemy-id", enemyID,
				"--level", "10", "--root", simRoot, "--name", "deep")
			if code != 0 {
				// slg/onmyoji hero ids may differ — try first hero id
				hdata2, _ := os.ReadFile(heroCSV)
				hid := firstDataID(string(hdata2))
				out, errS, code = run(t, "sim", "run",
					"--n", "40", "--workers", "4", "--seed", "9",
					"--from-config", cfgDir, "--hero-id", hid, "--enemy-id", enemyID,
					"--level", "10", "--root", simRoot, "--name", "deep")
				if code != 0 {
					t.Fatalf("sim code=%d err=%s out=%s", code, errS, out)
				}
			}
			if !strings.Contains(out, "run_id") && !strings.Contains(out, "html:") {
				// format table prints run_id
				if !strings.Contains(out, "win_rate") {
					t.Fatalf("sim out=%s", out)
				}
			}
			// auto html report
			out, errS, code = run(t, "report", "sim", "--root", simRoot)
			if code != 0 {
				t.Fatalf("report sim code=%d err=%s", code, errS)
			}

			// 6) script eval with gnd API
			out, errS, code = run(t, "script", "eval",
				`export default gnd.curve("piecewise-log", 10, {s1:1.8,s2:0.6,break:20})`)
			if code != 0 {
				t.Fatalf("script code=%d err=%s", code, errS)
			}
			if !strings.Contains(out, "result:") {
				t.Fatalf("script out=%s", out)
			}

			// 7) gacha suggest still works
			if _, errS, code = run(t, "gacha", "suggest", "--rate", "0.012", "--hard", "50"); code != 0 {
				t.Fatalf("gacha suggest err=%s", errS)
			}
		})
	}
}

func firstDataID(csv string) string {
	lines := strings.Split(strings.TrimSpace(csv), "\n")
	if len(lines) < 2 {
		return "1001"
	}
	parts := strings.Split(lines[1], ",")
	if len(parts) == 0 {
		return "1001"
	}
	return strings.TrimSpace(parts[0])
}

func TestDemoPresetsCommand(t *testing.T) {
	out, errS, code := run(t, "demo", "presets")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	for _, want := range []string{"genshin", "slg", "onmyoji"} {
		if !strings.Contains(out, want) {
			t.Fatalf("presets missing %q in %s", want, out)
		}
	}
}

func TestDemoGoalPresetFlag(t *testing.T) {
	for _, p := range []string{"slg", "onmyoji", "genshin"} {
		out, errS, code := run(t, "demo", "goal", "--preset", p)
		if code != 0 {
			t.Fatalf("%s code=%d err=%s", p, code, errS)
		}
		if !strings.Contains(out, "genre:") {
			t.Fatalf("%s goal missing genre: %s", p, out[:min(200, len(out))])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
