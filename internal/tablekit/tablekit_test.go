package tablekit

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTemp(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestCSVRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := writeTemp(t, dir, "a.csv", "id,name,atk,hp\n1,剑士,100,500\n2,法师,80,400\n")
	tbl, err := Load(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(tbl.Headers) != 4 || len(tbl.Rows) != 2 {
		t.Fatalf("headers=%v rows=%d", tbl.Headers, len(tbl.Rows))
	}
	dst := filepath.Join(dir, "b.json")
	if err := tbl.Save(dst, Options{}); err != nil {
		t.Fatal(err)
	}
	back, err := Load(dst, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Rows) != 2 || back.Rows[0][0] != "1" || back.Rows[1][1] != "法师" {
		t.Fatalf("roundtrip rows=%v", back.Rows)
	}
}

func TestJSONToYAMLToTSV(t *testing.T) {
	dir := t.TempDir()
	src := writeTemp(t, dir, "a.json", `[{"id":"10","name":"A","atk":"5"},{"id":"11","name":"B","atk":"9"}]`)
	tbl, err := Load(src, Options{})
	if err != nil {
		t.Fatal(err)
	}
	yml := filepath.Join(dir, "a.yaml")
	tsv := filepath.Join(dir, "a.tsv")
	if err := tbl.Save(yml, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := tbl.Save(tsv, Options{}); err != nil {
		t.Fatal(err)
	}
	fromY, err := Load(yml, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if len(fromY.Rows) != 2 {
		t.Fatalf("yaml rows=%v", fromY.Rows)
	}
	// yaml headers+rows preserves load order
	if fromY.Headers[0] != tbl.Headers[0] {
		t.Fatalf("yaml headers=%v want %v", fromY.Headers, tbl.Headers)
	}
	fromT, err := Load(tsv, Options{})
	if err != nil {
		t.Fatal(err)
	}
	atkIdx := fromT.ColIndex("atk")
	if atkIdx < 0 || fromT.Rows[1][atkIdx] != "9" {
		t.Fatalf("tsv headers=%v rows=%v", fromT.Headers, fromT.Rows)
	}
}

func TestXLSXRoundTrip(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "lv.xlsx")
	tbl := &Table{
		Headers: []string{"id", "name", "exp"},
		Rows: [][]string{
			{"1", "Lv1", "100"},
			{"2", "Lv2", "220"},
		},
	}
	if err := tbl.Save(src, Options{Sheet: "等级"}); err != nil {
		t.Fatal(err)
	}
	names, err := SheetNames(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "等级" {
		t.Fatalf("sheets=%v", names)
	}
	back, err := Load(src, Options{Sheet: "等级"})
	if err != nil {
		t.Fatal(err)
	}
	if back.Rows[1][2] != "220" {
		t.Fatalf("rows=%v", back.Rows)
	}
	// convert to csv
	csvPath := filepath.Join(dir, "lv.csv")
	if _, err := Convert(src, csvPath, Options{Sheet: "等级"}); err != nil {
		t.Fatal(err)
	}
	c, err := Load(csvPath, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if c.Rows[0][1] != "Lv1" {
		t.Fatalf("csv from xlsx=%v", c.Rows)
	}
}

func TestSetCellExpected(t *testing.T) {
	tbl := &Table{
		Headers: []string{"id", "atk"},
		Rows:    [][]string{{"1", "100"}, {"2", "200"}},
	}
	ch, err := tbl.SetCellExpected("2", "atk", "250", "200", Options{})
	if err != nil {
		t.Fatal(err)
	}
	if ch.OldValue != "200" || ch.NewValue != "250" {
		t.Fatalf("change=%+v", ch)
	}
	if _, err := tbl.SetCellExpected("2", "atk", "999", "123", Options{}); err == nil {
		t.Fatal("expected lock mismatch error")
	}
}

func TestUpsertRow(t *testing.T) {
	tbl := &Table{
		Headers: []string{"id", "atk", "hp"},
		Rows:    [][]string{{"1", "100", "500"}},
	}
	idx, created, changes, err := tbl.UpsertRow(map[string]string{"id": "1", "atk": "150"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if created || idx != 0 || len(changes) != 1 || changes[0].NewValue != "150" {
		t.Fatalf("idx=%d created=%v changes=%+v", idx, created, changes)
	}
	idx2, created2, _, err := tbl.UpsertRow(map[string]string{"id": "9", "atk": "1", "hp": "1"}, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !created2 || idx2 != 1 || len(tbl.Rows) != 2 {
		t.Fatalf("create failed idx=%d created=%v", idx2, created2)
	}
}

func TestDataStartRow(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "biz.xlsx")
	tbl := &Table{
		Headers: []string{"id", "name"},
		Rows:    [][]string{{"1", "A"}},
	}
	if err := tbl.Save(src, Options{}); err != nil {
		t.Fatal(err)
	}
	// reload with data-start 1 is fine
	back, err := Load(src, Options{DataStartRow: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Rows) != 1 {
		t.Fatalf("rows=%v", back.Rows)
	}
}

func TestDetectFormat(t *testing.T) {
	cases := map[string]Format{
		"a.xlsx": FormatXLSX, "a.CSV": FormatCSV, "a.tsv": FormatTSV,
		"a.json": FormatJSON, "a.yaml": FormatYAML, "a.yml": FormatYAML,
	}
	for path, want := range cases {
		got, err := DetectFormat(path)
		if err != nil || got != want {
			t.Fatalf("%s: got %v err=%v", path, got, err)
		}
	}
	if _, err := DetectFormat("a.txt"); err == nil {
		t.Fatal("txt should fail")
	}
}
