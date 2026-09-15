package htmlreport

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neko233-com/game-numerical-design-cli/internal/goal"
	"github.com/neko233-com/game-numerical-design-cli/internal/simulator"
)

func TestRenderSelfContained(t *testing.T) {
	r := New("测试报告", "副标题")
	r.Meta("k", "v")
	r.SectionKV("信息", [][2]string{{"a", "1"}})
	r.SectionTable("表", []string{"h1", "h2"}, [][]string{{"1", "2"}})
	r.SectionWarnings("告警", []string{"某问题"})
	html := r.Render()
	if !strings.Contains(html, "<!DOCTYPE html>") {
		t.Fatal("missing doctype")
	}
	if strings.Contains(html, "http://") || strings.Contains(html, "https://") {
		// allow xmlns only
		for _, line := range strings.Split(html, "\n") {
			if strings.Contains(line, "http://") && !strings.Contains(line, "www.w3.org") {
				t.Fatalf("external url: %s", line)
			}
		}
	}
	if !strings.Contains(html, "测试报告") {
		t.Fatal("missing title")
	}
}

func TestLineChartSVG(t *testing.T) {
	svg := LineChartSVG(LineChartConfig{
		Title: "t",
		Series: []LineSeries{
			{Name: "s", Points: []Point{{X: 0, Y: 0}, {X: 10, Y: 100}}},
		},
	})
	if !strings.Contains(svg, "<svg") || !strings.Contains(svg, "<path") {
		t.Fatalf("bad svg: %s", svg[:min(200, len(svg))])
	}
}

func TestBuildDemoReportFile(t *testing.T) {
	g := goal.DefaultGoal()
	gen, err := goal.Build(g)
	if err != nil {
		t.Fatal(err)
	}
	rep := BuildDemoReport(gen, g)
	dir := t.TempDir()
	path := filepath.Join(dir, "report.html")
	if err := rep.WriteFile(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	for _, want := range []string{"成长曲线", "战斗平衡", "抽卡参数", "<svg"} {
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q", want)
		}
	}
}

func TestBuildSimReport(t *testing.T) {
	man := &simulator.RunManifest{
		RunID: "run_x", Name: "t", N: 10, WinRate: 0.6, AvgTurns: 5,
		TurnHistogram: map[string]int{"4": 3, "5": 5, "6": 2},
		Attacker:      simulator.Unit{Name: "A", HP: 100, Attack: 10},
		Defender:      simulator.Unit{Name: "D", HP: 100, Attack: 10},
	}
	rep := BuildSimReport(man, []simulator.BattleLog{{
		BattleID: 1, Winner: "attacker", Turns: 5, Seed: 1,
	}})
	html := rep.Render()
	if !strings.Contains(html, "战斗模拟报告") {
		t.Fatal("title")
	}
	if !strings.Contains(html, "Turns histogram") {
		t.Fatal("histogram")
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
