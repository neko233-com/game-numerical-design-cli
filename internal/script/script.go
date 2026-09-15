// Package script runs TypeScript/JavaScript numerical design scripts
// inside the process — no Node.js install required.
//
// Pipeline: TS/JS source → esbuild (transpile only) → goja VM.
// Host APIs are injected as the global `gnd`.
package script

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dop251/goja"
	"github.com/evanw/esbuild/pkg/api"
)

// Options tune execution.
type Options struct {
	// TimeoutMS for the whole script (0 = default 30s).
	TimeoutMS int
	// WorkingDir for relative file access inside host APIs.
	WorkingDir string
	// Globals extra key→value injected before run (JSON-serializable).
	Globals map[string]any
	// Host injects native functions onto the `gnd` object.
	Host HostAPI
}

// HostAPI is implemented by the CLI to expose numerical tools to scripts.
type HostAPI interface {
	// Curve evaluates a growth curve. kind: linear|exponential|logarithmic|power|sigmoid|quadratic|piecewise-log
	Curve(kind string, x float64, params map[string]float64) (float64, error)
	// Damage expected damage.
	Damage(in map[string]float64, typ string) (float64, error)
	// SimBattle runs n 1v1 fights, returns map with win_rate, avg_turns, ...
	SimBattle(attacker, defender map[string]any, n int, seed int64) (map[string]any, error)
	// LoadTable reads csv/tsv/json/yaml/xlsx into {headers, rows}.
	LoadTable(path string, sheet string) (map[string]any, error)
	// ReadJSON reads a JSON file into a JS value.
	ReadJSON(path string) (any, error)
	// WriteJSON writes a value as pretty JSON.
	WriteJSON(path string, v any) error
	// Log prints to CLI stdout.
	Log(args ...any)
}

// Result of a script run.
type Result struct {
	// Value is the script's completion value (last expression / explicit return).
	Value any
	// Logs collected from gnd.log
	Logs []string
}

// Run compiles and executes source (TS or JS detected by extension or content).
func Run(name, source string, opt Options) (*Result, error) {
	if opt.TimeoutMS <= 0 {
		opt.TimeoutMS = 30000
	}
	js, err := Transpile(name, source)
	if err != nil {
		return nil, err
	}
	return runJS(name, js, opt)
}

// RunFile loads and runs a .ts/.js/.mjs file.
func RunFile(path string, opt Options) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if opt.WorkingDir == "" {
		opt.WorkingDir = filepath.Dir(path)
	}
	return Run(path, string(data), opt)
}

// Transpile turns TS (or JS) into plain JS via esbuild. Identity for pure JS.
func Transpile(name, source string) (string, error) {
	lang := api.LoaderTS
	lower := strings.ToLower(name)
	switch {
	case strings.HasSuffix(lower, ".js"), strings.HasSuffix(lower, ".mjs"):
		lang = api.LoaderJS
	case strings.HasSuffix(lower, ".tsx"):
		lang = api.LoaderTSX
	case strings.HasSuffix(lower, ".jsx"):
		lang = api.LoaderJSX
	default:
		// sniff: if no types-ish syntax, still fine as TS
		lang = api.LoaderTS
	}
	result := api.Transform(source, api.TransformOptions{
		Loader:        lang,
		Format:        api.FormatCommonJS,
		Target:        api.ES2020,
		Sourcemap:     api.SourceMapNone,
		Sourcefile:    name,
		LegalComments: api.LegalCommentsNone,
	})
	if len(result.Errors) > 0 {
		var b strings.Builder
		for _, e := range result.Errors {
			loc := ""
			if e.Location != nil {
				loc = fmt.Sprintf("%s:%d:%d ", e.Location.File, e.Location.Line, e.Location.Column)
			}
			b.WriteString(loc + e.Text + "\n")
		}
		return "", fmt.Errorf("transpile %s:\n%s", name, b.String())
	}
	return string(result.Code), nil
}

func runJS(name, js string, opt Options) (*Result, error) {
	vm := goja.New()
	res := &Result{}

	// console.log
	console := vm.NewObject()
	_ = console.Set("log", func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, a := range call.Arguments {
			parts[i] = exportString(a)
		}
		line := strings.Join(parts, " ")
		res.Logs = append(res.Logs, line)
		if opt.Host != nil {
			opt.Host.Log(line)
		}
		return goja.Undefined()
	})
	_ = console.Set("error", console.Get("log"))
	_ = console.Set("warn", console.Get("log"))
	_ = vm.Set("console", console)

	// JSON is built into goja

	// gnd host object
	gnd := vm.NewObject()
	if opt.Host != nil {
		bindHost(vm, gnd, opt.Host, res)
	}
	if opt.Globals != nil {
		for k, v := range opt.Globals {
			_ = gnd.Set(k, v)
		}
	}
	// convenience constants
	_ = gnd.Set("cwd", opt.WorkingDir)
	_ = vm.Set("gnd", gnd)

	// CommonJS from esbuild assigns module.exports / exports.default
	mod := vm.NewObject()
	_ = vm.Set("module", mod)
	_ = vm.Set("exports", vm.NewObject())

	v, err := vm.RunString(js)
	if err != nil {
		return res, fmt.Errorf("run %s: %w", name, err)
	}
	if exp := mod.Get("exports"); exp != nil && !goja.IsUndefined(exp) && !goja.IsNull(exp) {
		if obj := exp.ToObject(vm); obj != nil {
			if d := obj.Get("default"); d != nil && !goja.IsUndefined(d) && !goja.IsNull(d) {
				res.Value = d.Export()
			} else if keys := obj.Keys(); len(keys) > 0 {
				res.Value = exp.Export()
			}
		}
	}
	if res.Value == nil && v != nil {
		res.Value = v.Export()
	}
	return res, nil
}

func bindHost(vm *goja.Runtime, gnd *goja.Object, host HostAPI, res *Result) {
	_ = gnd.Set("curve", func(kind string, x float64, params map[string]float64) (float64, error) {
		return host.Curve(kind, x, params)
	})
	_ = gnd.Set("damage", func(in map[string]float64, typ string) (float64, error) {
		return host.Damage(in, typ)
	})
	_ = gnd.Set("simBattle", func(attacker, defender map[string]any, n int, seed int64) (map[string]any, error) {
		return host.SimBattle(attacker, defender, n, seed)
	})
	_ = gnd.Set("loadTable", func(path, sheet string) (map[string]any, error) {
		return host.LoadTable(path, sheet)
	})
	_ = gnd.Set("readJSON", func(path string) (any, error) {
		return host.ReadJSON(path)
	})
	_ = gnd.Set("writeJSON", func(path string, v any) error {
		return host.WriteJSON(path, v)
	})
	_ = gnd.Set("log", func(call goja.FunctionCall) goja.Value {
		parts := make([]string, len(call.Arguments))
		for i, a := range call.Arguments {
			parts[i] = exportString(a)
		}
		line := strings.Join(parts, " ")
		res.Logs = append(res.Logs, line)
		host.Log(line)
		return goja.Undefined()
	})
}

func exportString(v goja.Value) string {
	if v == nil || goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	if s, ok := v.Export().(string); ok {
		return s
	}
	return v.String()
}
