package validate

import "testing"

func TestParseAndValidateOK(t *testing.T) {
	csv := "id,name,exp,hp\n1,Lv1,100,500\n2,Lv2,220,1100\n3,Lv3,500,2500\n"
	rows, err := ParseCSVTable(csv)
	if err != nil {
		t.Fatal(err)
	}
	issues := Table(rows, Options{GrowthCliff: 0.5, Monotonic: []string{"exp", "hp"}, MaxSafe: 1e12})
	for _, is := range issues {
		if is.Severity == "error" {
			t.Fatalf("unexpected error: %+v", is)
		}
	}
}

func TestCliffDetected(t *testing.T) {
	csv := "id,name,exp\n1,a,100\n2,b,400\n"
	rows, err := ParseCSVTable(csv)
	if err != nil {
		t.Fatal(err)
	}
	issues := Table(rows, Options{GrowthCliff: 0.5})
	found := false
	for _, is := range issues {
		if is.Field == "exp" && is.Severity == "warn" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected cliff warn, got %+v", issues)
	}
}

func TestDuplicateID(t *testing.T) {
	csv := "id,name,exp\n1,a,100\n1,b,200\n"
	rows, _ := ParseCSVTable(csv)
	issues := Table(rows, DefaultOptions())
	found := false
	for _, is := range issues {
		if is.Message == "重复 ID" {
			found = true
		}
	}
	if !found {
		t.Fatal("duplicate id not flagged")
	}
}

func TestOverflow(t *testing.T) {
	csv := "id,name,exp\n1,a,3e10\n"
	rows, _ := ParseCSVTable(csv)
	issues := Table(rows, DefaultOptions())
	found := false
	for _, is := range issues {
		if is.Severity == "error" && is.Field == "exp" {
			found = true
		}
	}
	if !found {
		t.Fatalf("overflow not flagged: %+v", issues)
	}
}
