package goal

import (
	"os"
	"testing"
)

func TestResolveApplyOneLine(t *testing.T) {
	res, err := ResolveApply(ApplyOptions{
		Preset:  "slg",
		OneLine: "max_level=28 daily_exp=1500 win_rate=0.5",
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Goal.Progress.MaxLevel != 28 {
		t.Fatalf("max_level=%d", res.Goal.Progress.MaxLevel)
	}
	if res.Goal.Progress.DailyExp != 1500 {
		t.Fatalf("daily=%v", res.Goal.Progress.DailyExp)
	}
	if res.Goal.Combat.TargetWinRate != 0.5 {
		t.Fatalf("wr=%v", res.Goal.Combat.TargetWinRate)
	}
	if res.Goal.Genre != "slg" {
		t.Fatalf("genre=%s", res.Goal.Genre)
	}
	if len(res.Changed) == 0 {
		t.Fatal("changed empty")
	}
}

func TestResolveApplyFromGoalFile(t *testing.T) {
	base := Preset(PresetOnmyojiTurn)
	dir := t.TempDir()
	// write via marshal path used by CLI — inline here
	p := dir + "/g.yaml"
	yaml := "name: base\ngenre: onmyoji\nprogress:\n  max_level: 40\ncombat:\n  target_win_rate: 0.58\n"
	if err := writeFile(p, yaml); err != nil {
		t.Fatal(err)
	}
	res, err := ResolveApply(ApplyOptions{
		FromGoal: p,
		WinRate:  0.7,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Goal.Combat.TargetWinRate != 0.7 {
		t.Fatalf("wr=%v", res.Goal.Combat.TargetWinRate)
	}
	if res.Source != "goal_file" {
		t.Fatalf("source=%s", res.Source)
	}
	_ = base
}

func TestSelfCheckProductionGates(t *testing.T) {
	g := DefaultGoal()
	gen, err := Build(g)
	if err != nil {
		t.Fatal(err)
	}
	res := &ApplyResult{Goal: g, Generated: gen}
	ch := res.SelfCheck()
	if !ch.OK {
		t.Fatalf("self check should pass: %+v", ch)
	}
	if ch.HeroRows < 1 || ch.SkillRows < 3 {
		t.Fatalf("shape %+v", ch)
	}
}

func TestParseApplyLine(t *testing.T) {
	kv := ParseApplyLine("max_level=40  win_rate=0.55 hard_pity=90")
	if kv["max_level"] != "40" || kv["win_rate"] != "0.55" {
		t.Fatalf("kv=%v", kv)
	}
}

func writeFile(path, content string) error {
	return os.WriteFile(path, []byte(content), 0o644)
}
