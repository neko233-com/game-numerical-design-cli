package htmlreport

import (
	"fmt"
	"html"
	"strconv"
	"strings"

	"github.com/neko233-com/game-numerical-design-cli/internal/goal"
	"github.com/neko233-com/game-numerical-design-cli/internal/simulator"
)

// BuildDemoReport renders the full numerical design demo report from goal.Build output.
func BuildDemoReport(gen *goal.Generated, g goal.Goal) *Report {
	r := New("数值设计报告 · "+gen.Report.GoalName, g.Description)
	r.Meta("max_level", strconv.Itoa(gen.Report.MaxLevel))
	r.Meta("heroes", strconv.Itoa(gen.Report.HeroCount))
	r.Meta("skills", strconv.Itoa(gen.Report.SkillCount))
	r.Meta("tasks", strconv.Itoa(gen.Report.TaskCount))
	r.Meta("enemies", strconv.Itoa(gen.Report.EnemyCount))

	// KPI
	okTone := "good"
	if !gen.Report.OK {
		okTone = "warn"
	}
	r.SectionRaw("总览", StatRow([][3]string{
		{"目标", gen.Report.GoalName, ""},
		{"是否达标", boolText(gen.Report.OK), okTone},
		{"告警数", strconv.Itoa(len(gen.Report.Warnings)), toneByCount(len(gen.Report.Warnings))},
		{"日均经验", fmt.Sprintf("%.0f", g.Progress.DailyExp), ""},
		{"目标胜率", fmt.Sprintf("%.0f%%", g.Combat.TargetWinRate*100), ""},
		{"硬保底", strconv.Itoa(g.Gacha.HardPity), ""},
	}))

	// Level curve
	var expPts, cumPts []Point
	for _, lv := range gen.Report.LevelExp {
		expPts = append(expPts, Point{X: float64(lv.Level), Y: lv.ExpToNext})
		cumPts = append(cumPts, Point{X: float64(lv.Level), Y: lv.CumExp})
	}
	r.SectionRaw("成长曲线", ChartGrid(true,
		LineChartSVG(LineChartConfig{
			Title: "每级所需经验 exp_to_next", XLabel: "等级", YLabel: "经验",
			Series: []LineSeries{{Name: "exp_to_next", Points: expPts}},
		}),
		LineChartSVG(LineChartConfig{
			Title: "累计经验 cum_exp", XLabel: "等级", YLabel: "累计经验",
			Series: []LineSeries{{Name: "cum_exp", Points: cumPts, Color: colorOK}},
		}),
		LineChartSVG(LineChartConfig{
			Title: "属性成长倍率 stat_mult", XLabel: "等级", YLabel: "倍率",
			Series: []LineSeries{statMultSeries(gen)},
		}),
	))

	// Pace table + chart
	var pacePts []Point
	for _, p := range gen.Report.Pace {
		if p.TargetDays > 0 {
			pacePts = append(pacePts, Point{X: float64(p.Level), Y: p.DaysNeeded})
		}
	}
	var targetPts []Point
	for _, p := range gen.Report.Pace {
		if p.TargetDays > 0 {
			targetPts = append(targetPts, Point{X: float64(p.Level), Y: p.TargetDays})
		}
	}
	r.SectionRaw("升级节奏 vs 设计目标", ChartGrid(false,
		LineChartSVG(LineChartConfig{
			Title: "达成天数（实测 vs 目标）", XLabel: "等级", YLabel: "天",
			Series: []LineSeries{
				{Name: "实际所需天数", Points: pacePts},
				{Name: "设计目标天数", Points: targetPts, Color: colorWarn},
			}, ShowPoints: true,
		}),
	))
	paceRows := make([][]string, 0)
	for _, p := range gen.Report.Pace {
		if p.TargetDays == 0 {
			continue
		}
		paceRows = append(paceRows, []string{
			"Lv" + strconv.Itoa(p.Level),
			fmt.Sprintf("%.0f", p.RequiredTotal),
			fmt.Sprintf("%.2f", p.DaysNeeded),
			fmt.Sprintf("%.2f", p.TargetDays),
			fmt.Sprintf("%+.0f%%", p.DeltaRatio*100),
			p.Verdict,
		})
	}
	r.SectionTable("升级节奏明细", []string{"等级", "累计经验", "所需天", "目标天", "Δ", "判定"}, paceRows)

	// Combat
	r.SectionRaw("战斗平衡（同级 1v1 模拟）", combatSection(gen))

	// Gacha
	r.SectionKV("抽卡参数", [][2]string{
		{"5★ 基础概率", fmt.Sprintf("%.3f%%", gen.Report.Gacha.BaseRate5*100)},
		{"软保底起点", strconv.Itoa(gen.Report.Gacha.SoftStart)},
		{"软保底步进", fmt.Sprintf("%.2f%%", gen.Report.Gacha.SoftStep*100)},
		{"硬保底", strconv.Itoa(gen.Report.Gacha.HardPity)},
		{"UP 占比", fmt.Sprintf("%.1f%%", gen.Report.Gacha.Featured*100)},
	})
	// soft pity curve: rate vs pull
	r.SectionRaw("软保底概率曲线", ChartGrid(false, softPityChart(gen.Report.Gacha)))

	// Config tables summary
	if gen.Hero != nil {
		r.SectionTable("英雄配置", gen.Hero.Headers, sampleRows(gen.Hero.Rows, 12))
	}
	if gen.Enemy != nil {
		r.SectionTable("敌人配置", gen.Enemy.Headers, sampleRows(gen.Enemy.Rows, 12))
	}
	if gen.Task != nil {
		r.SectionTable("任务配置（前 12 行）", gen.Task.Headers, sampleRows(gen.Task.Rows, 12))
	}
	if gen.Item != nil {
		r.SectionTable("道具配置", gen.Item.Headers, gen.Item.Rows)
	}

	r.SectionWarnings("告警与建议", gen.Report.Warnings)
	return r
}

