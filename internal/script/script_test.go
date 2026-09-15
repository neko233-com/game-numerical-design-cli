package script

import (
	"strings"
	"testing"
)

type mockHost struct {
	logs []string
}

func (m *mockHost) Curve(kind string, x float64, params map[string]float64) (float64, error) {
	if kind == "linear" {
		return params["a"] + params["b"]*x, nil
	}
	return x, nil
}
func (m *mockHost) Damage(in map[string]float64, typ string) (float64, error) {
	return in["atk"] * 2, nil
}
func (m *mockHost) SimBattle(attacker, defender map[string]any, n int, seed int64) (map[string]any, error) {
	return map[string]any{"win_rate": 0.5, "n": float64(n)}, nil
}
func (m *mockHost) LoadTable(path string, sheet string) (map[string]any, error) {
	return map[string]any{"headers": []string{"id"}, "rows": [][]string{{"1"}}, "count": 1}, nil
}
func (m *mockHost) ReadJSON(path string) (any, error)  { return map[string]any{"ok": true}, nil }
func (m *mockHost) WriteJSON(path string, v any) error { return nil }
func (m *mockHost) Log(args ...any) {
	m.logs = append(m.logs, "logged")
}

func num(t *testing.T, v any) float64 {
	t.Helper()
	switch x := v.(type) {
	case float64:
		return x
	case int:
		return float64(x)
	case int64:
		return float64(x)
	default:
		t.Fatalf("not a number: %v (%T)", v, v)
		return 0
	}
}

func TestTranspileTS(t *testing.T) {
	src := `const x: number = 1 + 2; export default x;`
	js, err := Transpile("t.ts", src)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(js, ": number") {
		t.Fatalf("types not stripped: %s", js)
	}
}

func TestRunExportDefault(t *testing.T) {
	h := &mockHost{}
	res, err := Run("b.ts", `
interface Foo { a: number }
const f: Foo = { a: 42 };
export default f;
`, Options{Host: h})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("value type %T %v", res.Value, res.Value)
	}
	if num(t, m["a"]) != 42 {
		t.Fatalf("a=%v", m["a"])
	}
}

func TestRunCurveAndLog(t *testing.T) {
	h := &mockHost{}
	res, err := Run("a.ts", `
const y = gnd.curve("linear", 10, {a: 1, b: 2});
gnd.log("hi");
export default y;
`, Options{Host: h})
	if err != nil {
		t.Fatal(err)
	}
	if num(t, res.Value) != 21 {
		t.Fatalf("value=%v", res.Value)
	}
	if len(h.logs) == 0 && len(res.Logs) == 0 {
		t.Fatal("expected log")
	}
}

func TestSimBattleAPI(t *testing.T) {
	h := &mockHost{}
	res, err := Run("c.ts", `export default gnd.simBattle({hp:1000,atk:100}, {hp:1000,atk:100}, 50, 1)`, Options{Host: h})
	if err != nil {
		t.Fatal(err)
	}
	m, ok := res.Value.(map[string]any)
	if !ok {
		t.Fatalf("value type %T %v", res.Value, res.Value)
	}
	if num(t, m["win_rate"]) != 0.5 {
		t.Fatalf("win=%v", m["win_rate"])
	}
}

func TestTranspileError(t *testing.T) {
	if _, err := Transpile("bad.ts", `const x: = 1;`); err == nil {
		t.Fatal("expected transpile error")
	}
}
