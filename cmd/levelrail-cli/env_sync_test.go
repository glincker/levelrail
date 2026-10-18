package main

import (
	"reflect"
	"testing"
)

func TestSplitEnvSyncFlags(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantFile   string
		wantMasked bool
		wantRest   []string
	}{
		{"defaults", []string{"web"}, ".env", false, []string{"web"}},
		{"file and masked", []string{"web", "--file", "local.env", "--masked"}, "local.env", true, []string{"web"}},
		{"file equals", []string{"--file=a.env", "web", "--api-url", "http://x"}, "a.env", false, []string{"web", "--api-url", "http://x"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, masked, rest := splitEnvSyncFlags(tt.args)
			if file != tt.wantFile || masked != tt.wantMasked || !reflect.DeepEqual(rest, tt.wantRest) {
				t.Fatalf("got (%q, %v, %v), want (%q, %v, %v)", file, masked, rest, tt.wantFile, tt.wantMasked, tt.wantRest)
			}
		})
	}
}
