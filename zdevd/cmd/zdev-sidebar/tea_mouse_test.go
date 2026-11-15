// Tests for the tea engine's mouse hover handling (ZDEV_SIDEBAR_HOVER):
// resolveHover/handleMouseMsg against tea.MouseMsg. See
// internal/render/hover_test.go for the RenderOpts/RenderWithOpts-level
// tests (the styling itself).
package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

// twoProjectSnapshot gives resolveHover two distinct, unambiguous named
// rows to resolve Y coordinates against.
func twoProjectSnapshot(seq int64) *proto.Snapshot {
	return &proto.Snapshot{
		V:        proto.CurrentProtocolVersion,
		Type:     "snapshot",
		Schema:   proto.SchemaVersion,
		Seq:      seq,
		Sessions: []string{},
		Projects: []proto.Project{
			{Name: "alpha", Status: "alive"},
			{Name: "beta", Status: "alive"},
		},
	}
}

// rowYFor looks up the Y a project's row landed on in the model's last
// paint — the same RowRef slice paintSideEffectsCmd publishes as
// @zdev-rows, used here instead of hardcoding frame geometry so this test
// doesn't drift if the frame gains/loses a line elsewhere.
func rowYFor(t *testing.T, m *teaModel, name string) int {
	t.Helper()
	for _, r := range m.cachedRows {
		if r.Name == name {
			return r.Y
		}
	}
	t.Fatalf("no RowRef for project %q in cachedRows: %+v", name, m.cachedRows)
	return -1
}

func TestTeaUpdate_MouseMotion_SetsHoverOnMappedRow(t *testing.T) {
	const fixedNow int64 = 1_777_860_000
	m := newHoverTestModel(twoProjectSnapshot(1), 50, fixedNow)
	alphaY := rowYFor(t, m, "alpha")

	newModel, _ := m.Update(tea.MouseMsg{Y: alphaY, Action: tea.MouseActionMotion})
	nm := newModel.(*teaModel)

	if nm.hovered != "alpha" {
		t.Errorf("hovered = %q, want %q (Y=%d)", nm.hovered, "alpha", alphaY)
	}
	if !bytes.Contains(nm.cachedBody, []byte("alpha")) {
		t.Fatalf("sanity: cachedBody lost alpha entirely: %q", nm.cachedBody)
	}
}

func TestTeaUpdate_MouseMotion_UnmappedLineClearsHover(t *testing.T) {
	const fixedNow int64 = 1_777_860_000
	m := newHoverTestModel(twoProjectSnapshot(1), 50, fixedNow)
	alphaY := rowYFor(t, m, "alpha")

	m1, _ := m.Update(tea.MouseMsg{Y: alphaY, Action: tea.MouseActionMotion})
	nm1 := m1.(*teaModel)
	if nm1.hovered != "alpha" {
		t.Fatalf("setup: expected alpha hovered, got %q", nm1.hovered)
	}

	// Y=0 is always the mood divider — RowRef's contract (frame.go) is
	// that dividers own no entry, so this line is deliberately unmapped.
	m2, _ := nm1.Update(tea.MouseMsg{Y: 0, Action: tea.MouseActionMotion})
	nm2 := m2.(*teaModel)
	if nm2.hovered != "" {
		t.Errorf("hovered = %q after motion onto an unmapped line, want \"\"", nm2.hovered)
	}
}

func TestTeaUpdate_MouseMotion_OutOfFrameClearsHover(t *testing.T) {
	const fixedNow int64 = 1_777_860_000
	m := newHoverTestModel(twoProjectSnapshot(1), 50, fixedNow)
	alphaY := rowYFor(t, m, "alpha")

	m1, _ := m.Update(tea.MouseMsg{Y: alphaY, Action: tea.MouseActionMotion})
	nm1 := m1.(*teaModel)
	if nm1.hovered != "alpha" {
		t.Fatalf("setup: expected alpha hovered, got %q", nm1.hovered)
	}

	height := bytes.Count(nm1.cachedBody, []byte("\n"))
	m2, _ := nm1.Update(tea.MouseMsg{Y: height + 50, Action: tea.MouseActionMotion})
	nm2 := m2.(*teaModel)
	if nm2.hovered != "" {
		t.Errorf("hovered = %q after motion far below the frame, want \"\"", nm2.hovered)
	}

	// Negative Y (a coordinate above the pane) must clear too.
	m3, _ := nm2.Update(tea.MouseMsg{Y: alphaY, Action: tea.MouseActionMotion})
	if m3.(*teaModel).hovered != "alpha" {
		t.Fatalf("setup 2: expected alpha hovered again, got %q", m3.(*teaModel).hovered)
	}
	m4, _ := m3.Update(tea.MouseMsg{Y: -1, Action: tea.MouseActionMotion})
	if got := m4.(*teaModel).hovered; got != "" {
		t.Errorf("hovered = %q after motion with negative Y, want \"\"", got)
	}
}

