package render

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

func tallSnapshot(n int) *proto.Snapshot {
	snap := &proto.Snapshot{}
	for i := 0; i < n; i++ {
		snap.Projects = append(snap.Projects, proto.Project{Name: fmt.Sprintf("proj%02d", i), Status: "alive"})
	}
	return snap
}

func lineCount(b []byte) int { return bytes.Count(b, []byte("\n")) }

func TestFitHeightLeavesShortFramesAlone(t *testing.T) {
	body, rows := Body(tallSnapshot(5), 50, NewAnimator(), fixedNowFn)
	got, gotRows, top := FitHeight(body, rows, 40, -1, 7)
	if !bytes.Equal(got, body) || len(gotRows) != len(rows) || top != 0 {
		t.Fatalf("short frame was altered (top=%d)", top)
	}
}

func TestFitHeightWindowsTallFrames(t *testing.T) {
	snap := tallSnapshot(60)
	body, rows := Body(snap, 50, NewAnimator(), fixedNowFn)
	full := strings.Split(stripAnsi(body), "\n")

	for _, tc := range []struct {
		name  string
		focus string
		want  []string
	}{
		{"no focus shows the top", "", []string{"proj00", "↓"}},
		{"focus near the end", "proj55", []string{"↑", "proj55"}},
		{"focus mid-list", "proj30", []string{"↑", "proj30", "↓"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			snap.CurrentSession = tc.focus
			focus := FocusLine(snap, rows)
			got, gotRows, _ := FitHeight(body, rows, 20, focus, 0)
			if n := lineCount(got); n != 20 {
				t.Fatalf("got %d lines, want 20", n)
			}
			assertRowMapMatchesFrame(t, got, gotRows)
			out := stripAnsi(got)
			lines := strings.Split(out, "\n")
			if lines[0] != full[0] {
				t.Errorf("mood divider not pinned: %q", lines[0])
			}
			if lines[19] != full[len(full)-2] {
				t.Errorf("footer not pinned: %q want %q", lines[19], full[len(full)-2])
			}
			for _, w := range tc.want {
				if !strings.Contains(out, w) {
					t.Errorf("window missing %q:\n%s", w, out)
				}
			}
		})
	}
}

func TestFitHeightScrollsMinimally(t *testing.T) {
	snap := tallSnapshot(60)
	body, rows := Body(snap, 50, NewAnimator(), fixedNowFn)
	snap.CurrentSession = "proj40"
	_, _, top := FitHeight(body, rows, 20, FocusLine(snap, rows), 0)

	snap.CurrentSession = "proj38"
	_, _, again := FitHeight(body, rows, 20, FocusLine(snap, rows), top)
	if again != top {
		t.Errorf("focus already visible, but window moved %d → %d", top, again)
	}
}

func TestFitHeightFollowsCursor(t *testing.T) {
	snap := tallSnapshot(60)
	snap.CursorActive = true
	snap.CursorRow = 45
	snap.CurrentSession = "proj02"
	body, rows := Body(snap, 50, NewAnimator(), fixedNowFn)
	got, gotRows, _ := FitHeight(body, rows, 20, FocusLine(snap, rows), 0)
	for _, r := range gotRows {
		if r.Name == "proj45" {
			assertRowMapMatchesFrame(t, got, gotRows)
			return
		}
	}
	t.Fatalf("cursor row proj45 not in window:\n%s", stripAnsi(got))
}