func statMultSeries(gen *goal.Generated) LineSeries {
	var pts []Point
	for _, lv := range gen.Report.LevelExp {
		pts = append(pts, Point{X: float64(lv.Level), Y: lv.StatMult})
	}
	return LineSeries{Name: "stat_mult", Points: pts, Color: colorAccent}
}

func combatSection(gen *goal.Generated) string {
	checks := gen.Report.CombatChecks
	if len(checks) == 0 {
		return "<p>无战斗检查数据</p>"
	}
	// group by hero for bar chart of elite win rates
	var eliteBars, bossBars []BarItem
	rows := make([][]string, 0, len(checks))
	for i, c := range checks {
		tone := ""
		if !c.OK {
			tone = "bad"
		}
		_ = tone
		rows = append(rows, []string{
			boolText(c.OK), c.HeroName, c.VsEnemy,
			fmt.Sprintf("%.0f%%", c.WinRate*100),
			fmt.Sprintf("%.1f", c.AvgTurns), c.Note,
		})
		col := seriesColors[i%len(seriesColors)]
		if c.VsEnemy != "" && len(c.VsEnemy) >= 2 && c.VsEnemy[:2] == "精英" {
			eliteBars = append(eliteBars, BarItem{Label: c.HeroName, Value: c.WinRate * 100, Color: col})
		} else {
			bossBars = append(bossBars, BarItem{Label: c.HeroName, Value: c.WinRate * 100, Color: col})
		}
	}
	// target line as annotation in title
	charts := ChartGrid(true,
		BarChartSVG("对精英胜率 %（目标参考 "+fmt.Sprintf("%.0f", 55.0)+"%）", eliteBars, 520, 200, true),
		BarChartSVG("对 Boss 胜率 %", bossBars, 520, 200, true),
	)
	table := tableHTML([]string{"OK", "英雄", "对手", "胜率", "回合", "说明"}, rows)
	return charts + table
}

func softPityChart(g gachaPreview) string {
	base := g.BaseRate5
	hard := g.HardPity
	if hard <= 0 {
		hard = 90
	}
	start := g.SoftStart
	if start <= 0 {
		start = int(float64(hard) * 0.65)
	}
	step := g.SoftStep
	var pts []Point
	for i := 1; i <= hard; i++ {
		rate := base
		if i >= start {
			rate = base + step*float64(i-start+1)
			if rate > 1 {
				rate = 1
			}
		}
		if i >= hard {
			rate = 1
		}
		pts = append(pts, Point{X: float64(i), Y: rate * 100})
	}
	return LineChartSVG(LineChartConfig{
		Title: "单抽出货概率 vs 抽数", XLabel: "抽数", YLabel: "概率 %",
		Series: []LineSeries{{Name: "rate%", Points: pts, Color: colorAccent}},
	})
}

