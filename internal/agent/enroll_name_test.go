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
