package pressure

import (
	"path/filepath"
	"testing"
)

func TestBumpPersistsAndRaisesRank(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bumps.json")
	if err := Bump(p, "a"); err != nil {
		t.Fatal(err)
	}
	if err := Bump(p, "a"); err != nil {
		t.Fatal(err)
	}
	b, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	got := ApplyBumps([]Item{{ID: "a", Score: 25}, {ID: "b", Score: 40}}, b)
	if got[0].ID != "a" || got[0].Score != 45 {
		t.Fatalf("bumped = %+v", got)
	}
}
