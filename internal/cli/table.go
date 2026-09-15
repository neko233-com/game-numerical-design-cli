package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/neko233-com/game-numerical-design-cli/internal/report"
	"github.com/neko233-com/game-numerical-design-cli/internal/tablekit"
	"github.com/neko233-com/game-numerical-design-cli/internal/validate"
)

func (a *App) cmdTable(args []string) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" {
		fmt.Fprint(a.Stdout, `usage: gnd table <convert|schema|get|rows|set|add|upsert|batch|sheets> [flags]

Formats: xlsx | csv | tsv | json | yaml  (detected from extension)

convert   <src> <dst>          跨格式转换
  --sheet NAME          xlsx sheet
  --data-start N        xlsx 表头物理行（默认 1；五行表头可设 3/5）

schema    <file>               打印表头与行数
get       <file> --id 1001 --field atk
rows      <file> [--id X] [--count N] [--start N]
set       <file> --id 1001 --field atk --value 800 [--expected 700] [--write]
add       <file> --values '{"id":"1002","hp":"100"}' [--write]
upsert    <file> --values '{"id":"1002","atk":"90"}' [--write]
batch     <file> --input changes.json [--write]
sheets    <file.xlsx>          列出非空 sheet

默认 dry-run（只打印变更计划）；加 --write 才落盘。
--out PATH  写到新文件而不是原地修改。
`)
		return 0
	}
	switch args[0] {
	case "convert":
		return a.tableConvert(args[1:])
	case "schema":
		return a.tableSchema(args[1:])
	case "get":
		return a.tableGet(args[1:])
	case "rows":
		return a.tableRows(args[1:])
	case "set":
		return a.tableSet(args[1:])
	case "add":
		return a.tableAdd(args[1:])
	case "upsert":
		return a.tableUpsert(args[1:])
	case "batch":
		return a.tableBatch(args[1:])
	case "sheets":
		return a.tableSheets(args[1:])
	default:
		fmt.Fprintf(a.Stderr, "unknown table subcommand %q\n", args[0])
		return 2
	}
}

// parseTableFlags allows a positional file path before flags.
// stdlib flag stops at the first non-flag, so non-flags are moved to the end.
func parseTableFlags(fs *flag.FlagSet, args []string) error {
	var flags, positionals []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}
		flags = append(flags, arg)
		if strings.Contains(arg, "=") || i+1 >= len(args) || strings.HasPrefix(args[i+1], "-") {
			continue
		}
		name := strings.TrimLeft(arg, "-")
		if f := fs.Lookup(name); f != nil && !isBoolFlag(f) {
			flags = append(flags, args[i+1])
			i++
		}
	}
	return fs.Parse(append(flags, positionals...))
}

func isBoolFlag(f *flag.Flag) bool {
	type boolFlag interface{ IsBoolFlag() bool }
	if bf, ok := f.Value.(boolFlag); ok {
		return bf.IsBoolFlag()
	}
	return false
}

func (a *App) tableConvert(args []string) int {
	fs := a.newFlagSet("table convert")
	opt := a.bindTableIO(fs)
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	rest := fs.Args()
	if len(rest) != 2 {
		return a.fail(fmt.Errorf("need <src> <dst>"))
	}
	t, err := tablekit.Convert(rest[0], rest[1], *opt)
	if err != nil {
		return a.fail(err)
	}
	if *format == "json" {
		return exitJSON(a, map[string]any{
			"src": rest[0], "dst": rest[1],
			"headers": t.Headers, "rowCount": len(t.Rows),
		})
	}
	fmt.Fprintf(a.Stdout, "converted %s → %s  (%d cols × %d rows)\n",
		rest[0], rest[1], len(t.Headers), len(t.Rows))
	return 0
}

