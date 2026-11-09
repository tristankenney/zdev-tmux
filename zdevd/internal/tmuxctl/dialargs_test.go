package tmuxctl

import (
	"reflect"
	"testing"
)

func TestDialArgs(t *testing.T) {
	cases := []struct {
		name    string
		socket  string
		present bool
		want    []string
	}{
		{"no server yet may start one", "", false,
			[]string{"-CC", "new-session", "-A", "-s", "zdevd-watcher"}},
		{"live socket never replaced", "", true,
			[]string{"-N", "-CC", "new-session", "-A", "-s", "zdevd-watcher"}},
		{"named socket", "it", true,
			[]string{"-L", "it", "-N", "-CC", "new-session", "-A", "-s", "zdevd-watcher"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := dialArgs(c.socket, c.present); !reflect.DeepEqual(got, c.want) {
				t.Errorf("dialArgs(%q, %v) = %v; want %v", c.socket, c.present, got, c.want)
			}
		})
	}
}
