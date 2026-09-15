package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func run(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errBuf bytes.Buffer
	app := &App{Stdout: &out, Stderr: &errBuf, Args: args}
	code := app.Run()
	return out.String(), errBuf.String(), code
}

func TestRootHelp(t *testing.T) {
	out, _, code := run(t)
	if code != 0 {
		t.Fatalf("code=%d", code)
	}
	if !strings.Contains(out, "curve") || !strings.Contains(out, "gacha") {
		t.Fatalf("help missing commands: %s", out)
	}
}

func TestCurveLinear(t *testing.T) {
	out, errS, code := run(t, "curve", "linear", "--a", "10", "--b", "2", "--max-x", "5", "--n", "5")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "y =") && !strings.Contains(out, "linear") {
		t.Fatalf("out=%s", out)
	}
}

func TestCombatDmg(t *testing.T) {
	out, errS, code := run(t, "combat", "dmg", "--atk", "1000", "--def", "1000", "--type", "physical")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "expected dmg") {
		t.Fatalf("out=%s", out)
	}
}

func TestEconomyBalance(t *testing.T) {
	out, errS, code := run(t, "economy", "balance", "--name", "gold", "--prod", "2000", "--cons", "1000")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "通胀") && !strings.Contains(out, "critical") {
		t.Fatalf("out=%s", out)
	}
}

func TestGachaSuggest(t *testing.T) {
	out, errS, code := run(t, "gacha", "suggest", "--rate", "0.006", "--hard", "90")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "soft start") {
		t.Fatalf("out=%s", out)
	}
}

func TestFeelDecode(t *testing.T) {
	out, errS, code := run(t, "feel", "decode", "--metric", "level_stat_gain_pct", "--value", "5")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "升级没爽感") {
		t.Fatalf("out=%s", out)
	}
}

func TestRecipeList(t *testing.T) {
	out, errS, code := run(t, "recipe", "list")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "gacha-soft-pity") {
		t.Fatalf("out=%s", out)
	}
}

func TestNDD(t *testing.T) {
	out, errS, code := run(t, "ndd", "new", "--name", "测试", "--system", "gacha")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "数值设计文档") || !strings.Contains(out, "评审 Checklist") {
		t.Fatalf("ndd template incomplete")
	}
}

func TestValidateCSV(t *testing.T) {
	// write temp via stdin path is harder; use file in t.TempDir
	dir := t.TempDir()
	path := dir + "/t.csv"
	data := "id,name,exp\n1,a,100\n2,b,400\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errS, code := run(t, "validate", "table", "--file", path)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "断崖") && !strings.Contains(out, "跳变") {
		t.Fatalf("expected cliff warn, out=%s", out)
	}
}
