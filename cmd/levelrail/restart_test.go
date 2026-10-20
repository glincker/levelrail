package main

import "testing"

func TestRestartSupervised(t *testing.T) {
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	no := func(string) bool { return false }
	yes := func(string) bool { return true }
	cases := []struct {
		name   string
		env    map[string]string
		exists func(string) bool
		want   bool
	}{
		{"bare process", nil, no, false},
		{"systemd", map[string]string{"INVOCATION_ID": "x"}, no, true},
		{"docker", nil, yes, true},
		{"override off", map[string]string{"APP_RESTART_SUPERVISED": "0", "INVOCATION_ID": "x"}, no, false},
		{"override on", map[string]string{"APP_RESTART_SUPERVISED": "true"}, no, true},
	}
	for _, c := range cases {
		if got := restartSupervised(env(c.env), c.exists); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}
