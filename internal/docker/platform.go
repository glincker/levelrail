package docker

import (
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

// parsePlatform turns "linux/arm64/v8" into the create-time platform; "" means the node default.
func parsePlatform(p string) *ocispec.Platform {
	if p == "" {
		return nil
	}
	parts := strings.SplitN(p, "/", 3)
	out := &ocispec.Platform{OS: parts[0]}
	if len(parts) > 1 {
		out.Architecture = parts[1]
	}
	if len(parts) > 2 {
		out.Variant = parts[2]
	}
	return out
}

// platformMatches compares a local image's platform with the pinned one; a missing variant matches any.
func platformMatches(want, os, arch, variant string) bool {
	w := parsePlatform(want)
	return w.OS == os && w.Architecture == arch && (w.Variant == "" || w.Variant == variant)
}
