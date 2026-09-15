package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/neko233-com/game-numerical-design-cli/internal/calllog"
	"github.com/neko233-com/game-numerical-design-cli/internal/report"
)

func (a *App) cmdLogs(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd logs <list|show|search|stats|clear|path>

调用日志（默认 .gnd/logs/calls.jsonl，LRU 上限 5MB，已 gitignore）。
Agent 排查：不必重跑，直接看上次 argv / exit / stdout 尾部。

list    最近调用
  --limit 20 --cmd apply --fail-only --json

show    按 id 前缀查看完整条目（含输出尾部）
  show <id-prefix>

search  关键词搜 argv/stdout/stderr
  --contains "error" --limit 10

stats   体积 / 条数 / 失败数
clear   清空日志
path    打印日志文件路径

环境变量:
  GND_LOG=0          关闭日志
  GND_LOG_DIR=path   自定义目录
`)
		return 0
	}
	lg := calllog.New(calllog.Options{})
	switch args[0] {
	case "path":
		fmt.Fprintln(a.Stdout, lg.Path())
		return 0
	case "stats":
		return a.logsStats(lg, args[1:])
	case "clear":
		if err := lg.Clear(); err != nil {
			return a.fail(err)
		}
		fmt.Fprintf(a.Stdout, "cleared %s\n", lg.Path())
		return 0
	case "show":
		return a.logsShow(lg, args[1:])
	case "search":
		return a.logsSearch(lg, args[1:])
	case "list":
		return a.logsList(lg, args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown logs subcommand %q\n", args[0])
		return 2
	}
}

func (a *App) logsList(lg *calllog.Logger, args []string) int {
	fs := a.newFlagSet("logs list")
	limit := fs.Int("limit", 20, "max rows")
	cmd := fs.String("cmd", "", "filter by command")
	failOnly := fs.Bool("fail-only", false, "only exit!=0")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	entries, err := lg.Search(calllog.Query{Cmd: *cmd, Limit: 0})
	if err != nil {
		return a.fail(err)
	}
	if *failOnly {
		var f []calllog.Entry
		for _, e := range entries {
			if e.Exit != 0 {
				f = append(f, e)
			}
		}
		entries = f
	}
	if *limit > 0 && len(entries) > *limit {
		entries = entries[:*limit]
	}
	if *format == "json" {
		return exitJSON(a, entries)
	}
	if len(entries) == 0 {
		fmt.Fprintln(a.Stdout, "no call logs (or GND_LOG=0)")
		return 0
	}
	rows := make([][]string, len(entries))
	for i, e := range entries {
		rows[i] = []string{
			e.ID, e.TS.Local().Format("01-02 15:04:05"),
			strconv.Itoa(e.Exit), fmt.Sprintf("%d", e.DurationMS),
			strings.Join(e.Argv, " "),
		}
	}
	_ = report.Table(a.Stdout, []string{"id", "time", "exit", "ms", "argv"}, rows)
	fmt.Fprintf(a.Stdout, "\nstore: %s\nshow: gnd logs show <id-prefix>\n", lg.Path())
	return 0
}

func (a *App) logsShow(lg *calllog.Logger, args []string) int {
	if len(args) < 1 {
		return a.fail(fmt.Errorf("need id prefix"))
	}
	e, err := lg.Get(args[0])
	if err != nil {
		return a.fail(err)
	}
	_ = report.KeyValue(a.Stdout, [][2]string{
		{"id", e.ID},
		{"ts", e.TS.Local().Format(time.RFC3339)},
		{"cmd", e.Cmd},
		{"argv", strings.Join(e.Argv, " ")},
		{"cwd", e.Cwd},
		{"exit", strconv.Itoa(e.Exit)},
		{"duration_ms", strconv.FormatInt(e.DurationMS, 10)},
		{"dir_flag", e.DirFlag},
		{"goal_flag", e.GoalFlag},
		{"preset", e.Preset},
	})
	if e.StdoutTail != "" {
		fmt.Fprintln(a.Stdout, "\n--- stdout tail ---")
		fmt.Fprintln(a.Stdout, e.StdoutTail)
	}
	if e.StderrTail != "" {
		fmt.Fprintln(a.Stdout, "\n--- stderr tail ---")
		fmt.Fprintln(a.Stdout, e.StderrTail)
	}
	return 0
}

func (a *App) logsSearch(lg *calllog.Logger, args []string) int {
	fs := a.newFlagSet("logs search")
	contains := fs.String("contains", "", "keyword")
	limit := fs.Int("limit", 10, "max rows")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if *contains == "" {
		return a.fail(fmt.Errorf("need --contains"))
	}
	entries, err := lg.Search(calllog.Query{Contains: *contains, Limit: *limit})
	if err != nil {
		return a.fail(err)
	}
	if len(entries) == 0 {
		fmt.Fprintf(a.Stdout, "no matches for %q\n", *contains)
		return 0
	}
	rows := make([][]string, len(entries))
	for i, e := range entries {
		rows[i] = []string{e.ID, strconv.Itoa(e.Exit), strings.Join(e.Argv, " ")}
	}
	_ = report.Table(a.Stdout, []string{"id", "exit", "argv"}, rows)
	return 0
}

func (a *App) logsStats(lg *calllog.Logger, args []string) int {
	fs := a.newFlagSet("logs stats")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	st, err := lg.Stats()
	if err != nil {
		return a.fail(err)
	}
	if *format == "json" {
		return exitJSON(a, st)
	}
	_ = report.KeyValue(a.Stdout, [][2]string{
		{"path", st.Path},
		{"bytes", fmt.Sprintf("%d / %d (cap 5MB)", st.Bytes, st.MaxBytes)},
		{"entries", strconv.Itoa(st.Entries)},
		{"failures", strconv.Itoa(st.FailCount)},
		{"oldest", st.OldestTS},
		{"newest", st.NewestTS},
	})
	return 0
}