// gachaPreview mirrors goal.GachaPreview to avoid extra import churn in signature.
type gachaPreview = goal.GachaPreview

// BuildSimReport renders a simulator run report.
func BuildSimReport(man *simulator.RunManifest, battles []simulator.BattleLog) *Report {
	r := New("战斗模拟报告 · "+man.Name, "并行蒙特卡洛 · 每场独立日志")
	r.Meta("run_id", man.RunID)
	r.Meta("N", strconv.Itoa(man.N))
	r.Meta("workers", strconv.Itoa(man.Workers))
	r.Meta("seed", strconv.FormatInt(man.Seed, 10))
	r.Meta("duration", fmt.Sprintf("%d ms", man.DurationMS))

	tone := "good"
	if man.WinRate < 0.35 || man.WinRate > 0.75 {
		tone = "warn"
	}
	r.SectionRaw("KPI", StatRow([][3]string{
		{"进攻方胜率", fmt.Sprintf("%.1f%%", man.WinRate*100), tone},
		{"平局率", fmt.Sprintf("%.1f%%", man.DrawRate*100), ""},
		{"平均回合", fmt.Sprintf("%.2f", man.AvgTurns), ""},
		{"P50 回合", fmt.Sprintf("%.0f", man.P50Turns), ""},
		{"P90 回合", fmt.Sprintf("%.0f", man.P90Turns), ""},
		{"日志数", strconv.Itoa(man.LogCount), ""},
	}))

	// turn histogram
	hist := map[int]int{}
	for k, v := range man.TurnHistogram {
		if n, err := strconv.Atoi(k); err == nil {
			hist[n] = v
		}
	}
	r.SectionRaw("回合分布", ChartGrid(false, HistogramSVG("Turns histogram", hist, 640, 240)))

	// units
	r.SectionKV("对阵", [][2]string{
		{"进攻", man.Attacker.Name + fmt.Sprintf("  HP=%.0f ATK=%.0f DEF=%.0f SPD=%.0f", man.Attacker.HP, man.Attacker.Attack, man.Attacker.Defense, man.Attacker.Speed)},
		{"防守", man.Defender.Name + fmt.Sprintf("  HP=%.0f ATK=%.0f DEF=%.0f SPD=%.0f", man.Defender.HP, man.Defender.Attack, man.Defender.Defense, man.Defender.Speed)},
		{"日志目录", man.LogDir},
	})

	// sample battles
	if len(battles) > 0 {
		rows := make([][]string, 0, len(battles))
		for _, b := range battles {
			rows = append(rows, []string{
				strconv.Itoa(b.BattleID), b.Winner, strconv.Itoa(b.Turns),
				fmt.Sprintf("%.0f", b.DmgDealt), fmt.Sprintf("%.0f", b.DmgTaken),
				strconv.FormatInt(b.Seed, 10),
			})
		}
		r.SectionTable("战斗日志抽样", []string{"#", "胜者", "回合", "伤害", "承伤", "seed"}, rows)
	}
	return r
}

func sampleRows(rows [][]string, n int) [][]string {
	if len(rows) <= n {
		return rows
	}
	return rows[:n]
}

func boolText(ok bool) string {
	if ok {
		return "OK"
	}
	return "FAIL"
}

func toneByCount(n int) string {
	switch {
	case n == 0:
		return "good"
	case n <= 3:
		return "warn"
	default:
		return "bad"
	}
}

func tableHTML(headers []string, rows [][]string) string {
	var sb strings.Builder
	sb.WriteString(`<div class="table-wrap"><table class="data"><thead><tr>`)
	for _, h := range headers {
		sb.WriteString("<th>" + html.EscapeString(h) + "</th>")
	}
	sb.WriteString("</tr></thead><tbody>")
	for _, row := range rows {
		sb.WriteString("<tr>")
		for i := 0; i < len(headers); i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			sb.WriteString("<td>" + html.EscapeString(cell) + "</td>")
		}
		sb.WriteString("</tr>")
	}
	sb.WriteString("</tbody></table></div>")
	return sb.String()
}
