package pressure

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
)

type Bumps map[string]int

func Load(path string) (Bumps, error) {
	b := Bumps{}
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &b); err != nil {
		return nil, err
	}
	return b, nil
}

// Bump persists one deliberate deferral using an atomic same-directory rename.
func Bump(path, id string) error {
	b, err := Load(path)
	if err != nil {
		return err
	}
	b[id]++
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ApplyBumps(items []Item, bumps Bumps) []Item {
	out := append([]Item(nil), items...)
	for i := range out {
		out[i].Score += bumps[out[i].ID] * 10
		if out[i].Score > 100 {
			out[i].Score = 100
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	return out
}
