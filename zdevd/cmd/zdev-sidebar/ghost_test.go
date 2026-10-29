package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestSpawnServerPID(t *testing.T) {
	cases := []struct{ env, want string }{
		{"/private/tmp/tmux-501/default,3839,0", "3839"},
		{"", ""},
		{"/tmp/sock", ""},
	}
	for _, c := range cases {
		if got := spawnServerPID(c.env); got != c.want {
			t.Errorf("spawnServerPID(%q) = %q; want %q", c.env, got, c.want)
		}
	}
}

func TestIsGhost(t *testing.T) {
	cases := []struct {
		spawn, answering string
		want             bool
	}{
		{"3839", "44964", true},
		{"3839", "3839", false},
		{"3839", "", false}, // query failed: not evidence
		{"", "44964", false},
	}
	for _, c := range cases {
		if got := isGhost(c.spawn, c.answering); got != c.want {
			t.Errorf("isGhost(%q, %q) = %v; want %v", c.spawn, c.answering, got, c.want)
		}
	}
}

func TestWatchForGhostExitsOnReplacedServer(t *testing.T) {
	var answering atomic.Value
	answering.Store("3839")
	orig := answeringServerPIDFn
	answeringServerPIDFn = func(context.Context) string { return answering.Load().(string) }
	t.Cleanup(func() { answeringServerPIDFn = orig })

	exited := make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go watchForGhost(ctx, "3839", time.Millisecond, func() { close(exited) })

	answering.Store("44964")
	select {
	case <-exited:
	case <-time.After(5 * time.Second):
		t.Fatal("watchForGhost did not exit after the server was replaced")
	}
}

func TestWatchForGhostStaysOnOwnServer(t *testing.T) {
	var calls atomic.Int32
	orig := answeringServerPIDFn
	answeringServerPIDFn = func(context.Context) string { calls.Add(1); return "3839" }
	t.Cleanup(func() { answeringServerPIDFn = orig })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		watchForGhost(ctx, "3839", time.Millisecond, func() { t.Error("exit called on own server") })
		close(done)
	}()
	deadline := time.Now().Add(5 * time.Second)
	for calls.Load() < 3 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	cancel()
	<-done
	if calls.Load() < 3 {
		t.Fatalf("expected repeated checks, got %d", calls.Load())
	}
}
