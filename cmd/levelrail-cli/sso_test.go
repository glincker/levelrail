package main

import "testing"

func TestParseSSOMapping(t *testing.T) {
	for _, tc := range []struct {
		in      string
		group   string
		role    string
		prio    int
		wantErr bool
	}{
		{"Admins=role_admin:100", "Admins", "role_admin", 100, false},
		{"eng = role_operator", "eng", "role_operator", 0, false},
		{"eng=role_x", "eng", "role_x", 0, false},
		{"eng=role_x:high", "", "", 0, true},
		{"=role_x", "", "", 0, true},
		{"eng=", "", "", 0, true},
		{"no-equals", "", "", 0, true},
	} {
		m, err := parseSSOMapping(tc.in)
		if (err != nil) != tc.wantErr {
			t.Fatalf("%q: err %v", tc.in, err)
		}
		if !tc.wantErr && (m.Group != tc.group || m.RoleID != tc.role || m.Priority != tc.prio) {
			t.Fatalf("%q: got %+v", tc.in, m)
		}
	}
}
