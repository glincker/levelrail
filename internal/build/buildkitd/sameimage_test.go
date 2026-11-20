package buildkitd

import "testing"

func TestSameImage(t *testing.T) {
	const digest = "@sha256:0000000000000000000000000000000000000000000000000000000000000000"
	tests := []struct {
		name, got, want string
		ok              bool
	}{
		{"docker style keeps tag and digest", testPinned, testPinned, true},
		{"podman style drops the tag", "docker.io/moby/buildkit" + digest, testPinned, true},
		{"other digest", "docker.io/moby/buildkit@sha256:1111111111111111111111111111111111111111111111111111111111111111", testPinned, false},
		{"unpinned tolerates a missing tag", "registry.local:5000/bk", "registry.local:5000/bk:v1", true},
		{"unpinned other repo", "registry.local:5000/other", "registry.local:5000/bk:v1", false},
	}
	for _, tt := range tests {
		if got := sameImage(tt.got, tt.want); got != tt.ok {
			t.Errorf("%s: sameImage = %t, want %t", tt.name, got, tt.ok)
		}
	}
}
