package goal

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultGoalBuild(t *testing.T) {
	g := DefaultGoal()
	if err := g.normalize(); err != nil {
		t.Fatal(err)
	}
	gen, err := Build(g)
	if err != nil {
		t.Fatal(err)
	}
	if gen.Hero == nil || len(gen.Hero.Rows) < 4 {
		t.Fatalf("heroes=%v", gen.Hero)
	}
	if gen.Skill == nil || len(gen.Skill.Rows) < 12 {
		t.Fatalf("skills rows=%d", len(gen.Skill.Rows))
	}
	if gen.LvUp == nil || len(gen.LvUp.Rows) != 40 {
		t.Fatalf("levels=%d", len(gen.LvUp.Rows))
	}
	if gen.Task == nil || len(gen.Task.Rows) < 20 {
		t.Fatalf("tasks=%d", len(gen.Task.Rows))
	}
	if gen.Enemy == nil || len(gen.Enemy.Rows) < 4 {
		t.Fatalf("enemies=%d", len(gen.Enemy.Rows))
	}
	if len(gen.Report.CombatChecks) == 0 {
		t.Fatal("no combat checks")
	}
	if len(gen.Report.LevelExp) != 40 {
		t.Fatalf("level exp points=%d", len(gen.Report.LevelExp))
	}
	// cumulative exp should be non-decreasing
	for i := 1; i < len(gen.Report.LevelExp); i++ {
		if gen.Report.LevelExp[i].CumExp < gen.Report.LevelExp[i-1].CumExp {
			t.Fatalf("cum exp decreased at lv%d", gen.Report.LevelExp[i].Level)
		}
	}
}

func TestWriteAllAndCheck(t *testing.T) {
	g := DefaultGoal()
	gen, err := Build(g)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	written, err := gen.WriteAll(dir, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(written) < 14 { // 7 csv + 7 xlsx
		t.Fatalf("written=%d %v", len(written), written)
	}
	// csv exists
	data, err := os.ReadFile(filepath.Join(dir, "HeroConfig.csv"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "毁灭") && !strings.Contains(string(data), "path") {
		t.Fatalf("hero csv unexpected: %s", data[:min(200, len(data))])
	}
	issues := gen.CheckTables()
	for _, is := range issues {
		if is.Severity == "error" {
			t.Fatalf("validate error: %+v", is)
		}
	}
}

func TestLoadGoalYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "g.yaml")
	content := `
name: 自定义目标
version: 1
progress:
  max_level: 30
  daily_exp: 1000
  early_slope: 2.0
  late_slope: 0.5
  break_level: 15
  stat_gain_pct: 10
  target_days:
    "10": 0.8
    "20": 2.0
    "30": 3.5
combat:
  target_win_rate: 0.5
  target_turns: 6
  archetypes:
    - id: "2001"
      name: 测试战士
      path: 毁灭
      element: 物理
      rarity: 5
      hp: 1000
      atk: 600
      def: 400
      spd: 100
      crit_rate: 0.05
      crit_dmg: 0.5
      basic_mult: 1.0
      skill_mult: 1.5
      ult_mult: 3.0
gacha:
  base_rate_5: 0.008
  hard_pity: 80
`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	g, err := LoadGoal(p)
	if err != nil {
		t.Fatal(err)
	}
	if g.Name != "自定义目标" || g.Progress.MaxLevel != 30 {
		t.Fatalf("loaded=%+v", g.Progress)
	}
	if len(g.Combat.Archetypes) != 1 || g.Combat.Archetypes[0].ID != "2001" {
		t.Fatalf("archetypes=%+v", g.Combat.Archetypes)
	}
	gen, err := Build(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(gen.Hero.Rows) != 1 {
		t.Fatalf("heroes=%d", len(gen.Hero.Rows))
	}
	if len(gen.LvUp.Rows) != 30 {
		t.Fatalf("levels=%d", len(gen.LvUp.Rows))
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
