package main

import (
	"slices"
	"testing"

	"github.com/GLINCKER/levelrail/internal/experimental"
)

func TestCompletionTree_HidesDisabledExperimental(t *testing.T) {
	children := func(path string) []string {
		for _, e := range walkCommandTree() {
			if e.path == path {
				return e.children
			}
		}
		return nil
	}
	cases := []struct {
		name    string
		enable  []experimental.Feature
		path    string
		verb    string
		visible bool
	}{
		{"models off", nil, "", "models", false},
		{"lb off", nil, "", "lb", false},
		{"apply off", nil, "", "apply", false},
		{"diff off", nil, "", "diff", false},
		{"tunnel off", nil, "", "cloudflare-tunnel", false},
		{"ai-assistant off", nil, "settings", "ai-assistant", false},
		{"apps apply stays", nil, "apps", "apply", true},
		{"apps stays", nil, "", "apps", true},
		{"models on", []experimental.Feature{experimental.AIModels}, "", "models", true},
		{"lb on", []experimental.Feature{experimental.LoadBalancer}, "", "lb", true},
		{"iac on", []experimental.Feature{experimental.IaC}, "", "apply", true},
		{"ai-assistant on", []experimental.Feature{experimental.AIChat}, "settings", "ai-assistant", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			experimental.Set(tc.enable...)
			t.Cleanup(experimental.Reset)
			if got := slices.Contains(children(tc.path), tc.verb); got != tc.visible {
				t.Fatalf("verb %q under %q visible=%v, want %v", tc.verb, tc.path, got, tc.visible)
			}
		})
	}
}
