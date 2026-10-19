package appwizard

import (
	"context"
	"testing"
)

func TestAnalyzeWarnsWhenBaseDirectoryIsMissing(t *testing.T) {
	files := map[string]string{"apps/web/package.json": `{"name":"web"}`, "apps/web/package-lock.json": "{}"}
	tests := []struct {
		name string
		dir  string
		want bool
	}{
		{"existing directory", "apps/web", false},
		{"trailing slash is tolerated", "apps/web/", false},
		{"missing directory", "apps/api", true},
		{"no directory chosen", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			o := testOpts("repo")
			o.BaseDirectory = tc.dir
			a, err := Analyze(context.Background(), &fakeSource{files: files}, o)
			if err != nil {
				t.Fatal(err)
			}
			got := false
			for _, w := range a.Warnings {
				if w.Code == WarnDirectoryMissing {
					got = true
				}
			}
			if got != tc.want {
				t.Errorf("directory_missing warning = %v, want %v", got, tc.want)
			}
		})
	}
}
