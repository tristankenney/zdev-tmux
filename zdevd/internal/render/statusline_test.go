package render

import (
	"testing"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

const slNow = int64(1_700_000_000)

func TestStatusLine(t *testing.T) {
	proj := func(name string, att proto.Attention) proto.Project {
		return proto.Project{Name: name, Attention: att}
	}
	waiting := func(name, kind string, age int64) proto.Project {
		return proto.Project{
			Name: name, Attention: proto.AttWaiting,
			WaitKind: kind, WaitStartedTS: slNow - age,
		}
	}

	cases := []struct {
		name string
		snap *proto.Snapshot
		want string
	}{
		{"nil snapshot", nil, ""},
		{
			// "Nothing to say beats a bar saying 0" — an idle fleet must
			// render nothing at all, not "0 waiting · 0 working".
			name: "idle fleet renders the empty string",
			snap: &proto.Snapshot{Projects: []proto.Project{proj("a", proto.AttIdle)}},
			want: "",
		},
		{
			name: "cheap wait leads with the bolt",
			snap: &proto.Snapshot{
				Projects: []proto.Project{waiting("api", proto.WaitKindPermission, 240)},
				Triage:   []string{"api"},
			},
			want: "⚡ api 4m needs you",
		},
		{
			name: "a decision costs thought, so it is not a bolt",
			snap: &proto.Snapshot{
				Projects: []proto.Project{waiting("api", proto.WaitKindDecision, 90)},
				Triage:   []string{"api"},
			},
			want: "● api 1m needs you",
		},
		{
			name: "the lead is one thing plus a remainder, never a tally",
			snap: &proto.Snapshot{
				Projects: []proto.Project{
					waiting("api", proto.WaitKindPermission, 300),
					waiting("web", proto.WaitKindDecision, 60),
					waiting("infra", proto.WaitKindDecision, 30),
				},
				Triage: []string{"api", "web", "infra"},
			},
			want: "⚡ api 5m needs you · +2",
		},
		{
			// Waiting is carried by the lead and its +N; counting it again in
			// the quieter states was the footer habit this replaces.
			name: "quiet states are counted, waiting is not double-counted",
			snap: &proto.Snapshot{
				Projects: []proto.Project{
					waiting("api", proto.WaitKindPermission, 60),
					proj("w1", proto.AttWorking), proj("w2", proto.AttWorking),
					proj("d1", proto.AttFinished),
					proj("x1", proto.AttDead),
				},
				Triage: []string{"api"},
			},
			want: "⚡ api 1m needs you · 2 working · 1 done · 1 dead",
		},
		{
			// The row the operator turned off, at zero row cost.
			name: "no demand: the anchor and its held set lead instead",
			snap: &proto.Snapshot{
				Projects: []proto.Project{proj("api", proto.AttWorking)},
				Anchor:   &proto.Anchor{Title: "IMP-97", SinceTS: slNow - 1920},
				Held:     []proto.HeldItem{{}, {}, {}},
			},
			want: "▶ IMP-97 · 32m · ┊ holding 3 · 1 working",
		},
		{
			name: "demand outranks the anchor",
			snap: &proto.Snapshot{
				Projects: []proto.Project{waiting("api", proto.WaitKindPermission, 60)},
				Triage:   []string{"api"},
				Anchor:   &proto.Anchor{Title: "IMP-97", SinceTS: slNow - 60},
			},
			want: "⚡ api 1m needs you",
		},
		{
			// The daemon owns rank order; a stale name must not become a lead.
			name: "a triage name with no project is skipped",
			snap: &proto.Snapshot{
				Projects: []proto.Project{proj("api", proto.AttWorking)},
				Triage:   []string{"vanished"},
			},
			want: "1 working",
		},
		{
			name: "an anchor with no title is not an anchor",
			snap: &proto.Snapshot{
				Projects: []proto.Project{proj("api", proto.AttWorking)},
				Anchor:   &proto.Anchor{SinceTS: slNow - 60},
			},
			want: "1 working",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := StatusLine(tc.snap, slNow); got != tc.want {
				t.Errorf("StatusLine =\n  %q\nwant\n  %q", got, tc.want)
			}
		})
	}
}

// The caller's own status-right owns styling, so the line must carry no
// escapes of its own — otherwise it would fight every theme it lands in.
func TestStatusLineCarriesNoColor(t *testing.T) {
	snap := &proto.Snapshot{
		Projects: []proto.Project{{
			Name: "api", Attention: proto.AttWaiting,
			WaitKind: proto.WaitKindPermission, WaitStartedTS: slNow - 60,
		}},
		Triage: []string{"api"},
	}
	got := StatusLine(snap, slNow)
	if got == "" {
		t.Fatal("expected a line")
	}
	for _, r := range got {
		if r == 0x1b {
			t.Errorf("StatusLine emitted an escape sequence: %q", got)
			break
		}
	}
}
