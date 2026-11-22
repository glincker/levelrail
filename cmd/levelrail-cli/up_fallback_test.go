package main

import "testing"

func TestDecideUpSource(t *testing.T) {
	tests := []struct {
		name      string
		f         upFlags
		hasRemote bool
		noUpload  bool
		noWait    bool
		want      upSourceKind
	}{
		{"no remote uploads", upFlags{}, false, false, false, upSourceUpload},
		{"remote builds from git", upFlags{}, true, false, false, upSourceGit},
		{"repo flag builds from git", upFlags{repo: "https://example.com/a.git"}, false, false, false, upSourceGit},
		{"no-upload refuses without remote", upFlags{}, false, true, false, upSourceGit},
		{"no-upload with remote stays git", upFlags{}, true, true, false, upSourceGit},
		{"domain needs git", upFlags{domain: "a.example.com"}, false, false, false, upSourceGit},
		{"database needs git", upFlags{with: []string{"postgres"}}, false, false, false, upSourceGit},
		{"no-wait needs git", upFlags{}, false, false, true, upSourceGit},
		{"name and port still upload", upFlags{name: "shop", port: 8080}, false, false, false, upSourceUpload},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := decideUpSource(tc.f, tc.hasRemote, tc.noUpload, tc.noWait); got != tc.want {
				t.Fatalf("decideUpSource = %v, want %v", got, tc.want)
			}
		})
	}
}
