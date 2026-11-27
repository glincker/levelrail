package sources

import (
	"fmt"
	"regexp"
)

var (
	sourceNamePattern = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)
	refPattern        = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,100}$`)
)

// ValidName reports whether name can namespace a source's templates.
func ValidName(name string) bool { return sourceNamePattern.MatchString(name) }

// ValidRef reports whether ref is a plain branch or tag name.
func ValidRef(ref string) bool {
	if ref == "" {
		return true
	}
	return refPattern.MatchString(ref) && !containsDotDot(ref) && ref[0] != '-'
}

func containsDotDot(s string) bool {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '.' && s[i+1] == '.' {
			return true
		}
	}
	return false
}

// NamespacedID returns source/id. A bare id that matches a built-in is refused
// outright, so a source cannot look like (or later shadow) a shipped template.
func NamespacedID(source, id string, isBuiltin func(string) bool) (string, error) {
	if !ValidName(source) {
		return "", fmt.Errorf("source name %q must be lowercase letters, digits and dashes", source)
	}
	if isBuiltin != nil && (isBuiltin(id) || isBuiltin(source+"/"+id)) {
		return "", fmt.Errorf("template id %q collides with a built-in template", id)
	}
	return source + "/" + id, nil
}
