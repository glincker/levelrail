package samlsso

import "strings"

// GroupRole maps one IDP or directory group to a stored role.
type GroupRole struct {
	Group    string
	RoleID   string
	Priority int
}

// ResolveRole picks the role of the highest priority mapping whose group the
// user is in, comparing group names case-insensitively. Ties go to the
// alphabetically first group so the answer never depends on input order.
func ResolveRole(groups []string, mappings []GroupRole) (string, bool) {
	member := make(map[string]bool, len(groups))
	for _, g := range groups {
		member[strings.ToLower(strings.TrimSpace(g))] = true
	}
	var best *GroupRole
	for i := range mappings {
		m := &mappings[i]
		if m.RoleID == "" || !member[strings.ToLower(strings.TrimSpace(m.Group))] {
			continue
		}
		if best == nil || m.Priority > best.Priority ||
			(m.Priority == best.Priority && strings.ToLower(m.Group) < strings.ToLower(best.Group)) {
			best = m
		}
	}
	if best == nil {
		return "", false
	}
	return best.RoleID, true
}

// EmailDomainAllowed reports whether email is in one of domains; an empty
// list allows every domain.
func EmailDomainAllowed(email string, domains []string) bool {
	if len(domains) == 0 {
		return true
	}
	at := strings.LastIndexByte(email, '@')
	if at < 0 {
		return false
	}
	host := strings.ToLower(email[at+1:])
	for _, d := range domains {
		if strings.EqualFold(strings.TrimPrefix(strings.TrimSpace(d), "@"), host) {
			return true
		}
	}
	return false
}
