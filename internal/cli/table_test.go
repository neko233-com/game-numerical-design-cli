package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTableConvertJSONCSV(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "in.json")
	dst := filepath.Join(dir, "out.csv")
	if err := os.WriteFile(src, []byte(`[{"id":"1","hp":"100"},{"id":"2","hp":"220"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errS, code := run(t, "table", "convert", src, dst)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "converted") {
		t.Fatalf("out=%s", out)
	}
	data, _ := os.ReadFile(dst)
	if !strings.Contains(string(data), "id,hp") && !strings.Contains(string(data), "id") {
		t.Fatalf("csv=%s", data)
	}
}

func TestTableSetDryRunAndWrite(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "lv.csv")
	if err := os.WriteFile(src, []byte("id,name,atk\n1,A,100\n2,B,200\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errS, code := run(t, "table", "set", src, "--id", "2", "--field", "atk", "--value", "250")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	if !strings.Contains(out, "dry-run") {
		t.Fatalf("out=%s", out)
	}
	// file unchanged
	data, _ := os.ReadFile(src)
	if strings.Contains(string(data), "250") {
		t.Fatal("dry-run modified file")
	}
	_, errS, code = run(t, "table", "set", src, "--id", "2", "--field", "atk", "--value", "250", "--write")
	if code != 0 {
		t.Fatalf("write code=%d err=%s", code, errS)
	}
	data, _ = os.ReadFile(src)
	if !strings.Contains(string(data), "250") {
		t.Fatalf("file not updated: %s", data)
	}
}

func TestTableSchemaAndXLSX(t *testing.T) {
	dir := t.TempDir()
	csv := filepath.Join(dir, "a.csv")
	xlsx := filepath.Join(dir, "a.xlsx")
	if err := os.WriteFile(csv, []byte("id,name,exp\n1,Lv1,100\n2,Lv2,220\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, errS, code := run(t, "table", "convert", csv, xlsx); code != 0 {
		t.Fatalf("convert code=%d %s", code, errS)
	}
	out, errS, code := run(t, "table", "schema", xlsx)
	if code != 0 {
		t.Fatalf("schema code=%d %s", code, errS)
	}
	if !strings.Contains(out, "exp") {
		t.Fatalf("schema=%s", out)
	}
	out, errS, code = run(t, "table", "get", xlsx, "--id", "2", "--field", "exp")
	if code != 0 {
		t.Fatalf("get code=%d %s", code, errS)
	}
	if !strings.Contains(out, "220") {
		t.Fatalf("get=%s", out)
	}
}

func TestValidateJSON(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "lv.json")
	content := `[{"id":"1","exp":"100","hp":"500"},{"id":"2","exp":"400","hp":"1100"}]`
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errS, code := run(t, "validate", "table", "--file", p, "--cliff", "0.5")
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errS)
	}
	// 100→400 is 200% jump → cliff warn
	if !strings.Contains(out, "跳变") && !strings.Contains(out, "断崖") && !strings.Contains(out, "warn") {
		t.Fatalf("expected cliff warn, out=%s", out)
	}
}

func TestTableBatch(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "lv.csv")
	batch := filepath.Join(dir, "ch.json")
	if err := os.WriteFile(src, []byte("id,atk,hp\n1,100,500\n2,200,800\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(batch, []byte(`{"changes":[{"id":"1","field":"atk","value":"150"},{"id":"2","field":"hp","value":"900"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	out, errS, code := run(t, "table", "batch", src, "--input", batch, "--write")
	if code != 0 {
		t.Fatalf("code=%d err=%s out=%s", code, errS, out)
	}
	data, _ := os.ReadFile(src)
	if !strings.Contains(string(data), "150") || !strings.Contains(string(data), "900") {
		t.Fatalf("batch not applied: %s", data)
	}
}
