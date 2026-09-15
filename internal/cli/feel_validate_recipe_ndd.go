package cli

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/neko233-com/game-numerical-design-cli/internal/feel"
	"github.com/neko233-com/game-numerical-design-cli/internal/recipe"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
	"github.com/neko233-com/game-numerical-design-cli/internal/validate"
)

func (a *App) cmdFeel(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd feel <list|decode|encode>

list
  [--category combat|progress|economy|gacha]

decode — metric readings → diagnoses
  --metric level_stat_gain_pct --value 5
  --readings "level_stat_gain_pct=5,hit_stop_frames=4"

encode — phrase → candidate rules
  --phrase "打击感太轻"
`)
		return 0
	}
	switch args[0] {
	case "list":
		return a.feelList(args[1:])
	case "decode":
		return a.feelDecode(args[1:])
	case "encode":
		return a.feelEncode(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown feel subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) feelList(args []string) int {
	fs := a.newFlagSet("feel list")
	cat := fs.String("category", "", "filter category")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	ms := feel.DefaultMappings()
	rows := make([][]string, 0, len(ms))
	for _, m := range ms {
		if *cat != "" && m.Category != *cat {
			continue
		}
		band := ""
		switch m.Direction {
		case "low_is_bad":
			band = fmt.Sprintf("≥ %g", m.Min)
		case "high_is_bad":
			band = fmt.Sprintf("≤ %g", m.Max)
		default:
			band = fmt.Sprintf("[%g, %g]", m.Min, m.Max)
		}
		rows = append(rows, []string{m.Phrase, m.Category, m.Metric, m.Unit, band, m.Note})
	}
	if err := report.Table(a.Stdout,
		[]string{"phrase", "cat", "metric", "unit", "healthy", "note"}, rows); err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "\ncategories: %s\n", strings.Join(feel.Categories(nil), ", "))
	return 0
}

func (a *App) feelDecode(args []string) int {
	fs := a.newFlagSet("feel decode")
	metric := fs.String("metric", "", "single metric key")
	value := fs.String("value", "", "value for --metric")
	readings := fs.String("readings", "", "k=v,k=v")
	format := fs.String("format", "table", "table|json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	rmap, err := parseMap(*readings)
	if err != nil {
		return a.fail(err)
	}
	if *metric != "" && *value != "" {
		v, err := strconv.ParseFloat(*value, 64)
		if err != nil {
			return a.fail(fmt.Errorf("bad --value"))
		}
		rmap[*metric] = v
	}
	if len(rmap) == 0 {
		return a.fail(fmt.Errorf("provide --metric/--value or --readings"))
	}
	diags := feel.Decode(rmap, nil)
	if *format == "json" {
		return exitJSON(a, diags)
	}
	if len(diags) == 0 {
		fmt.Fprintln(a.Stdout, "无匹配规则（检查 metric 名，可用 gnd feel list）")
		return 0
	}
	for _, d := range diags {
		status := "OK"
		if !d.Healthy {
			status = "BAD"
		}
		fmt.Fprintf(a.Stdout, "[%s] %s (%s/%s)\n  %s\n",
			status, d.Phrase, d.Category, d.Metric, d.Detail)
		if d.Note != "" {
			fmt.Fprintf(a.Stdout, "  note: %s\n", d.Note)
		}
	}
	return 0
}

func (a *App) feelEncode(args []string) int {
	fs := a.newFlagSet("feel encode")
	phrase := fs.String("phrase", "", "free-text designer phrase")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(*phrase) == "" && fs.NArg() > 0 {
		*phrase = strings.Join(fs.Args(), " ")
	}
	hits := feel.Encode(*phrase, nil)
	if len(hits) == 0 {
		fmt.Fprintf(a.Stdout, "未匹配到规则：%q\n可先 gnd feel list 查看内置表，或在 feel.DefaultMappings 扩展。\n", *phrase)
		return 0
	}
	rows := make([][]string, len(hits))
	for i, m := range hits {
		rows[i] = []string{m.Phrase, m.Metric, m.Unit, m.Direction, m.Note}
	}
	_ = report.Table(a.Stdout, []string{"phrase", "metric", "unit", "dir", "note"}, rows)
	return 0
}

func (a *App) cmdValidate(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd validate table --file data.csv
  CSV header: id,name,field1,field2,...
  --cliff 0.5          relative step warn threshold
  --monotonic exp,hp   fields that must be non-decreasing by id
  --allow-negative
  --max-safe 2147483647
  --format table|json
`)
		return 0
	}
	if args[0] != "table" {
		fmt.Fprintf(a.Stderr, "unknown validate subcommand %q\n", args[0])
		return 2
	}
	fs := a.newFlagSet("validate table")
	file := fs.String("file", "", "csv path (or - for stdin)")
	cliff := fs.Float64("cliff", 0.5, "cliff threshold")
	mono := fs.String("monotonic", "", "comma fields")
	allowNeg := fs.Bool("allow-negative", false, "allow negative values")
	maxSafe := fs.Float64("max-safe", 2147483647, "max safe numeric")
	format := fs.String("format", "table", "table|json")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	if *file == "" {
		return a.fail(fmt.Errorf("--file is required"))
	}
	var data []byte
	var err error
	if *file == "-" {
		data, err = readAll(a)
	} else {
		data, err = os.ReadFile(*file)
	}
	if err != nil {
		return a.fail(err)
	}
	rows, err := validate.ParseCSVTable(string(data))
	if err != nil {
		return a.fail(err)
	}
	opt := validate.Options{
		GrowthCliff:   *cliff,
		AllowNegative: *allowNeg,
		MaxSafe:       *maxSafe,
		Monotonic:     splitCSV(*mono),
	}
	issues := validate.Table(rows, opt)
	if *format == "json" {
		return exitJSON(a, map[string]any{"summary": validate.Summarize(issues), "issues": issues})
	}
	fmt.Fprintf(a.Stdout, "rows=%d  %s\n", len(rows), validate.Summarize(issues))
	for _, is := range issues {
		fmt.Fprintf(a.Stdout, "  [%s] id=%d field=%s %s\n",
			strings.ToUpper(is.Severity), is.RowID, is.Field, is.Message)
	}
	if validate.Summarize(issues) == "error=0 warn=0 info=0" {
		fmt.Fprintln(a.Stdout, "OK 未发现数值异常")
	}
	return 0
}

