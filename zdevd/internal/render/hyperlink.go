package render

import "strings"

// OSC 8 hyperlinks — the sidebar's chips become clickable where the terminal
// supports it, and identical text where it does not.
//
// This is the tmux-native answer to Herdr's [[link_handlers]] plugin surface
// (zdev-herdr DESIGN.md): tmux has no link-handler concept, but it does carry
// OSC 8 through to the terminal. Verified on tmux 3.7b, 2026-09-09 — the
// escape survives into the pane, and `capture-pane -p` returns the visible
// text alone, so golden captures never see the markup.
//
// Two properties make this safe to sprinkle through chip renderers:
//
//   - ansi.StringWidth counts an OSC 8 wrapper as ZERO cells, and
//     ansi.Truncate will not cut inside one (both verified against the
//     pinned x/ansi, 2026-09-09). Column alignment is therefore unaffected,
//     which matters because every chip is width-budgeted.
//   - a terminal without OSC 8 support renders the payload text and drops
//     the escape, so the fallback is exactly today's output.
//
// HyperlinksEnabled gates it anyway. Not every terminal is well-behaved with
// unknown OSC sequences, and this repo's convention is that a new user-facing
// surface ships behind a knob with current behavior as the default
// (ZDEV_SIDEBAR_LINKS=1).
var HyperlinksEnabled bool

// oscStart/oscEnd bracket a hyperlink. ST is the two-byte ESC \ form rather
// than BEL: BEL is what most examples use, but tmux and several terminals
// treat a bare BEL inside OSC as a terminator ambiguity, and ESC \ is the
// form the spec prefers.
const (
	oscStart = "\x1b]8;;"
	oscEnd   = "\x1b]8;;\x1b\\"
	oscST    = "\x1b\\"
)

// Hyperlink wraps text so it opens uri when clicked. Returns text unchanged
// when links are disabled, when uri is empty, or when either argument carries
// a byte that would break out of the escape — a URI is agent- or
// probe-sourced, and an unescaped ESC or ST inside it would let that source
// inject arbitrary terminal control into the operator's screen. Same trust
// posture as render.SanitizeLine on the wire.
func Hyperlink(text, uri string) string {
	if !HyperlinksEnabled || uri == "" || text == "" {
		return text
	}
	if !safeURI(uri) {
		return text
	}
	return oscStart + uri + oscST + text + oscEnd
}

// safeURI rejects anything that could terminate the escape early or smuggle
// control bytes. Deliberately a strict allow-list of schemes: a chip's link
// is always a repo page, a run URL, or a local file, and "javascript:" or
// "data:" have no business in a terminal even if some emulator would ignore
// them.
func safeURI(uri string) bool {
	if len(uri) > 2048 {
		return false
	}
	for i := 0; i < len(uri); i++ {
		if c := uri[i]; c < 0x20 || c == 0x7f {
			return false
		}
	}
	switch {
	case strings.HasPrefix(uri, "https://"),
		strings.HasPrefix(uri, "http://"),
		strings.HasPrefix(uri, "file://"):
		return true
	}
	return false
}
