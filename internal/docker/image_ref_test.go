package docker

import "testing"

func TestSameImageRef(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"docker.io/traefik/whoami:v1.10.1", "traefik/whoami:v1.10.1", true},
		{"docker.io/library/nginx:1.27", "nginx:1.27", true},
		{"index.docker.io/library/nginx:1.27", "docker.io/library/nginx:1.27", true},
		{"docker.io/library/nginx@sha256:aa", "nginx@sha256:aa", true},
		{"nginx:1.27", "nginx@sha256:aa", false},
		{"nginx@sha256:aa", "nginx@sha256:bb", false},
		{"ghcr.io/a/b:1", "a/b:1", false},
		{"library/foo/bar:1", "foo/bar:1", false},
		{"", "", true},
	}
	for _, tc := range cases {
		if got := SameImageRef(tc.a, tc.b); got != tc.want {
			t.Errorf("SameImageRef(%q, %q) = %v, want %v", tc.a, tc.b, got, tc.want)
		}
	}
}