func readAll(a *App) ([]byte, error) {
	var buf []byte
	tmp := make([]byte, 4096)
	for {
		n, err := os.Stdin.Read(tmp)
		if n > 0 {
			buf = append(buf, tmp[:n]...)
		}
		if err != nil {
			if err.Error() == "EOF" {
				return buf, nil
			}
			// treat EOF-like as done
			if n == 0 {
				return buf, nil
			}
		}
	}
}

func (a *App) cmdRecipe(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd recipe <list|show|search|tags>
  list
  show <id>
  search <query>
  tags
`)
		return 0
	}
	switch args[0] {
	case "list":
		for _, r := range recipe.All() {
			fmt.Fprintf(a.Stdout, "%-22s %s\n", r.ID, r.Title)
		}
		return 0
	case "show":
		if len(args) < 2 {
			return a.fail(fmt.Errorf("need recipe id"))
		}
		r, err := recipe.Get(args[1])
		if err != nil {
			return a.fail(err)
		}
		_ = report.KeyValue(a.Stdout, [][2]string{
			{"id", r.ID},
			{"title", r.Title},
			{"goal", r.Goal},
			{"model", r.Model},
			{"params", r.Params},
			{"anchors", r.Anchors},
			{"pitfalls", r.Pitfalls},
			{"tags", strings.Join(r.Tags, ", ")},
			{"example", r.Example},
		})
		return 0
	case "search":
		q := ""
		if len(args) > 1 {
			q = strings.Join(args[1:], " ")
		}
		hits := recipe.Search(q)
		if len(hits) == 0 {
			fmt.Fprintf(a.Stdout, "no recipes match %q\n", q)
			return 0
		}
		for _, r := range hits {
			fmt.Fprintf(a.Stdout, "%-22s %s\n    goal: %s\n", r.ID, r.Title, r.Goal)
		}
		return 0
	case "tags":
		fmt.Fprintln(a.Stdout, strings.Join(recipe.Tags(), ", "))
		return 0
	default:
		fmt.Fprintf(a.Stderr, "unknown recipe subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) cmdNDD(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd ndd new --name "系统名" --system combat|economy|gacha|progress
  --out ndd.md
`)
		return 0
	}
	if args[0] != "new" {
		fmt.Fprintf(a.Stderr, "unknown ndd subcommand %q\n", args[0])
		return 2
	}
	fs := a.newFlagSet("ndd new")
	name := fs.String("name", "未命名系统", "system name")
	system := fs.String("system", "combat", "combat|economy|gacha|progress")
	out := fs.String("out", "", "output file (default stdout)")
	if err := fs.Parse(args[1:]); err != nil {
		return 2
	}
	doc := nddTemplate(*name, *system)
	if *out == "" {
		fmt.Fprint(a.Stdout, doc)
		return 0
	}
	if err := os.WriteFile(*out, []byte(doc), 0o644); err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "wrote %s\n", *out)
	return 0
}

func nddTemplate(name, system string) string {
	return fmt.Sprintf(`# 数值设计文档（NDD）— %s

> system: %s
> status: draft
> owner:
> last_updated:

## 1. 设计目标
- 玩家体验目标（可验证）：
- 业务目标（留存/付费/时长）：
- 不做什么：

## 2. 核心循环
- 输入资源：
- 转化：
- 输出/消耗出口：
- 闭环图（文字即可）：

## 3. 锚点值（必须有依据，禁止拍脑袋）
| 锚点 | 值 | 单位 | 依据 |
|------|----|------|------|
|  |  |  |  |

## 4. 公式与参数
- 公式：
- 参数表：
| 参数 | 默认 | 下限 | 上限 | 敏感度(高/中/低) |
|------|------|------|------|------------------|
|  |  |  |  |  |

## 5. 成长/难度曲线
- 模型：（linear / exp / log / power / sigmoid / piecewise-log）
- 曲线图或采样表：
- 平滑度结论：

## 6. 模拟验证
- 战斗/经济/抽卡模拟命令与结论：
- 边界 case（全满 / 全零）：
- 胜率 / 通关率 / 出货分布：

## 7. 手感量化验收
| 模糊描述 | 指标 | 健康带 | 实测 |
|----------|------|--------|------|
|  |  |  |  |

## 8. 评审 Checklist
- [ ] 锚点值有依据
- [ ] 极端属性安全
- [ ] 成长曲线平滑（无断崖）
- [ ] 经济闭环完整（产出有消耗出口）
- [ ] 付费不破坏免费体验
- [ ] 数值膨胀速度可控（3 个月视角）

## 9. 上线监控
- 核心指标与预警阈值：
- 看板链接：

## 10. 变更记录
| 日期 | 变更 | 原因 | 结果 |
|------|------|------|------|
|  |  |  |  |
`, name, system)
}
