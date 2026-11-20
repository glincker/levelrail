package agent

import "testing"

func TestValidEnrollNodeName(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"node-1", true}, {"Edge.eu_2", true}, {"", false}, {"a b", false},
		{"x\nAPP_JOIN_TOKEN=y", false}, {"-lead", false}, {"a$(id)", false},
		{"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", false},
	}
	for _, tc := range tests {
		if got := validEnrollNodeName(tc.name); got != tc.want {
			t.Errorf("%q: got %v want %v", tc.name, got, tc.want)
		}
	}
}

func TestValidEnrollNodeNameRejectsAliases(t *testing.T) {
	for _, n := range []string{"server", "Managed", "PERSONAL", "0123456789abcdef01234567"} {
		if validEnrollNodeName(n) {
			t.Errorf("%q must be refused", n)
		}
	}
	if !validEnrollNodeName("server-2") {
		t.Error("server-2 should stay valid")
	}
}
