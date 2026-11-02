package main

import "testing"

func TestEnsureConfigHome(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"neither set defaults under data dir", map[string]string{}, "/d/.config"},
		{"HOME set is left alone", map[string]string{"HOME": "/h"}, ""},
		{"XDG set is left alone", map[string]string{"XDG_CONFIG_HOME": "/x"}, "/x"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := tc.env
			ensureConfigHome(func(k string) string { return env[k] }, func(k, v string) error { env[k] = v; return nil }, "/d")
			if got := env["XDG_CONFIG_HOME"]; got != tc.want {
				t.Fatalf("XDG_CONFIG_HOME = %q, want %q", got, tc.want)
			}
		})
	}
}
