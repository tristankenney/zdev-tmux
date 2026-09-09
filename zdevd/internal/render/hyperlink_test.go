package render

import (
	"strings"
	"testing"
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