func TestTeaUpdate_MouseMotion_AbsentNameNeverMatches(t *testing.T) {
	// hovering a name that isn't in the snapshot is a no-op: resolveHover
	// only ever returns a name it read off cachedRows, so this is really a
	// property of resolveHover's construction, pinned here directly.
	const fixedNow int64 = 1_777_860_000
	m := newHoverTestModel(twoProjectSnapshot(1), 50, fixedNow)
	if got := m.resolveHover(999999); got != "" {
		t.Errorf("resolveHover(way out of range) = %q, want \"\"", got)
	}
}

// TestTeaUpdate_MouseMotion_SameYNoDoubleRepaint is the perf-sensitive
// assertion: a motion event per pixel-row at 15fps must not force a repaint
// storm when the resolved project hasn't changed. Uses stubPaintSideEffects
// (the same stamp/publish counting seam TestTeaUpdate_TickSkipsWhenSigUnchanged
// already relies on) as the observable proxy for "a repaint happened",
// since Update() defers all I/O to a returned Cmd.
func TestTeaUpdate_MouseMotion_SameYNoDoubleRepaint(t *testing.T) {
	const fixedNow int64 = 1_777_860_000
	m := newHoverTestModel(twoProjectSnapshot(1), 50, fixedNow)
	alphaY := rowYFor(t, m, "alpha")
	stampCalls, publishCalls := stubPaintSideEffects(t)

	m1, cmd1 := m.Update(tea.MouseMsg{Y: alphaY, Action: tea.MouseActionMotion})
	drainCmd(t, cmd1)
	if *stampCalls != 1 || *publishCalls != 1 {
		t.Fatalf("first motion onto alpha: stamp=%d publish=%d, want 1/1", *stampCalls, *publishCalls)
	}

	// Same Y again — hovered is already "alpha", so this must be a
	// complete no-op: no repaint, no additional stamp/publish calls, and
	// (per Update's Cmd contract) no non-nil Cmd doing more I/O.
	nm1 := m1.(*teaModel)
	_, cmd2 := nm1.Update(tea.MouseMsg{Y: alphaY, Action: tea.MouseActionMotion})
	drainCmd(t, cmd2)
	if *stampCalls != 1 || *publishCalls != 1 {
		t.Errorf("second identical-Y motion caused a repaint: stamp=%d publish=%d, want unchanged 1/1", *stampCalls, *publishCalls)
	}
}

// TestTeaUpdate_MouseMotion_IgnoredWhenHoverDisabled is defensive: in
// production tea_run.go never enables real stdin/mouse reporting unless
// ZDEV_SIDEBAR_HOVER=1, so a MouseMsg can't physically arrive when
// hoverEnabled is false. Pinned anyway so Update()'s own gate doesn't
// silently regress if that production wiring ever changes.
func TestTeaUpdate_MouseMotion_IgnoredWhenHoverDisabled(t *testing.T) {
	const fixedNow int64 = 1_777_860_000
	m := newTestModel(twoProjectSnapshot(1), 50, fixedNow) // hoverEnabled=false
	bodyBefore := append([]byte(nil), m.cachedBody...)

	newModel, cmd := m.Update(tea.MouseMsg{Y: 1, Action: tea.MouseActionMotion})
	nm := newModel.(*teaModel)

	if nm.hovered != "" {
		t.Errorf("hovered = %q with hover disabled, want \"\"", nm.hovered)
	}
	if !bytes.Equal(nm.cachedBody, bodyBefore) {
		t.Errorf("cachedBody changed from a MouseMsg while hover is disabled")
	}
	if cmd != nil {
		t.Errorf("expected a nil Cmd when hover is disabled, got non-nil")
	}
}

