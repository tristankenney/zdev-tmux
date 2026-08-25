package hub

import (
	"testing"

	"github.com/tristankenney/zdev/zdevd/internal/proto"
)

// TestDeriveAttention is the canonical state-table test. Each row is one
// transition; reading the table top-to-bottom should let a maintainer
// understand every documented behavior at a glance.
//
// Time axis convention: all timestamps in the table are unix-seconds.
// `now` is fixed at 1000 across rows so the "fresh-stamp" expectation is
// readable (`WaitStartedTS: 1000` means "stamped on this call").
func TestDeriveAttention(t *testing.T) {
	const now int64 = 1000

	type tc struct {
		name string
		in   AttentionInputs
		want AttentionResult
	}

	cases := []tc{
		// ── Idle → various entries ─────────────────────────────────────
		{
			name: "idle: no titles, no history",
			in:   AttentionInputs{},
			want: AttentionResult{Attention: proto.AttIdle, WaitStartedTS: 0},
		},
		{
			name: "idle: zsh prompt only",
			in:   AttentionInputs{Titles: []string{"zsh", "shell"}},
			want: AttentionResult{Attention: proto.AttIdle, WaitStartedTS: 0},
		},
		{
			name: "fresh wait stamps WaitStartedTS to now",
			in: AttentionInputs{
				Titles: []string{"✳ Refactor the renderer"},
			},
			want: AttentionResult{Attention: proto.AttWaiting, WaitStartedTS: now},
		},
		{
			name: "fresh wait via legacy ● claude glyph",
			in: AttentionInputs{
				Titles: []string{"● claude"},
			},
			want: AttentionResult{Attention: proto.AttWaiting, WaitStartedTS: now},
		},
		{
			name: "working: braille spinner",
			in:   AttentionInputs{Titles: []string{"⠂ Generating code"}},
			want: AttentionResult{Attention: proto.AttWorking, WaitStartedTS: 0},
		},
		{
			// The load-bearing case: title parked at a bare "claude" (idle by
			// the classifier) during a blocking hook, but a fresh hook working
			// stamp keeps the session showing Working.
			name: "working: fresh HookWorkTS with a parked (idle) title",
			in: AttentionInputs{
				Titles:     []string{"claude", "shell"},
				HookWorkTS: now - 10, // well within hookWorkFreshSec
			},
			want: AttentionResult{Attention: proto.AttWorking, WaitStartedTS: 0},
		},
		{
			name: "working: stale HookWorkTS falls back to title (idle)",
			in: AttentionInputs{
				Titles:     []string{"claude"},
				HookWorkTS: now - hookWorkFreshSec - 1, // just decayed
			},
			want: AttentionResult{Attention: proto.AttIdle, WaitStartedTS: 0},
		},
		{
			// A wait always wins over a working heartbeat (in production the
			// wait NotifSeen also zeroes HookWorkTS at the source).
			name: "waiting title beats a fresh HookWorkTS",
			in: AttentionInputs{
				Titles:     []string{"✳ Resolve the conflict"},
				HookWorkTS: now - 1,
			},
			want: AttentionResult{Attention: proto.AttWaiting, WaitStartedTS: now},
		},
		{
			name: "finished: ◆ finished",
			in:   AttentionInputs{Titles: []string{"◆ claude"}},
			want: AttentionResult{Attention: proto.AttFinished, WaitStartedTS: 0},
		},
		{
			name: "literal ✳ Claude Code is NOT a wait",
			in:   AttentionInputs{Titles: []string{"✳ Claude Code"}},
			want: AttentionResult{Attention: proto.AttIdle, WaitStartedTS: 0},
		},

		// ── Wait continuity: preserve WaitStartedTS across passes ───────
		{
			name: "ongoing wait keeps original WaitStartedTS",
			in: AttentionInputs{
				Titles:        []string{"✳ Some task"},
				WaitStartedTS: 500,
				PrevAttention: proto.AttWaiting,
			},
			want: AttentionResult{Attention: proto.AttWaiting, WaitStartedTS: 500},
		},

		// ── Stale-waiting demoter ───────────────────────────────────────
		{
			name: "stale wait: visit post-dates title change → demoted to idle",
			in: AttentionInputs{
				Titles:            []string{"✳ Stale leftover"},
				LastTitleChangeTS: 100,
				LastVisitTS:       500,
				WaitStartedTS:     100,
				PrevAttention:     proto.AttWaiting,
			},
			want: AttentionResult{Attention: proto.AttIdle, WaitStartedTS: 0},
		},
		{
			name: "fresh title change after visit re-elevates",
			in: AttentionInputs{
				Titles:            []string{"✳ Brand new task"},
				LastTitleChangeTS: 600,
				LastVisitTS:       500,
				WaitStartedTS:     0,
				PrevAttention:     proto.AttIdle,
			},
			want: AttentionResult{Attention: proto.AttWaiting, WaitStartedTS: now},
		},
		{
			name: "visit exactly at title-change ts → demoted (>= boundary)",
			in: AttentionInputs{
				Titles:            []string{"✳ Stale"},
				LastTitleChangeTS: 300,
				LastVisitTS:       300,
				PrevAttention:     proto.AttWaiting,
			},
			want: AttentionResult{Attention: proto.AttIdle, WaitStartedTS: 0},
		},
		{
			name: "no visit ever → no demotion even with old title-change",
			in: AttentionInputs{
				Titles:            []string{"✳ Real wait"},
				LastTitleChangeTS: 100,
				LastVisitTS:       0,
				WaitStartedTS:     100,
				PrevAttention:     proto.AttWaiting,
			},
			want: AttentionResult{Attention: proto.AttWaiting, WaitStartedTS: 100},
		},

		// ── Latch path ──────────────────────────────────────────────────
		{
			name: "latch: prev=waiting, agent transitioned to working before visit",
			in: AttentionInputs{
				Titles:            []string{"⠂ Resumed on its own"},
				LastTitleChangeTS: 600,
				LastVisitTS:       0,
				WaitStartedTS:     500,
				PrevAttention:     proto.AttWaiting,
				WaitConfirmed:     true, // displayed or hook-receipted
			},
			want: AttentionResult{Attention: proto.AttWaiting, WaitStartedTS: 500},
		},
		{
			name: "latch does NOT arm for an unconfirmed blip (dwell-suppressed ✳ sample)",
			in: AttentionInputs{
				Titles:            []string{"⠂ Resumed on its own"},
				LastTitleChangeTS: 600,
				LastVisitTS:       0,
				WaitStartedTS:     500,
				PrevAttention:     proto.AttWaiting,
				WaitConfirmed:     false, // never displayed, no hook receipt
			},
			want: AttentionResult{Attention: proto.AttWorking, WaitStartedTS: 0},
		},
		{
			name: "latch: prev=waiting, agent transitioned to idle before visit",
			in: AttentionInputs{
				Titles:            []string{"zsh"},
				LastTitleChangeTS: 600,
				LastVisitTS:       0,
				WaitStartedTS:     500,
				PrevAttention:     proto.AttWaiting,
				WaitConfirmed:     true,
			},
			want: AttentionResult{Attention: proto.AttWaiting, WaitStartedTS: 500},
		},
		{
			name: "latch cleared once user visits, agent already non-waiting",
			in: AttentionInputs{
				Titles:            []string{"⠂ Working now"},
				LastTitleChangeTS: 600,
				LastVisitTS:       700, // post-dates WaitStartedTS
				WaitStartedTS:     500,
				PrevAttention:     proto.AttWaiting,
			},
			want: AttentionResult{Attention: proto.AttWorking, WaitStartedTS: 0},
		},
		{
			name: "no latch when prev was not waiting",
			in: AttentionInputs{
				Titles:        []string{"zsh"},
				LastVisitTS:   0,
				WaitStartedTS: 500, // residual but prev not waiting
				PrevAttention: proto.AttWorking,
			},
			want: AttentionResult{Attention: proto.AttIdle, WaitStartedTS: 0},
		},
		{
			name: "no latch when WaitStartedTS is zero (defensive)",
			in: AttentionInputs{
				Titles:        []string{"zsh"},
				PrevAttention: proto.AttWaiting,
				WaitStartedTS: 0,
			},
			want: AttentionResult{Attention: proto.AttIdle, WaitStartedTS: 0},
		},

		// ── Priority: waiting outranks working/finished ────────────────
		{
			name: "multi-pane: waiting wins over working",
			in: AttentionInputs{
				Titles: []string{"⠂ pane A working", "✳ pane B waiting"},
			},
			want: AttentionResult{Attention: proto.AttWaiting, WaitStartedTS: now},
		},
		{
			name: "multi-pane: working wins over finished when no wait",
			in:   AttentionInputs{Titles: []string{"◆ done", "⠂ still going"}},
			want: AttentionResult{Attention: proto.AttWorking, WaitStartedTS: 0},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := DeriveAttention(c.in, now)
			if got != c.want {
				t.Errorf("DeriveAttention(%s):\n  got  = %+v\n  want = %+v", c.name, got, c.want)
			}
		})
	}
}

