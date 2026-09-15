package goal

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func TestPresetIDsAndParse(t *testing.T) {
	ids := PresetIDs()
	if len(ids) != 3 {
		t.Fatalf("ids=%v", ids)
	}
	cases := map[string]PresetID{
		"genshin": PresetGenshinRPG, "原神": PresetGenshinRPG,
		"slg": PresetWhiteoutSLG, "无尽冬日": PresetWhiteoutSLG,
		"onmyoji": PresetOnmyojiTurn, "阴阳师": PresetOnmyojiTurn,
		"": PresetGenshinRPG, "unknown": PresetGenshinRPG,
	}
	for in, want := range cases {
		if got := ParsePreset(in); got != want {
			t.Fatalf("ParsePreset(%q)=%q want %q", in, got, want)
		}
	}
}

func TestAllPresetsBuildDeep(t *testing.T) {
	dir := t.TempDir()
	for _, id := range PresetIDs() {
		id := id
		t.Run(string(id), func(t *testing.T) {
			g := Preset(id)
			if g.Genre == "" {
				t.Fatal("genre empty")
			}
			if len(g.Combat.Archetypes) < 4 {
				t.Fatalf("archetypes=%d", len(g.Combat.Archetypes))
			}
			gen, err := Build(g)
			if err != nil {
				t.Fatal(err)
			}
			// structural
			if gen.Hero == nil || len(gen.Hero.Rows) != len(g.Combat.Archetypes) {
				t.Fatalf("hero rows=%v", gen.Hero)
			}
			if gen.Skill == nil || len(gen.Skill.Rows) != len(g.Combat.Archetypes)*3 {
				t.Fatalf("skill rows=%d", len(gen.Skill.Rows))
			}
			if gen.LvUp == nil || len(gen.LvUp.Rows) != g.Progress.MaxLevel {
				t.Fatalf("levels=%d want %d", len(gen.LvUp.Rows), g.Progress.MaxLevel)
			}
			if gen.Enemy == nil || len(gen.Enemy.Rows) < 4 {
				t.Fatalf("enemies=%d", len(gen.Enemy.Rows))
			}
			if gen.Task == nil || len(gen.Task.Rows) < g.Tasks.MainCount {
				t.Fatalf("tasks=%d", len(gen.Task.Rows))
			}
			if gen.Item == nil || len(gen.Item.Rows) < 6 {
				t.Fatalf("items=%d", len(gen.Item.Rows))
			}
			if gen.Gacha == nil || len(gen.Gacha.Rows) < 2 {
				t.Fatal("gacha missing")
			}

			// level curve integrity
			if len(gen.Report.LevelExp) != g.Progress.MaxLevel {
				t.Fatalf("level exp pts=%d", len(gen.Report.LevelExp))
			}
			for i := 1; i < len(gen.Report.LevelExp); i++ {
				if gen.Report.LevelExp[i].CumExp < gen.Report.LevelExp[i-1].CumExp {
					t.Fatalf("cum exp decreased at lv%d", gen.Report.LevelExp[i].Level)
				}
			}
			// pace hits target days reasonably at last milestone
			last := gen.Report.LevelExp[len(gen.Report.LevelExp)-1]
			targetKey := strconv.Itoa(g.Progress.MaxLevel)
			if td, ok := g.Progress.TargetDays[targetKey]; ok && td > 0 {
				days := last.CumExp / g.Progress.DailyExp
				delta := (days - td) / td
				if delta > 0.25 || delta < -0.5 {
					t.Fatalf("max-level pace days=%.2f target=%.2f delta=%.0f%%", days, td, delta*100)
				}
			}

			// combat checks exist for each archetype × (elite + boss)
			if len(gen.Report.CombatChecks) < len(g.Combat.Archetypes) {
				t.Fatalf("combat checks=%d", len(gen.Report.CombatChecks))
			}

			// validate tables — no error severity
			for _, is := range gen.CheckTables() {
				if is.Severity == "error" {
					t.Fatalf("validate error: %+v", is)
				}
			}

			// write + reload
			out := filepath.Join(dir, string(id))
			written, err := gen.WriteAll(out, true)
			if err != nil {
				t.Fatal(err)
			}
			if len(written) < 14 {
				t.Fatalf("written=%d", len(written))
			}
			for _, name := range []string{"HeroConfig", "SkillConfig", "EnemyConfig", "TaskConfig", "ItemConfig", "GachaPoolConfig", "HeroLvUpConfig"} {
				p := filepath.Join(out, name+".csv")
				if _, err := os.Stat(p); err != nil {
					t.Fatalf("missing %s: %v", p, err)
				}
			}
		})
	}
}

func TestGenreFlavorNames(t *testing.T) {
	slg := Preset(PresetWhiteoutSLG)
	gen, err := Build(slg)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(gen.Enemy.Rows[0][1], "雪原") && !strings.Contains(gen.Item.Rows[0][1], "生肉") {
		t.Fatalf("slg flavor missing: enemy=%v item=%v", gen.Enemy.Rows[0], gen.Item.Rows[0])
	}
	// skill suffix
	found := false
	for _, r := range gen.Skill.Rows {
		if strings.Contains(r[2], "战术指令") || strings.Contains(r[2], "统率技") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("slg skill names=%v", gen.Skill.Rows[0])
	}

	on := Preset(PresetOnmyojiTurn)
	gen2, err := Build(on)
	if err != nil {
		t.Fatal(err)
	}
	okItem := false
	for _, r := range gen2.Item.Rows {
		if r[1] == "勾玉" || r[1] == "神秘的符咒" {
			okItem = true
		}
	}
	if !okItem {
		t.Fatalf("onmyoji items=%v", gen2.Item.Rows)
	}
	okEnemy := false
	for _, r := range gen2.Enemy.Rows {
		if strings.Contains(r[1], "妖灵") || strings.Contains(r[1], "八岐") {
			okEnemy = true
		}
	}
	if !okEnemy {
		t.Fatalf("onmyoji enemies=%v", gen2.Enemy.Rows[0])
	}
}

func TestGenshinDefaultStillWorks(t *testing.T) {
	g := DefaultGoal()
	if g.Genre != string(PresetGenshinRPG) {
		t.Fatalf("genre=%s", g.Genre)
	}
	gen, err := Build(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(gen.Hero.Headers) == 0 || gen.Hero.ColIndex("weapon") < 0 {
		t.Fatalf("headers=%v", gen.Hero.Headers)
	}
}