func (a *App) tableSchema(args []string) int {
	fs := a.newFlagSet("table schema")
	opt := a.bindTableIO(fs)
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		return a.fail(fmt.Errorf("need file path"))
	}
	path := fs.Arg(0)
	t, err := tablekit.Load(path, *opt)
	if err != nil {
		return a.fail(err)
	}
	if *format == "json" {
		return exitJSON(a, map[string]any{
			"file": path, "name": t.Name,
			"headers": t.Headers, "rowCount": len(t.Rows),
		})
	}
	fmt.Fprintf(a.Stdout, "file: %s\nrows: %d\ncolumns (%d):\n", path, len(t.Rows), len(t.Headers))
	for i, h := range t.Headers {
		sample := ""
		if len(t.Rows) > 0 && i < len(t.Rows[0]) {
			sample = t.Rows[0][i]
		}
		fmt.Fprintf(a.Stdout, "  %2d  %-20s  e.g. %s\n", i, h, sample)
	}
	return 0
}

func (a *App) tableGet(args []string) int {
	fs := a.newFlagSet("table get")
	opt := a.bindTableIO(fs)
	id := fs.String("id", "", "row id")
	field := fs.String("field", "", "field name")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 || *id == "" || *field == "" {
		return a.fail(fmt.Errorf("need file --id --field"))
	}
	t, err := tablekit.Load(fs.Arg(0), *opt)
	if err != nil {
		return a.fail(err)
	}
	v, err := t.GetCell(*id, *field, *opt)
	if err != nil {
		return a.fail(err)
	}
	if *format == "json" {
		return exitJSON(a, map[string]string{"id": *id, "field": *field, "value": v})
	}
	fmt.Fprintf(a.Stdout, "%s.%s = %s\n", *id, *field, v)
	return 0
}

func (a *App) tableRows(args []string) int {
	fs := a.newFlagSet("table rows")
	opt := a.bindTableIO(fs)
	id := fs.String("id", "", "filter by id")
	count := fs.Int("count", 20, "max rows to print")
	start := fs.Int("start", 0, "skip first N data rows")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 {
		return a.fail(fmt.Errorf("need file path"))
	}
	t, err := tablekit.Load(fs.Arg(0), *opt)
	if err != nil {
		return a.fail(err)
	}
	var rows [][]string
	if *id != "" {
		ri, err := t.FindRow(*id, *opt)
		if err != nil {
			return a.fail(err)
		}
		rows = [][]string{t.Rows[ri]}
	} else {
		for i := *start; i < len(t.Rows) && len(rows) < *count; i++ {
			rows = append(rows, t.Rows[i])
		}
	}
	if *format == "json" {
		var recs []map[string]string
		for _, r := range rows {
			m := map[string]string{}
			for i, h := range t.Headers {
				if i < len(r) {
					m[h] = r[i]
				}
			}
			recs = append(recs, m)
		}
		return exitJSON(a, recs)
	}
	if err := report.Table(a.Stdout, t.Headers, rows); err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "\n(%d of %d rows)\n", len(rows), len(t.Rows))
	return 0
}

func (a *App) tableSet(args []string) int {
	fs := a.newFlagSet("table set")
	opt := a.bindTableIO(fs)
	id := fs.String("id", "", "row id")
	field := fs.String("field", "", "field name")
	value := fs.String("value", "", "new value")
	expected := fs.String("expected", "", "required current value (optimistic lock)")
	write := fs.Bool("write", false, "apply changes to disk (default dry-run)")
	out := fs.String("out", "", "write result to this path instead of in-place")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 || *id == "" || *field == "" {
		return a.fail(fmt.Errorf("need file --id --field --value"))
	}
	path := fs.Arg(0)
	t, err := tablekit.Load(path, *opt)
	if err != nil {
		return a.fail(err)
	}
	var ch tablekit.Change
	if *expected != "" {
		ch, err = t.SetCellExpected(*id, *field, *value, *expected, *opt)
	} else {
		ch, err = t.SetCell(*id, *field, *value, *opt)
	}
	if err != nil {
		return a.fail(err)
	}
	return a.finishTableEdit(t, path, []tablekit.Change{ch}, *write, *out, *format)
}