// TestAttentionToStatus verifies the back-compat shim that keeps the
// legacy four-value Status string aligned with the new Attention enum.
// Once step 3 of the refactor removes Status from the snapshot, this
// function and its test go away.
func TestAttentionToStatus(t *testing.T) {
	cases := []struct {
		att  proto.Attention
		want string
	}{
		{proto.AttIdle, "alive"},
		{proto.AttWorking, "shell-running"},
		{proto.AttFinished, "finished"},
		{proto.AttWaiting, "waiting"},
	}
	for _, c := range cases {
		if got := AttentionToStatus(c.att); got != c.want {
			t.Errorf("AttentionToStatus(%q) = %q; want %q", c.att, got, c.want)
		}
	}
}

// The 2026-08-24 report: three sessions working for 20+ minutes, all showing
// idle. Two independent causes, one per assertion here.
//
// PreToolUse was never installed, so "working" was stamped once per turn and
// decayed after 180s. And even with the heartbeat wired, PreToolUse fires
// BEFORE a tool runs — an observed `/bin/sleep 240` emits nothing for four
// minutes, which no heartbeat interval can bridge. The window has to.
func TestWorkingSurvivesALongSingleToolCall(t *testing.T) {
	now := int64(1_700_000_000)

	// A four-minute tool call with the title parked at a bare "claude" — the
	// exact shape of the screenshot that reported this, and the worst case the
	// window is sized against.
	longCall := AttentionInputs{
		Titles:     []string{"claude", "shell"},
		HookWorkTS: now - 240,
	}
	if got := DeriveAttention(longCall, now); got.Attention != proto.AttWorking {
		t.Errorf("a 240s tool call must still read as working, got %q", got.Attention)
	}

	// A twenty-minute turn, heartbeat rate-limited to once a minute: the
	// newest stamp is at most ZDEV_HEARTBEAT_MIN_SEC old, so this is what an
	// actively-working long turn actually looks like.
	heartbeat := AttentionInputs{
		Titles:     []string{"claude"},
		HookWorkTS: now - 60,
	}
	if got := DeriveAttention(heartbeat, now); got.Attention != proto.AttWorking {
		t.Errorf("a rate-limited heartbeat must read as working, got %q", got.Attention)
	}

	// The window is bounded on BOTH sides. Too short and a long tool call reads
	// as idle; too long and an agent that stopped without firing Stop shows
	// working for as long as the window lasts (observed at 445s on the live
	// fleet with a 900s window — the "everything is staying as active" report).
	if hookWorkFreshSec < 300 {
		t.Errorf("hookWorkFreshSec = %d: too short to bridge one long tool call",
			hookWorkFreshSec)
	}
	if hookWorkFreshSec > 600 {
		t.Errorf("hookWorkFreshSec = %d: long enough that a stopped agent lingers as working",
			hookWorkFreshSec)
	}

	// It must still decay eventually — a hard-crashed agent that fired no
	// Stop and no SessionEnd cannot latch "working" forever.
	crashed := AttentionInputs{
		Titles:     []string{"claude"},
		HookWorkTS: now - hookWorkFreshSec - 1,
	}
	if got := DeriveAttention(crashed, now); got.Attention == proto.AttWorking {
		t.Error("a stamp past the window must stop deriving working")
	}
}
