package hub

import (
	"reflect"
	"testing"
	"time"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

// TestOrderingIsRowSort pins the flat-root contract: row order is
// proto.RowSort on both ordering sites — lexicographic (the tree mirrors
// the disk; homes naturally precede their members) except that an
// initiative's floor members sort before its stream members, clustering
// streams after the floor — and buildSnapshot and orderedRowNames agree —
// the Invariant-9 drift class.
func TestOrderingIsRowSort(t *testing.T) {
	s := buildTestState("scratch-session", []string{"%1"}, []string{"shell"})
	s.projectListNames = []string{
		"zdev", "projects/pay-app", "marketplace",
		"marketplace/backend/pay-app", "marketplace/backend",
		"marketplace/area-selector/pay-app",
		"marketplace/pay-app", "dotfiles", "projects/onboarding",
	}

	fromOrdered := orderedRowNames(s)
	want := []string{
		"dotfiles", "marketplace", "marketplace/pay-app",
		"marketplace/area-selector/pay-app",
		"marketplace/backend", "marketplace/backend/pay-app",
		"projects/onboarding", "projects/pay-app",
		"scratch-session", "zdev",
	}
	if !reflect.DeepEqual(fromOrdered, want) {
		t.Errorf("order:\n got %v\nwant %v", fromOrdered, want)
	}

	now := time.Now().Unix()
	snap := buildSnapshot(s, 1, time.Now(), now, now*1000)
	fromSnapshot := make([]string, len(snap.Projects))
	for i := range snap.Projects {
		fromSnapshot[i] = snap.Projects[i].Name
	}
	if !reflect.DeepEqual(fromOrdered, fromSnapshot) {
		t.Errorf("orderedRowNames and buildSnapshot disagree\n ordered:  %v\n snapshot: %v",
			fromOrdered, fromSnapshot)
	}
}

// TestCollapsedNames pins the collapse rule under the flat model:
// attendance holds a whole group open; otherwise per-row — quiet members
// fold, working/waiting/finished/dead stay visible (death via DeadSinceTS).
// Homes (bare group rows) never fold; unmarked groups (no home row) fold
// their quiet members behind the synthetic header.
func TestCollapsedNames(t *testing.T) {
	names := []string{
		"alpha", "alpha/repo-a", "alpha/repo-b", "alpha/stream-s/repo-x",
		"beta", "beta/repo-c",
		"gamma", "gamma/repo-d",
		"delta", "delta/repo-e",
		"epsilon", "epsilon/repo-w", "epsilon/repo-q",
		"projects/pay-app",
		"zdev",
	}
	s := newState()
	s.collapseGroups = true
	s.clientSessions = map[string]string{"c1": "beta-repo-c"}
	s.projectData["gamma-repo-d"] = projectData{Attention: proto.AttWaiting}
	s.projectData["delta-repo-e"] = projectData{DeadSinceTS: 12345}
	s.projectData["epsilon-repo-w"] = projectData{Attention: proto.AttWorking}

	hidden := collapsedNames(s, names)
	// Stream members fold in their OWN unit (<init>/<stream>), not their
	// initiative's — see collapseUnitKey. Here nothing in alpha is attended
	// or active, so its floor and its stream fold alike.
	want := map[string]bool{
		"alpha/repo-a":          true,
		"alpha/repo-b":          true,
		"alpha/stream-s/repo-x": true,
		"epsilon/repo-q":        true,
		"projects/pay-app":      true,
	}
	for n := range want {
		if _, ok := hidden[n]; !ok {
			t.Errorf("%s should be hidden", n)
		}
	}
	for n := range hidden {
		if !want[n] {
			t.Errorf("%s hidden but should not be", n)
		}
	}

	s.collapseGroups = false
	if h := collapsedNames(s, names); h != nil {
		t.Errorf("collapseGroups=false must hide nothing, got %v", h)
	}
}

// TestCollapseSettings pins the [collapse] gates under the flat model:
// initiatives = marked groups (home row present), containers = unmarked.
func TestCollapseSettings(t *testing.T) {
	names := []string{
		"alpha", "alpha/repo-a",
		"projects/pay-app",
	}
	base := func() *state {
		s := newState()
		s.collapseGroups = true
		return s
	}

	s := base()
	s.collapseInitiatives = false
	h := collapsedNames(s, names)
	if _, ok := h["alpha/repo-a"]; ok {
		t.Errorf("collapse.initiatives=false must keep marked-group members visible")
	}
	if _, ok := h["projects/pay-app"]; !ok {
		t.Errorf("unmarked groups still fold")
	}

	s = base()
	s.collapseContainers = false
	h = collapsedNames(s, names)
	if _, ok := h["projects/pay-app"]; ok {
		t.Errorf("collapse.containers=false must keep unmarked-group members visible")
	}
	if _, ok := h["alpha/repo-a"]; !ok {
		t.Errorf("marked groups still fold")
	}

	s = base()
	s.collapseExpand = map[string]struct{}{"alpha": {}}
	h = collapsedNames(s, names)
	if _, ok := h["alpha/repo-a"]; ok {
		t.Errorf("expand-pinned group must never fold")
	}
}

// TestCollapseParity: wire Collapsed flags and cursorFlatRows visible rows
// agree.
func TestCollapseParity(t *testing.T) {
	s := buildTestState("scratch-session", []string{"%1"}, []string{"shell"})
	s.collapseGroups = true
	s.projectListNames = []string{
		"alpha", "alpha/repo-a", "projects/pay-app", "zdev",
	}

	now := time.Now().Unix()
	snap := buildSnapshot(s, 1, time.Now(), now, now*1000)
	visibleWire := map[string]bool{}
	for _, p := range snap.Projects {
		if !p.Collapsed {
			visibleWire[p.Name] = true
		}
	}
	if visibleWire["alpha/repo-a"] {
		t.Fatalf("unattended marked-group member must be Collapsed on the wire")
	}
	if visibleWire["projects/pay-app"] {
		t.Fatalf("unattended unmarked-group member must be Collapsed on the wire")
	}

	rows := cursorFlatRows(s)
	rowNames := map[string]bool{}
	for _, r := range rows {
		rowNames[r.Project.Name] = true
		if !visibleWire[r.Project.Name] {
			t.Errorf("cursor row %q is not wire-visible", r.Project.Name)
		}
	}
	for n := range visibleWire {
		if !rowNames[n] {
			t.Errorf("wire-visible %q missing from cursor rows", n)
		}
	}
}

// A stream is its own collapse unit: entering an initiative expands the FLOOR
// but must not expand its streams. Operator request 2026-08-26 — attendance
// holds a whole unit open, so one key for the entire initiative meant merely
// entering it expanded every stream at once.
func TestStreamsDoNotExpandWithTheirInitiative(t *testing.T) {
	names := []string{
		"alpha", "alpha/repo-a", "alpha/repo-b",
		"alpha/s1", "alpha/s1/repo-x", "alpha/s1/repo-y",
		"alpha/s2", "alpha/s2/repo-z",
	}
	newS := func() *state {
		s := newState()
		s.collapseGroups = true
		return s
	}

	t.Run("attending a floor member expands the floor, not the streams", func(t *testing.T) {
		s := newS()
		s.clientSessions = map[string]string{"c1": proto.SessionKey("alpha/repo-a")}
		h := collapsedNames(s, names)
		for _, n := range []string{"alpha/s1/repo-x", "alpha/s1/repo-y", "alpha/s2/repo-z"} {
			if _, hidden := h[n]; !hidden {
				t.Errorf("%s should stay folded — its stream is not active", n)
			}
		}
		for _, n := range []string{"alpha/repo-a", "alpha/repo-b"} {
			if _, hidden := h[n]; hidden {
				t.Errorf("%s is floor and the initiative is attended — must stay visible", n)
			}
		}
		// Attending the initiative reveals which streams it HAS — headers
		// only, members still folded. "Don't expand" was never "don't exist":
		// folding a quiet stream away completely made one vanish from an
		// initiative the operator was working in (reported 2026-08-27).
		for _, n := range []string{"alpha/s1", "alpha/s2"} {
			if _, hidden := h[n]; hidden {
				t.Errorf("%s must be visible from inside the attended initiative", n)
			}
		}
	})

	t.Run("attending a stream expands only that stream", func(t *testing.T) {
		s := newS()
		s.clientSessions = map[string]string{"c1": proto.SessionKey("alpha/s1/repo-x")}
		h := collapsedNames(s, names)
		for _, n := range []string{"alpha/s1/repo-x", "alpha/s1/repo-y"} {
			if _, hidden := h[n]; hidden {
				t.Errorf("%s is in the attended stream — must be visible", n)
			}
		}
		if _, hidden := h["alpha/s2/repo-z"]; !hidden {
			t.Error("alpha/s2/repo-z is a different stream and must stay folded")
		}
		// The sibling stream's HEADER does come back (see the sibling-reveal
		// case below) — attending anywhere inside an initiative shows what
		// streams it has. Only its members stay folded, which is the
		// distinction this subtest is about: reveal is not expansion.
	})

	t.Run("an active member pierces its stream without attendance", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			pd   projectData
		}{
			{"working", projectData{Attention: proto.AttWorking}},
			{"waiting", projectData{Attention: proto.AttWaiting}},
			{"finished", projectData{Attention: proto.AttFinished}},
			{"dead", projectData{DeadSinceTS: 12345}},
		} {
			t.Run(tc.name, func(t *testing.T) {
				s := newS()
				s.projectData[proto.SessionKey("alpha/s1/repo-x")] = tc.pd
				h := collapsedNames(s, names)
				if _, hidden := h["alpha/s1/repo-x"]; hidden {
					t.Errorf("a %s stream member must pierce the fold", tc.name)
				}
				if _, hidden := h["alpha/s1/repo-y"]; !hidden {
					t.Error("its quiet sibling must still fold")
				}
			})
		}
	})

	// Reported 2026-09-10: working inside one stream hid every sibling
	// stream, because the reveal checked the initiative unit's own hold and
	// attending a stream sets hold on the STREAM unit. From inside bowens's
	// login stream, trigger was invisible and sdk showed only because it
	// happened to be waiting.
	t.Run("attending one stream reveals its sibling stream headers", func(t *testing.T) {
		s := newS()
		s.clientSessions = map[string]string{"c1": proto.SessionKey("alpha/s1/repo-x")}
		h := collapsedNames(s, names)
		if _, hidden := h["alpha/s2"]; hidden {
			t.Error("alpha/s2 heads a sibling stream and must be visible from inside alpha/s1")
		}
		// Its MEMBERS stay folded — headers are not expansion.
		if _, hidden := h["alpha/s2/repo-z"]; !hidden {
			t.Error("the sibling stream's members must stay folded")
		}
		// And the attended stream is still expanded.
		if _, hidden := h["alpha/s1/repo-y"]; hidden {
			t.Error("the attended stream's own members must be visible")
		}
	})

	t.Run("a surviving member brings its stream header back", func(t *testing.T) {
		s := newS()
		s.projectData[proto.SessionKey("alpha/s1/repo-x")] = projectData{Attention: proto.AttWaiting}
		h := collapsedNames(s, names)
		if _, hidden := h["alpha/s1"]; hidden {
			t.Error("alpha/s1 has a visible member — its header must return, " +
				"or the member renders orphaned with nothing naming its stream")
		}
		if _, hidden := h["alpha/s2"]; !hidden {
			t.Error("alpha/s2 is still wholly quiet and must stay folded")
		}
		if _, hidden := h["alpha/s1/repo-y"]; !hidden {
			t.Error("the quiet sibling inside the revived stream still folds")
		}
	})

	t.Run("nothing active anywhere: streams and floor both fold", func(t *testing.T) {
		h := collapsedNames(newS(), names)
		// Nothing attended anywhere, so headers fold away too — a quiet
		// stream in an initiative you are NOT in costs no rows at all.
		for _, n := range []string{"alpha/repo-a", "alpha/s1", "alpha/s1/repo-x", "alpha/s2", "alpha/s2/repo-z"} {
			if _, hidden := h[n]; !hidden {
				t.Errorf("%s should fold when nothing is attended or active", n)
			}
		}
	})
}
