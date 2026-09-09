package render

import (
	"fmt"
	"strings"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

// The one-liner — the fleet in a status bar rather than a pane.
//
// Stolen from zdev-herdr's cmd/zdev-status, which puts this in Herdr's
// [ui.tab_bar_right] and says in its own comment that it is "where the old
// sidebar's ▶ now row and footer went". The insight worth taking: a
// persistent one-liner wants no pane. In tmux terms it belongs in
// status-right, which is visible in EVERY window at EVERY width — including
// below ZDEV_SIDEBAR_THRESHOLD, where the sidebar is suppressed entirely and
// there is currently no fleet view at all.
//
// It also gives the focus anchor back at zero row cost. The ▶ now row and the
// ┊ holding N counter were switched off (ZDEV_SIDEBAR_FOCUS) because two
// sidebar rows was too much rent for them; in a status bar they cost none.
//
// Three disciplines carried over verbatim, because they are what keep it from
// becoming noise:
//
//   - the lead is the SINGLE most demanding thing plus "+N" for the rest, not
//     a tally of everything;
//   - ⚡ when the wait is cheap to answer, ● when it costs thought — the
//     same answerCost distinction triage ranks on;
//   - nothing to say beats a bar saying 0, so zero counts are omitted and an
//     idle fleet renders the empty string.
//
// Pure: snapshot plus now in, one line out. No color — the caller's own
// status-right owns styling, so zdev never dictates the look of a bar it does
// not own.
func StatusLine(snap *proto.Snapshot, now int64) string {
	if snap == nil {
		return ""
	}

	known := make(map[string]*proto.Project, len(snap.Projects))
	for i := range snap.Projects {
		known[snap.Projects[i].Name] = &snap.Projects[i]
	}

	var parts []string

	// Lead: the top of the daemon-computed queue. Triage is already in rank
	// order on the wire, so this never re-ranks — one authority for "who
	// needs me next", shared with zdev next and the triage popup.
	var queue []string
	for _, name := range snap.Triage {
		if _, ok := known[name]; ok {
			queue = append(queue, name)
		}
	}

	switch {
	case len(queue) > 0:
		top := known[queue[0]]
		glyph := "●"
		if top.WaitKind == proto.WaitKindPermission {
			glyph = "⚡"
		}
		lead := glyph + " " + queue[0]
		if top.WaitStartedTS > 0 && now > top.WaitStartedTS {
			lead += " " + formatAge(now-top.WaitStartedTS)
		}
		parts = append(parts, lead+" needs you")
		if n := len(queue) - 1; n > 0 {
			parts = append(parts, fmt.Sprintf("+%d", n))
		}

	case snap.Anchor != nil && snap.Anchor.Title != "":
		anchor := "▶ " + snap.Anchor.Title
		if snap.Anchor.SinceTS > 0 && now > snap.Anchor.SinceTS {
			anchor += " · " + formatAge(now-snap.Anchor.SinceTS)
		}
		parts = append(parts, anchor)
		if n := len(snap.Held); n > 0 {
			parts = append(parts, fmt.Sprintf("┊ holding %d", n))
		}
	}

	// The quieter states. Waiting is deliberately NOT counted here — the lead
	// and its +N already carry it, and counting it twice was the footer's
	// habit this replaces.
	var working, finished, dead int
	for i := range snap.Projects {
		switch snap.Projects[i].Attention {
		case proto.AttWorking:
			working++
		case proto.AttFinished:
			finished++
		case proto.AttDead:
			dead++
		}
	}
	for _, b := range []struct {
		n    int
		word string
	}{{working, "working"}, {finished, "done"}, {dead, "dead"}} {
		if b.n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", b.n, b.word))
		}
	}

	return strings.Join(parts, " · ")
}
