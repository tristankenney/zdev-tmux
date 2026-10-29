package main

import (
	"context"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

const serverCheckInterval = 30 * time.Second

// spawnServerPID is the server pid in $TMUX ("<socket>,<pid>,<session>"),
// fixed at the moment tmux spawned this renderer.
func spawnServerPID(tmuxEnv string) string {
	parts := strings.Split(tmuxEnv, ",")
	if len(parts) < 3 {
		return ""
	}
	return parts[1]
}

// Only a definite mismatch is evidence: an empty answer means tmux was slow
// or between servers, not that we were orphaned.
func isGhost(spawnPID, answeringPID string) bool {
	return spawnPID != "" && answeringPID != "" && spawnPID != answeringPID
}

var answeringServerPIDFn = answeringServerPID

func answeringServerPID(ctx context.Context) string {
	sctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(sctx, "tmux", "list-sessions", "-F", "#{pid}").Output()
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(string(out), "\n")
	return strings.TrimSpace(first)
}

// watchForGhost calls exit once the server answering on the socket path is
// not the one that spawned this renderer. A server that lost its socket file
// keeps running, and its renderers' pane-option writes land on the
// replacement server, where their %N names an unrelated pane.
func watchForGhost(ctx context.Context, spawnPID string, every time.Duration, exit func()) {
	if spawnPID == "" {
		return
	}
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if pid := answeringServerPIDFn(ctx); isGhost(spawnPID, pid) {
				slog.Warn("zdev-sidebar: tmux server replaced on socket path; exiting",
					"spawn_server_pid", spawnPID, "answering_server_pid", pid)
				exit()
				return
			}
		}
	}
}
