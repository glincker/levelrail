package docker

import "strings"

// CanonicalImageRef drops the Docker Hub host and bare library/ prefix so a
// reference compares equal however the runtime reports it. Podman reports
// docker.io/traefik/whoami:v1 for a container created from traefik/whoami:v1.
// Tags and digests are kept, so repo:tag and repo@sha256 stay distinct.
func CanonicalImageRef(ref string) string {
	ref = strings.TrimSpace(ref)
	for _, host := range []string{"docker.io/", "index.docker.io/", "registry-1.docker.io/"} {
		if rest, ok := strings.CutPrefix(ref, host); ok {
			ref = rest
			break
		}
	}
	if rest, ok := strings.CutPrefix(ref, "library/"); ok && !strings.Contains(rest, "/") {
		ref = rest
	}
	return ref
}

// SameImageRef reports whether two references name the same image identity.
func SameImageRef(a, b string) bool {
	return CanonicalImageRef(a) == CanonicalImageRef(b)
}
