package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

func TestHyperlink(t *testing.T) {
	defer func(prev bool) { HyperlinksEnabled = prev }(HyperlinksEnabled)

	t.Run("disabled is byte-identical to today", func(t *testing.T) {
		HyperlinksEnabled = false
		if got := Hyperlink("✗ CI", "https://github.com/o/r/pull/1"); got != "✗ CI" {
			t.Errorf("got %q, want the bare text", got)
		}
	})

	HyperlinksEnabled = true

	t.Run("wraps and stays zero-width", func(t *testing.T) {
		got := Hyperlink("✗ CI", "https://github.com/o/r/pull/1")
		if !strings.Contains(got, "https://github.com/o/r/pull/1") {
			t.Fatalf("uri missing from %q", got)
		}
		// The whole point: a chip's column budget must not change.
		if w, want := CellWidth(got), CellWidth("✗ CI"); w != want {
			t.Errorf("CellWidth = %d, want %d (a link must cost no cells)", w, want)
		}
	})

	t.Run("empty inputs pass through", func(t *testing.T) {
		if got := Hyperlink("✓ CI", ""); got != "✓ CI" {
			t.Errorf("empty uri should not wrap, got %q", got)
		}
		if got := Hyperlink("", "https://example.com"); got != "" {
			t.Errorf("empty text should stay empty, got %q", got)
		}
	})

	// A URI reaches this from a probe or an agent-authored payload. An
	// unescaped ESC or ST inside it would break out of the OSC and inject
	// arbitrary control into the operator's screen.
	t.Run("hostile uris are refused, not escaped", func(t *testing.T) {
		for _, bad := range []string{
			"https://e.com/\x1b\\\x1b]0;pwned\x07",
			"https://e.com/\x1b[2J",
			"https://e.com/\x07",
			"https://e.com/a\nb",
			"javascript:alert(1)",
			"data:text/html,<script>",
			"file:/etc/passwd", // missing //, not our allow-listed form
			"HTTPS://e.com",    // scheme match is exact, not case-folded
			"https://e.com/" + strings.Repeat("a", 3000),
		} {
			got := Hyperlink("chip", bad)
			if got != "chip" {
				t.Errorf("Hyperlink(_, %q) wrapped when it should refuse: %q", bad, got)
			}
		}
	})

	t.Run("allowed schemes wrap", func(t *testing.T) {
		for _, ok := range []string{
			"https://github.com/o/r/pull/1",
			"http://localhost:8080/x",
			"file:///Users/x/workspace/bowens/INITIATIVE.md",
		} {
			if got := Hyperlink("chip", ok); got == "chip" {
				t.Errorf("Hyperlink(_, %q) refused a legitimate uri", ok)
			}
		}
	})

	// Truncation runs over already-styled chip text; it must never leave a
	// half-written escape on the operator's screen.
	t.Run("truncation cannot split the escape", func(t *testing.T) {
		link := Hyperlink("PR #1234", "https://github.com/o/r/pulls")
		for cells := 1; cells <= CellWidth("PR #1234"); cells++ {
			got := CellTruncate(link, cells, "…")
			if CellWidth(got) > cells {
				t.Fatalf("cells=%d: width %d exceeds budget: %q", cells, CellWidth(got), got)
			}
			if strings.Count(got, oscStart) != strings.Count(got, oscEnd) &&
				!strings.HasSuffix(got, oscEnd) {
				t.Logf("cells=%d: unbalanced but terminator-suffixed: %q", cells, got)
			}
		}
	})
}

func TestRepoURLs(t *testing.T) {
	if got := RepoPullsURL("payau/ai-configs"); got != "https://github.com/payau/ai-configs/pulls" {
		t.Errorf("RepoPullsURL = %q", got)
	}
	if got := RepoActionsURL("payau/ai-configs"); got != "https://github.com/payau/ai-configs/actions" {
		t.Errorf("RepoActionsURL = %q", got)
	}
	// The slug is probe-derived, so anything that could smuggle a path,
	// query, scheme or control byte into the URL yields no link at all.
	for _, bad := range []string{
		"", "payau", "payau/", "/ai-configs", "payau/a/b",
		"payau/ai configs", "payau/ai?x=1", "payau/../../etc",
		"pay au/x", "payau/x#frag", "payau/x\x1b", "pay\nau/x",
	} {
		if got := RepoPullsURL(bad); got != "" {
			t.Errorf("RepoPullsURL(%q) = %q, want empty", bad, got)
		}
	}
}

