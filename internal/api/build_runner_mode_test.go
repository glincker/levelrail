package api

import "testing"

func TestValidateRunnerMode(t *testing.T) {
	cases := []struct {
		name string
		req  setAppBuildRunnerRequest
		bad  bool
	}{
		{"auto", setAppBuildRunnerRequest{Mode: "auto"}, false},
		{"auto pin without node", setAppBuildRunnerRequest{Mode: "auto", Pinned: true}, true},
		{"control plane", setAppBuildRunnerRequest{Mode: "control_plane"}, false},
		{"control plane with node", setAppBuildRunnerRequest{Mode: "control_plane", NodeID: "n"}, true},
		{"node", setAppBuildRunnerRequest{Mode: "node", NodeID: "n", Pinned: true}, false},
		{"node without id", setAppBuildRunnerRequest{Mode: "node"}, true},
		{"unknown", setAppBuildRunnerRequest{Mode: "x"}, true},
	}
	for _, c := range cases {
		if got := validateRunnerMode(c.req) != ""; got != c.bad {
			t.Errorf("%s: invalid=%v want %v", c.name, got, c.bad)
		}
	}
}