func (a *App) tableAdd(args []string) int {
	fs := a.newFlagSet("table add")
	opt := a.bindTableIO(fs)
	valuesJSON := fs.String("values", "", `JSON object e.g. {"id":"1002","hp":"100"}`)
	write := fs.Bool("write", false, "apply to disk")
	out := fs.String("out", "", "write to path")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 || strings.TrimSpace(*valuesJSON) == "" {
		return a.fail(fmt.Errorf("need file --values JSON"))
	}
	path := fs.Arg(0)
	t, err := tablekit.Load(path, *opt)
	if err != nil {
		return a.fail(err)
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(*valuesJSON), &values); err != nil {
		return a.fail(fmt.Errorf("bad --values JSON: %w", err))
	}
	idx := t.AddRow(values)
	ch := tablekit.Change{RowIndex: idx, Column: "*", NewValue: "new row"}
	if idField, _, err := t.ResolveIDField(opt.IDField); err == nil {
		if v, ok := values[idField]; ok {
			ch.ID = v
		}
	}
	return a.finishTableEdit(t, path, []tablekit.Change{ch}, *write, *out, *format)
}

func (a *App) tableUpsert(args []string) int {
	fs := a.newFlagSet("table upsert")
	opt := a.bindTableIO(fs)
	valuesJSON := fs.String("values", "", `JSON object with id field`)
	write := fs.Bool("write", false, "apply to disk")
	out := fs.String("out", "", "write to path")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 || strings.TrimSpace(*valuesJSON) == "" {
		return a.fail(fmt.Errorf("need file --values JSON"))
	}
	path := fs.Arg(0)
	t, err := tablekit.Load(path, *opt)
	if err != nil {
		return a.fail(err)
	}
	var values map[string]string
	if err := json.Unmarshal([]byte(*valuesJSON), &values); err != nil {
		return a.fail(fmt.Errorf("bad --values JSON: %w", err))
	}
	idx, created, changes, err := t.UpsertRow(values, *opt)
	if err != nil {
		return a.fail(err)
	}
	if created {
		idField, _, _ := t.ResolveIDField(opt.IDField)
		changes = []tablekit.Change{{
			RowIndex: idx, Column: "*", NewValue: "created row", ID: values[idField],
		}}
	}
	if len(changes) == 0 {
		fmt.Fprintf(a.Stdout, "no-op: row already matches %s\n", path)
		return 0
	}
	return a.finishTableEdit(t, path, changes, *write, *out, *format)
}

type batchChange struct {
	ID       string `json:"id"`
	Field    string `json:"field"`
	Value    string `json:"value"`
	Expected string `json:"expected,omitempty"`
}

type batchInput struct {
	Changes []batchChange       `json:"changes"`
	Upserts []map[string]string `json:"upserts"`
}

func (a *App) tableBatch(args []string) int {
	fs := a.newFlagSet("table batch")
	opt := a.bindTableIO(fs)
	inputPath := fs.String("input", "", "batch JSON path (or - for stdin)")
	write := fs.Bool("write", false, "apply to disk")
	out := fs.String("out", "", "write to path")
	format := fs.String("format", "table", "table|json")
	if err := parseTableFlags(fs, args); err != nil {
		return 2
	}
	if fs.NArg() < 1 || *inputPath == "" {
		return a.fail(fmt.Errorf("need file --input"))
	}
	path := fs.Arg(0)
	t, err := tablekit.Load(path, *opt)
	if err != nil {
		return a.fail(err)
	}
	raw, err := readInputPath(*inputPath)
	if err != nil {
		return a.fail(err)
	}
	var bin batchInput
	if err := json.Unmarshal(raw, &bin); err != nil {
		return a.fail(fmt.Errorf("bad batch JSON: %w", err))
	}
	var changes []tablekit.Change
	for i, c := range bin.Changes {
		var ch tablekit.Change
		if c.Expected != "" {
			ch, err = t.SetCellExpected(c.ID, c.Field, c.Value, c.Expected, *opt)
		} else {
			ch, err = t.SetCell(c.ID, c.Field, c.Value, *opt)
		}
		if err != nil {
			return a.fail(fmt.Errorf("change[%d] id=%s field=%s: %w", i, c.ID, c.Field, err))
		}
		changes = append(changes, ch)
	}
	for i, u := range bin.Upserts {
		_, created, chs, err := t.UpsertRow(u, *opt)
		if err != nil {
			return a.fail(fmt.Errorf("upsert[%d]: %w", i, err))
		}
		if created {
			idField, _, _ := t.ResolveIDField(opt.IDField)
			chs = []tablekit.Change{{Column: "*", NewValue: "created row", ID: u[idField]}}
		}
		changes = append(changes, chs...)
	}
	if len(changes) == 0 {
		return a.fail(fmt.Errorf("batch input has no changes/upserts"))
	}
	return a.finishTableEdit(t, path, changes, *write, *out, *format)
}