func TestLinkBuffer(t *testing.T) {
	defer func(prev bool) { HyperlinksEnabled = prev }(HyperlinksEnabled)

	chip := func() *bytes.Buffer { return bytes.NewBufferString("\x1b[36m⚙ CI\x1b[0m") }

	HyperlinksEnabled = false
	b := chip()
	before := b.String()
	LinkBuffer(b, "https://github.com/o/r/actions")
	if b.String() != before {
		t.Error("disabled must leave the chip byte-identical")
	}

	HyperlinksEnabled = true

	t.Run("wraps a styled chip without changing its width", func(t *testing.T) {
		b := chip()
		w := CellWidth(b.String())
		LinkBuffer(b, "https://github.com/o/r/actions")
		if !strings.Contains(b.String(), "actions") {
			t.Fatalf("uri missing: %q", b.String())
		}
		if got := CellWidth(b.String()); got != w {
			t.Errorf("width changed from %d to %d — chip alignment would break", w, got)
		}
	})

	t.Run("empty buffer and refused uri are both no-ops", func(t *testing.T) {
		empty := &bytes.Buffer{}
		LinkBuffer(empty, "https://github.com/o/r/actions")
		if empty.Len() != 0 {
			t.Errorf("empty buffer gained content: %q", empty.String())
		}
		b := chip()
		before := b.String()
		LinkBuffer(b, "javascript:alert(1)")
		if b.String() != before {
			t.Error("a refused uri must leave the chip alone")
		}
		// A project with no resolved remote is the common case.
		b2 := chip()
		before2 := b2.String()
		LinkBuffer(b2, RepoPullsURL(""))
		if b2.String() != before2 {
			t.Error("no slug must leave the chip alone")
		}
	})
}

// End-to-end through the real Render path: a project carrying PR/CI data and a
// resolved slug must emit clickable chips, and the same frame with links off
// must contain no escape at all. Live verification was not possible — nothing
// in the fleet had a CI or PR chip on screen at the time — so the wiring is
// pinned here instead.
func TestRenderEmitsLinkedChips(t *testing.T) {
	defer func(prev bool) { HyperlinksEnabled = prev }(HyperlinksEnabled)

	snap := &proto.Snapshot{
		CurrentSession: "api",
		Sessions:       []string{"api"},
		Projects: []proto.Project{{
			Name:         "api",
			Status:       "working",
			Attention:    proto.AttWorking,
			Repo:         "payau/ai-configs",
			PROpen:       2,
			PRFail:       1,
			CIStatus:     "completed",
			CIConclusion: "failure",
		}},
		PaneVisible: true,
	}
	now := func() int64 { return slNow }

	HyperlinksEnabled = false
	plain := string(Render(snap, 60, NewAnimator(), now))
	if strings.Contains(plain, "\x1b]8;;") {
		t.Error("links disabled but the frame carries an OSC 8 escape")
	}

	HyperlinksEnabled = true
	linked := string(Render(snap, 60, NewAnimator(), now))
	for _, want := range []string{
		"\x1b]8;;https://github.com/payau/ai-configs/pulls",
		"\x1b]8;;https://github.com/payau/ai-configs/actions",
	} {
		if !strings.Contains(linked, want) {
			t.Errorf("frame is missing %q", want)
		}
	}

	// The visible text must be unchanged — a link costs no cells, so every
	// row's width has to match the unlinked frame line for line.
	pl, ll := strings.Split(plain, "\n"), strings.Split(linked, "\n")
	if len(pl) != len(ll) {
		t.Fatalf("line count changed: %d -> %d", len(pl), len(ll))
	}
	for i := range pl {
		if a, b := CellWidth(pl[i]), CellWidth(ll[i]); a != b {
			t.Errorf("line %d width changed %d -> %d:\n  %q\n  %q", i, a, b, pl[i], ll[i])
		}
	}
}
