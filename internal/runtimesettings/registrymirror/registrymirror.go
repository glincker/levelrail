// Package registrymirror rewrites image references through an operator
// supplied pull-through prefix. It is a leaf so provisioning code can use it
// without importing the settings registry.
package registrymirror

import (
	"fmt"
	"regexp"
	"strings"
)

var prefixPattern = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?(:[0-9]{1,5})?(/[a-zA-Z0-9._-]+)*$`)

// Validate accepts an empty prefix or host[:port][/path] with no scheme.
func Validate(prefix string) error {
	p := strings.TrimSpace(prefix)
	if p == "" {
		return nil
	}
	if strings.Contains(p, "://") {
		return fmt.Errorf("must not include a scheme such as https://")
	}
	if strings.HasSuffix(p, "/") || !prefixPattern.MatchString(p) {
		return fmt.Errorf("must be a hostname with an optional path, for example m.daocloud.io")
	}
	return nil
}

// Apply prepends the prefix to ref. A tag or digest in ref is never touched,
// an invalid or empty prefix leaves ref as is, and an already mirrored ref is
// returned unchanged so repeated calls are safe.
func Apply(ref, prefix string) string {
	p := strings.TrimSpace(prefix)
	if p == "" || ref == "" || Validate(p) != nil {
		return ref
	}
	if strings.HasPrefix(ref, p+"/") {
		return ref
	}
	return p + "/" + ref
}
