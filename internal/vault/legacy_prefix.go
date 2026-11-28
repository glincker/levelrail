package vault

import "strings"

// LegacyPathAllowed reports whether a legacy vault path sits inside one of the allowed prefixes.
// Paths with dot segments never match, so a prefix cannot be escaped.
func LegacyPathAllowed(path string, prefixes []string) bool {
	for _, seg := range strings.Split(path, "/") {
		if seg == ".." || seg == "." {
			return false
		}
	}
	p := strings.Trim(path, "/")
	for _, pre := range prefixes {
		pre = strings.Trim(pre, "/")
		if pre == "" {
			continue
		}
		if p == pre || strings.HasPrefix(p, pre+"/") {
			return true
		}
	}
	return false
}
