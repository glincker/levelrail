package api

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	resourcePrefixEnvironment     = "environment:"
	resourcePrefixEnvironmentKind = "environment-kind:"
)

var environmentKinds = []string{"dev", "test", "uat", "production", "preview", "custom"}

var errBadEnvironmentResource = errors.New("environment resource must look like environment:ID, environment-kind:KIND or end in *")

// environmentResources lists the policy resources an environment adds to
// whatever it contains; an untagged resource has none.
func environmentResources(ref *store.EnvironmentRef) []string {
	if ref == nil {
		return nil
	}
	out := []string{resourcePrefixEnvironment + ref.ID}
	if ref.Kind != "" {
		out = append(out, resourcePrefixEnvironmentKind+ref.Kind)
	}
	return out
}

func statementMatchesAny(s Statement, ability, resource string, extra []string) bool {
	if statementMatches(s, ability, resource) {
		return true
	}
	for _, e := range extra {
		if statementMatches(s, ability, e) {
			return true
		}
	}
	return false
}

// validateEnvironmentResource rejects malformed environment resources: an
// empty value, whitespace, a wildcard that is not trailing, or an unknown kind.
func validateEnvironmentResource(r string) error {
	var value, prefix string
	switch {
	case strings.HasPrefix(r, resourcePrefixEnvironmentKind):
		prefix, value = resourcePrefixEnvironmentKind, strings.TrimPrefix(r, resourcePrefixEnvironmentKind)
	case strings.HasPrefix(r, resourcePrefixEnvironment):
		prefix, value = resourcePrefixEnvironment, strings.TrimPrefix(r, resourcePrefixEnvironment)
	default:
		return nil
	}
	if value == "" || strings.ContainsAny(value, " \t\r\n") {
		return errBadEnvironmentResource
	}
	if i := strings.Index(value, "*"); i >= 0 && i != len(value)-1 {
		return errBadEnvironmentResource
	}
	if prefix == resourcePrefixEnvironmentKind && !strings.HasSuffix(value, "*") && !slices.Contains(environmentKinds, value) {
		return fmt.Errorf("unknown environment kind %q", value)
	}
	return nil
}
