package api

import "testing"

func TestDefaultImageRepo(t *testing.T) {
	const sha = "fe49a4b97c2100c26188d2845a386c58e9610350"
	tests := []struct {
		name    string
		current string
		want    string
	}{
		{"pending placeholder", "dogfood/web:pending", "dogfood/web"},
		{"commit sha tag", "dogfood/web:" + sha, "dogfood/web"},
		{"commit sha tag pinned by digest", "dogfood/web:" + sha + "@sha256:0b6d", "dogfood/web"},
		{"registry port kept", "host:5000/org/web:" + sha, "host:5000/org/web"},
		{"registry image falls back to app name", "nginx:1.27", "web"},
		{"untagged falls back to app name", "nginx", "web"},
		{"empty falls back to app name", "", "web"},
		{"short sha is not a build tag", "org/web:abc123", "web"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := defaultImageRepo("web", tt.current); got != tt.want {
				t.Errorf("defaultImageRepo(%q) = %q, want %q", tt.current, got, tt.want)
			}
		})
	}
}
