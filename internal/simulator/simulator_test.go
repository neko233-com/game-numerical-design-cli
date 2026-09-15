package simulator

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRunParallelLogsAndClear(t *testing.T) {
	root := t.TempDir()
	r := NewRunner(root)
	man, err := r.Run(Request{
		Name: "unit", N: 50, Seed: 7, Workers: 8,
		Attacker: Unit{Name: "A", HP: 4000, Attack: 700, Defense: 200, Speed: 100, CritRate: 0.1, CritDMG: 0.5, SkillMult: 1.4, Accuracy: 1},
		Defender: Unit{Name: "D", HP: 3500, Attack: 500, Defense: 220, Speed: 95, CritRate: 0.05, CritDMG: 0.5, SkillMult: 1, Accuracy: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if man.N != 50 || man.LogCount != 50 {
		t.Fatalf("n=%d logs=%d", man.N, man.LogCount)
	}
	if man.WinRate < 0 || man.WinRate > 1 {
		t.Fatalf("win=%v", man.WinRate)
	}
	// log files exist
	logs, err := r.ListBattles(man.RunID)
	if err != nil || len(logs) != 50 {
		t.Fatalf("logs=%v err=%v", logs, err)
	}
	bl, err := r.GetBattle(man.RunID, 1)
	if err != nil || bl.Winner == "" {
		t.Fatalf("battle1=%+v err=%v", bl, err)
	}

	// second run clears prior
	man2, err := r.Run(Request{
		Name: "unit2", N: 10, Seed: 1,
		Attacker: Unit{HP: 5000, Attack: 800, Defense: 200, Speed: 100, Accuracy: 1, SkillMult: 1.2},
		Defender: Unit{HP: 4000, Attack: 600, Defense: 200, Speed: 90, Accuracy: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, man.RunID)); !os.IsNotExist(err) {
		t.Fatal("prior run should be cleared")
	}
	list, err := r.Search(Query{Limit: 10})
	if err != nil || len(list) == 0 {
		t.Fatalf("index empty: %v", err)
	}
	// keep-prior
	f := false
	if _, err := r.Run(Request{
		Name: "unit3", N: 5, Seed: 2, ClearPrior: &f,
		Attacker: Unit{HP: 3000, Attack: 500, Defense: 100, Speed: 100, Accuracy: 1},
		Defender: Unit{HP: 3000, Attack: 500, Defense: 100, Speed: 100, Accuracy: 1},
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, man2.RunID)); err != nil {
		t.Fatal("keep-prior should retain previous run")
	}
}

func TestWorkersCapped(t *testing.T) {
	root := t.TempDir()
	r := NewRunner(root)
	man, err := r.Run(Request{
		Name: "cap", N: 20, Workers: 5000, Seed: 3,
		Attacker: Unit{HP: 2000, Attack: 400, Defense: 100, Speed: 100, Accuracy: 1},
		Defender: Unit{HP: 2000, Attack: 400, Defense: 100, Speed: 100, Accuracy: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if man.Workers > MaxConcurrent {
		t.Fatalf("workers=%d cap=%d", man.Workers, MaxConcurrent)
	}
}

func TestLogEvery(t *testing.T) {
	root := t.TempDir()
	r := NewRunner(root)
	man, err := r.Run(Request{
		Name: "every", N: 100, LogEvery: 10, Seed: 4,
		Attacker: Unit{HP: 3000, Attack: 600, Defense: 150, Speed: 100, Accuracy: 1},
		Defender: Unit{HP: 3000, Attack: 500, Defense: 150, Speed: 90, Accuracy: 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if man.LogCount < 10 || man.LogCount > 12 {
		t.Fatalf("logCount=%d want ~10", man.LogCount)
	}
}
