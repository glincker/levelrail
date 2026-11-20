package splittest

import "strings"

// staticImagePrefix marks a promote target that is a retained static deploy,
// not a container image, so the promote path can route it to a rollback.
const staticImagePrefix = "static-deploy:"

// StaticImageRef encodes a static deploy ID as a promote target.
func StaticImageRef(deployID string) string { return staticImagePrefix + deployID }

// ParseStaticImageRef returns the deploy ID of a ref made by StaticImageRef.
func ParseStaticImageRef(ref string) (string, bool) {
	id, ok := strings.CutPrefix(ref, staticImagePrefix)
	return id, ok && id != ""
}
