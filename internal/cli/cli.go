// Package cli implements the gnd subcommand surface using stdlib only.
package cli

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
)

// Version of the CLI.
const Version = "0.1.0"

// App is the root command dispatcher.
type App struct {
	Stdout io.Writer
	Stderr io.Writer
	Args   []string
}

// Run parses args and dispatches. Returns process exit code.
func (a *App) Run() int {
	if a.Stdout == nil {
		a.Stdout = os.Stdout
	}
	if a.Stderr == nil {
		a.Stderr = os.Stderr
	}
	args := a.Args
	if len(args) == 0 {
		a.printRootHelp()
		return 0
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		if len(rest) > 0 {
			return a.helpTopic(rest[0])
		}
		a.printRootHelp()
		return 0
	case "version", "-v", "--version":
		fmt.Fprintf(a.Stdout, "gnd %s\n", Version)
		return 0
	case "curve":
		return a.cmdCurve(rest)
	case "combat":
		return a.cmdCombat(rest)
	case "economy":
		return a.cmdEconomy(rest)
	case "gacha":
		return a.cmdGacha(rest)
	case "balance":
		return a.cmdBalance(rest)
	case "feel":
		return a.cmdFeel(rest)
	case "table":
		return a.cmdTable(rest)
	case "demo":
		return a.cmdDemo(rest)
	case "apply":
		return a.cmdApply(rest)
	case "script":
		return a.cmdScript(rest)
	case "sim":
		return a.cmdSim(rest)
	case "report":
		return a.cmdReport(rest)
	case "validate":
		return a.cmdValidate(rest)
	case "recipe":
		return a.cmdRecipe(rest)
	case "ndd":
		return a.cmdNDD(rest)
	default:
		fmt.Fprintf(a.Stderr, "unknown command %q\n\n", cmd)
		a.printRootHelp()
		return 2
	}
}

func (a *App) printRootHelp() {
	fmt.Fprint(a.Stdout, `gnd — game numerical design CLI

Usage:
  gnd <command> [subcommand] [flags]

Commands:
  curve      成长曲线采样 / 平滑度检查 / 公式说明
  combat     伤害计算 + 1v1 蒙特卡洛战斗模拟
  economy    产出消耗比 / 存量推演 / 成长节奏 / 闭环检查
  gacha      抽卡分布模拟（软/硬保底、P50/P90、累计出货率）
  balance    锚点校验 / 敏感度分析 / 维度打分 / 胜率对照
  feel       手感模糊描述 ↔ 量化指标
  table      多格式配置表：xlsx/csv/tsv/json/yaml 读写转换与改值
  demo       目标驱动 demo：init/build/check/sim（多类型预设）
  apply      一句话配置+自校验（配新/改旧，改旧需确认）
  script     内嵌 TS/JS 脚本（esbuild+goja，无需装 Node）
  sim        并行战斗模拟器（≤1000 并发，每场日志，可查询历史）
  report     HTML 报告（内联 SVG 图表）：demo / sim / curve
  validate   数值表安全检查（csv/tsv/json/yaml/xlsx）
  recipe     数值「菜谱」库：目标 → 模型 → 参数区间
  ndd        生成数值设计文档（NDD）骨架
  version    打印版本

Use "gnd help <command>" for command-specific help.
Examples:
  gnd curve exponential --a 100 --b 0.05 --max-x 60 --n 30
  gnd combat simulate --a-hp 5000 --a-atk 800 --d-hp 4000 --d-atk 600 --n 5000
  gnd economy balance --name gold --prod 1200 --cons 1000
  gnd gacha sim --rate 0.006 --hard 90 --pulls 80 --sessions 10000
  gnd feel decode --metric level_stat_gain_pct --value 5
  gnd recipe search gacha
`)
}

func (a *App) helpTopic(topic string) int {
	// re-dispatch with -h by invoking command help paths
	switch topic {
	case "curve":
		fmt.Fprint(a.Stdout, `gnd curve <linear|exponential|logarithmic|power|sigmoid|quadratic|piecewise-log> [flags]
  --a --b --c --l --k --x0 --base --s1 --s2 --break
  --max-x float   采样上限 (default 100)
  --n int         采样段数 (default 20)
  --format        table|csv|json (default table)
  --smooth        附加平滑度检查
`)
	case "combat":
		fmt.Fprint(a.Stdout, `gnd combat dmg|simulate|ehp
  dmg: 期望伤害
    --atk --def --skill --crit-rate --crit-mult --type physical|true|flat --mitigation
  simulate: 蒙特卡洛 1v1
    --a-hp --a-atk --a-def --a-crit-rate --a-crit-mult --a-speed --a-skill
    --d-hp --d-atk --d-def ...  --n --seed --format
`)
	case "economy":
		fmt.Fprint(a.Stdout, `gnd economy balance|project|pace|loop
`)
	case "gacha":
		fmt.Fprint(a.Stdout, `gnd gacha sim|expect|suggest
  sim: --rate --hard --soft-start --soft-step --pulls --sessions --seed --format
`)
	case "balance":
		fmt.Fprint(a.Stdout, `gnd balance anchors|sensitivity|dimensions|winrate
`)
	case "feel":
		fmt.Fprint(a.Stdout, `gnd feel list|decode|encode
`)
	case "table":
		fmt.Fprint(a.Stdout, `gnd table convert|schema|get|rows|set|add|upsert|batch|sheets
  Formats: xlsx/csv/tsv/json/yaml
  Default dry-run; add --write to apply. Use --sheet / --data-start for xlsx.
`)
	case "validate":
		fmt.Fprint(a.Stdout, `gnd validate table --file data.{csv,tsv,json,yaml,xlsx} [--sheet S] [--cliff 0.5] [--monotonic exp,hp]
`)
	case "recipe":
		fmt.Fprint(a.Stdout, `gnd recipe list|show <id>|search <q>|tags
`)
	case "ndd":
		fmt.Fprint(a.Stdout, `gnd ndd new --name "系统名" --system combat|economy|gacha|progress
`)
	default:
		fmt.Fprintf(a.Stderr, "unknown help topic %q\n", topic)
		return 2
	}
	return 0
}

// newFlagSet creates a FlagSet that prints to stderr and exits on error via ContinueOnError.
func (a *App) newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(a.Stderr)
	return fs
}

func (a *App) fail(err error) int {
	fmt.Fprintf(a.Stderr, "error: %v\n", err)
	return 1
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
