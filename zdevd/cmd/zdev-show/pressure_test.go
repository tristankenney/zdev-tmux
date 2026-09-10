package main

import (
	"encoding/json"
	"testing"

	"github.com/tristankenney/zdev/zdevd/internal/pressure"
)

func TestPressureJSONShapeIsStable(t *testing.T) {
	b, err := json.Marshal([]pressure.Item{{ID: "rot:pay/api", Kind: pressure.KindRot, Title: "ready to land", Project: "pay/api", Score: 50, AgeSec: 86400}})
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(b, &got); err != nil || len(got) != 1 {
		t.Fatalf("pressure JSON = %s, err=%v", b, err)
	}
}

func TestEmptyPressureJSONUsesArray(t *testing.T) {
	items := []pressure.Item{}
	b, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Fatalf("empty pressure JSON = %s, want []", b)
	}
}