func (a *App) tableSheets(args []string) int {
	if len(args) < 1 {
		return a.fail(fmt.Errorf("need xlsx path"))
	}
	names, err := tablekit.SheetNames(args[0])
	if err != nil {
		return a.fail(err)
	}
	for _, n := range names {
		fmt.Fprintln(a.Stdout, n)
	}
	return 0
}

func (a *App) finishTableEdit(t *tablekit.Table, srcPath string, changes []tablekit.Change, write bool, out, format string) int {
	status := "dry-run"
	if write {
		status = "write"
	}
	target := srcPath
	if out != "" {
		target = out
	}
	if format == "json" {
		payload := map[string]any{
			"status": status, "source": srcPath, "target": target,
			"changes": changes, "rowCount": len(t.Rows),
		}
		if write {
			if err := t.Save(target, tablekit.Options{}); err != nil {
				return a.fail(err)
			}
			payload["saved"] = true
		}
		return exitJSON(a, payload)
	}

	fmt.Fprintf(a.Stdout, "[%s] %s\n", status, srcPath)
	rows := make([][]string, len(changes))
	for i, c := range changes {
		rows[i] = []string{strconv.Itoa(c.RowIndex), c.ID, c.Column, c.OldValue, c.NewValue}
	}
	_ = report.Table(a.Stdout, []string{"row", "id", "field", "old", "new"}, rows)
	if !write {
		fmt.Fprintln(a.Stdout, "\ndry-run only — re-run with --write to apply")
		return 0
	}
	if err := t.Save(target, tablekit.Options{}); err != nil {
		return a.fail(err)
	}
	fmt.Fprintf(a.Stdout, "\nsaved %s (%d changes)\n", target, len(changes))
	return 0
}

func readInputPath(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

func (a *App) bindTableIO(fs *flag.FlagSet) *tablekit.Options {
	opt := &tablekit.Options{}
	fs.StringVar(&opt.Sheet, "sheet", "", "xlsx sheet name")
	fs.IntVar(&opt.DataStartRow, "data-start", 1, "xlsx header row (1-based)")
	fs.StringVar(&opt.IDField, "id-field", "", "id column name (default: id/Id/ID or first col)")
	return opt
}

func tableToValidateRows(t *tablekit.Table) []validate.Row {
	idField, _, _ := t.ResolveIDField("")
	var rows []validate.Row
	for i, r := range t.Rows {
		fields := map[string]float64{}
		var id int64
		var name string
		for ci, h := range t.Headers {
			val := ""
			if ci < len(r) {
				val = r[ci]
			}
			if h == idField {
				if v, err := strconv.ParseFloat(val, 64); err == nil {
					id = int64(v)
				} else {
					id = int64(i + 1)
				}
				continue
			}
			if h == "name" {
				name = val
				continue
			}
			if val == "" {
				continue
			}
			if v, err := strconv.ParseFloat(val, 64); err == nil {
				fields[h] = v
			}
		}
		rows = append(rows, validate.Row{ID: id, Name: name, Fields: fields})
	}
	return rows
}
