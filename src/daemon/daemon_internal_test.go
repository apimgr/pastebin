//go:build !windows

package daemon

import (
	"testing"
)

// ─── filterDaemonFlag ────────────────────────────────────────────────────────

func TestFilterDaemonFlag(t *testing.T) {
	cases := []struct {
		input []string
		want  []string
	}{
		{
			input: []string{"--port", "8080", "--daemon", "--config", "/etc"},
			want:  []string{"--port", "8080", "--config", "/etc"},
		},
		{
			input: []string{"-d", "--mode", "production"},
			want:  []string{"--mode", "production"},
		},
		{
			input: []string{"--daemon", "-d"},
			want:  []string{},
		},
		{
			input: []string{"--port", "8080"},
			want:  []string{"--port", "8080"},
		},
		{
			input: []string{},
			want:  []string{},
		},
	}
	for _, tc := range cases {
		got := filterDaemonFlag(tc.input)
		if len(got) != len(tc.want) {
			t.Errorf("filterDaemonFlag(%v) = %v; want %v", tc.input, got, tc.want)
			continue
		}
		for i, v := range got {
			if v != tc.want[i] {
				t.Errorf("filterDaemonFlag(%v)[%d] = %q; want %q", tc.input, i, v, tc.want[i])
			}
		}
	}
}