// A resize must emit a screen clear, not just re-render. Tea's inline
// renderer tracks only its own lines; a width change re-wraps what is
// already on screen, orphaning the previous frame above the managed region
// where it stays forever (seen live: stale rows stranded above the mood
// divider). Every resize path — live, no-live-snapshot, and outage — must
// clear.
func TestResizeClearsTheScreen(t *testing.T) {
	hasClear := func(t *testing.T, cmd tea.Cmd) bool {
		t.Helper()
		if cmd == nil {
			return false
		}
		// clearScreenMsg is unexported, so compare against the value
		// tea.ClearScreen produces — an empty struct, hence comparable.
		want := tea.ClearScreen()
		msg := cmd()
		if msg == want {
			return true
		}
		// tea.Batch collapses into a BatchMsg carrying the child Cmds.
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, c := range batch {
				if c != nil && c() == want {
					return true
				}
			}
		}
		return false
	}

	// Live snapshot present.
	m := newTestModel(quietSnapshot(1), 50, 1_777_860_000)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: m.width + 17})
	if !hasClear(t, cmd) {
		t.Errorf("resize with a live snapshot must clear the screen")
	}

	// During an outage.
	m2 := newTestModel(quietSnapshot(1), 50, 1_777_860_000)
	m2.outage = true
	_, cmd2 := m2.Update(tea.WindowSizeMsg{Width: m2.width + 9})
	if !hasClear(t, cmd2) {
		t.Errorf("resize during an outage must clear the screen too")
	}

	// A no-op resize (same width) must NOT clear — clearing on every
	// spurious WindowSizeMsg would flash the pane.
	m3 := newTestModel(quietSnapshot(1), 50, 1_777_860_000)
	_, cmd3 := m3.Update(tea.WindowSizeMsg{Width: m3.width})
	if hasClear(t, cmd3) {
		t.Errorf("same-width resize must not clear")
	}
}

func TestShortPaneWindowsTheFrameAndTheClickMap(t *testing.T) {
	stubPaintSideEffects(t)
	snap := quietSnapshot(1)
	snap.Projects = nil
	for i := 0; i < 40; i++ {
		snap.Projects = append(snap.Projects, proto.Project{Name: fmt.Sprintf("p%02d", i), Status: "alive"})
	}
	snap.CurrentSession = "p35"
	m := newTestModel(snap, 50, 1000)

	_, cmd := m.Update(tea.WindowSizeMsg{Width: 50, Height: 15})
	drainCmd(t, cmd)

	if n := bytes.Count(m.cachedBody, []byte("\n")); n > 14 {
		t.Fatalf("body is %d lines; a 15-line pane only shows 14 of them", n)
	}
	y := rowYFor(t, m, "p35")
	line := strings.Split(string(m.cachedBody), "\n")[y]
	if !strings.Contains(line, "p35") {
		t.Fatalf("click map sends line %d (%q) to p35", y, line)
	}
}

func tallTeaModel(t *testing.T) *teaModel {
	t.Helper()
	stubPaintSideEffects(t)
	snap := quietSnapshot(1)
	snap.Projects = nil
	for i := 0; i < 40; i++ {
		snap.Projects = append(snap.Projects, proto.Project{Name: fmt.Sprintf("p%02d", i), Status: "alive"})
	}
	snap.CurrentSession = "p02"
	m := newHoverTestModel(snap, 50, 1000)
	_, cmd := m.Update(tea.WindowSizeMsg{Width: 50, Height: 15})
	drainCmd(t, cmd)
	return m
}

func wheel(t *testing.T, m *teaModel, b tea.MouseButton, times int) {
	t.Helper()
	for i := 0; i < times; i++ {
		_, cmd := m.Update(tea.MouseMsg{X: 5, Y: 5, Button: b, Action: tea.MouseActionPress})
		drainCmd(t, cmd)
	}
}

func TestWheelScrollsTheWindowAndItStaysPut(t *testing.T) {
	m := tallTeaModel(t)
	if !strings.Contains(string(m.cachedBody), "p02") {
		t.Fatalf("focus row not shown initially")
	}
	wheel(t, m, tea.MouseButtonWheelDown, 6)
	if strings.Contains(string(m.cachedBody), "p02") || !strings.Contains(string(m.cachedBody), "p25") {
		t.Fatalf("wheel did not scroll down:\n%s", m.cachedBody)
	}
	y := rowYFor(t, m, "p25")
	if line := strings.Split(string(m.cachedBody), "\n")[y]; !strings.Contains(line, "p25") {
		t.Fatalf("click map stale after wheel: line %d is %q", y, line)
	}

	m.repaintLive()
	if strings.Contains(string(m.cachedBody), "p02") {
		t.Fatalf("a repaint snapped the window back to focus")
	}

	wheel(t, m, tea.MouseButtonWheelUp, 20)
	if !strings.Contains(string(m.cachedBody), "p00") {
		t.Fatalf("wheel up did not return to the top:\n%s", m.cachedBody)
	}
}

func TestFocusChangeAfterWheelBringsFocusBack(t *testing.T) {
	m := tallTeaModel(t)
	wheel(t, m, tea.MouseButtonWheelDown, 6)
	next := *m.snap
	next.CurrentSession = "p03"
	_, cmd := m.Update(teaSnapshotMsg{snap: &next})
	drainCmd(t, cmd)
	if !strings.Contains(string(m.cachedBody), "p03") {
		t.Fatalf("switching session did not bring its row back into view:\n%s", m.cachedBody)
	}
}
