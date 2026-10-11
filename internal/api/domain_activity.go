package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Activity event kinds, matching the dashboard timeline filters.
const (
	activityKindDNS         = "dns"
	activityKindCertificate = "certificate"
	activityKindProxy       = "proxy"
	activityKindAutomation  = "automation"
	activityKindDeploy      = "deploy"
	activityKindSettings    = "settings"

	activityActorUser       = "user"
	activityActorAutomation = "automation"
	activityActorSystem     = "system"

	auditCertIssued  = "cert.issued"
	auditCertRenewed = "cert.renewed"
)

// auditResourcePatterns turns "domain:x", "zone:x" or "app:x" into path
// patterns over the existing audit rows; the data model has no resource column.
func auditResourcePatterns(resource string) ([]string, error) {
	kind, name, ok := strings.Cut(strings.TrimSpace(resource), ":")
	name = strings.ToLower(strings.TrimSpace(name))
	if !ok || name == "" || strings.ContainsAny(name, "/?#%") {
		return nil, errors.New(`resource must look like "domain:<name>", "zone:<name>" or "app:<name>"`)
	}
	lit := store.AuditLikeLiteral(name)
	switch kind {
	case "domain":
		return []string{"%/domains/" + lit, "%/domains/" + lit + "/%", "/api/v1/certificates/" + lit}, nil
	case "zone":
		return []string{"%/zones/" + lit, "%/zones/" + lit + "/%"}, nil
	case "app":
		return []string{"/api/v1/apps/" + lit, "/api/v1/apps/" + lit + "/%"}, nil
	}
	return nil, fmt.Errorf("unknown resource kind %q: use domain, zone or app", kind)
}

// applyAuditResourceFilter reads ?resource= and ?actions= (comma separated
// action prefixes such as "dns_record.,proxy_route.").
func applyAuditResourceFilter(filter *store.AuditEntryFilter, q url.Values) error {
	if res := q.Get("resource"); res != "" {
		patterns, err := auditResourcePatterns(res)
		if err != nil {
			return err
		}
		filter.PathLike = patterns
	}
	for _, p := range strings.Split(q.Get("actions"), ",") {
		if p = strings.TrimSpace(p); p != "" {
			filter.ActionPrefixes = append(filter.ActionPrefixes, p)
		}
	}
	return nil
}

type activityActor struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type activityObject struct {
	Type string `json:"type"`
	ID   string `json:"id"`
	Href string `json:"href,omitempty"`
}

// activityEvent is one normalized timeline entry.
type activityEvent struct {
	ID     string         `json:"id"`
	At     time.Time      `json:"at"`
	Kind   string         `json:"kind"`
	Title  string         `json:"title"`
	Detail string         `json:"detail,omitempty"`
	Failed bool           `json:"failed,omitempty"`
	Action string         `json:"action,omitempty"`
	Actor  activityActor  `json:"actor"`
	Object activityObject `json:"object"`
}

// activityPage is GET /api/v1/domains/{domain}/activity's body.
type activityPage struct {
	Events []activityEvent `json:"events"`
	// NextCursor is passed back as ?before= for the next page.
	NextCursor string `json:"next_cursor,omitempty"`
}

var activitySubjects = map[string]string{
	"redirect": "Redirect", "maintenance": "Maintenance mode", "waf": "WAF", "auth": "Basic auth",
	"tls-cert": "Custom certificate", "error-pages": "Error pages", "search-visibility": "Search engine visibility",
	"dns-records": "DNS record", "renew": "Certificate renewal", "domains": "Domains",
}

func activityKindFor(e store.AuditEntry) string {
	switch {
	case strings.HasPrefix(e.Action, "dns_record."), strings.Contains(e.Path, "/dns-records"), strings.Contains(e.Path, "/zones/"):
		return activityKindDNS
	case strings.HasPrefix(e.Action, "proxy_route."):
		return activityKindProxy
	case strings.HasPrefix(e.Action, "domain."):
		return activityKindAutomation
	case strings.HasPrefix(e.Ability, "cert."), strings.HasPrefix(e.Path, "/api/v1/certificates/"),
		strings.Contains(e.Path, "/cert/"), strings.HasSuffix(e.Path, "/tls-cert"):
		return activityKindCertificate
	case strings.Contains(e.Path, "/deploy"):
		return activityKindDeploy
	}
	return activityKindSettings
}

func humanizeAction(action string) string {
	s := strings.NewReplacer("_", " ", ".", " ").Replace(action)
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func activityTitleFor(e store.AuditEntry) string {
	switch {
	case e.Ability == auditCertIssued:
		return "Certificate issued"
	case e.Ability == auditCertRenewed:
		return "Certificate renewed"
	case e.Action != "":
		return humanizeAction(e.Action)
	}
	seg := e.Path[strings.LastIndexByte(e.Path, '/')+1:]
	subject, ok := activitySubjects[seg]
	if !ok {
		subject = "Domain settings"
	}
	verb := map[string]string{http.MethodPut: "updated", http.MethodPatch: "updated", http.MethodPost: "requested", http.MethodDelete: "removed"}[e.Method]
	if verb == "" {
		verb = "changed"
	}
	return subject + " " + verb
}

func activityActorFor(e store.AuditEntry) activityActor {
	switch {
	case e.ActorType == activityActorSystem:
		return activityActor{Type: activityActorSystem, Name: e.ActorName}
	case e.AgentName != "":
		return activityActor{Type: activityActorAutomation, Name: e.AgentName}
	}
	return activityActor{Type: activityActorUser, Name: e.ActorName}
}

func activityFromAudit(e store.AuditEntry, domain string) activityEvent {
	at, _ := time.Parse(time.RFC3339Nano, e.CreatedAt)
	ev := activityEvent{
		ID: e.ID, At: at, Kind: activityKindFor(e), Title: activityTitleFor(e), Action: e.Action,
		Actor:  activityActorFor(e),
		Object: activityObject{Type: "domain", ID: domain, Href: "/domains/" + url.PathEscape(domain)},
		Failed: e.StatusCode >= 400,
	}
	if ev.Failed {
		ev.Detail = fmt.Sprintf("failed with HTTP %d", e.StatusCode)
	}
	return ev
}

// acmeActivity turns the CA's last recorded error into a timeline event.
func acmeActivity(domain string, before *time.Time) (activityEvent, bool) {
	f := acmeFailureFor(domain)
	if f == nil || (before != nil && !f.At.Before(*before)) {
		return activityEvent{}, false
	}
	title := "Certificate request failed"
	if f.Renewal {
		title = "Certificate renewal failed"
	}
	return activityEvent{
		ID: fmt.Sprintf("acme-failure:%s:%d", domain, f.At.Unix()), At: f.At.UTC(), Kind: activityKindCertificate,
		Title: title, Detail: f.Error, Failed: true, Action: "certificate." + f.Action,
		Actor:  activityActor{Type: activityActorSystem, Name: "Let's Encrypt"},
		Object: activityObject{Type: "domain", ID: domain, Href: "/domains/" + url.PathEscape(domain)},
	}, true
}

// domainVisibleToCaller reports whether the caller may read domain's history.
func (rt *Router) domainVisibleToCaller(r *http.Request, domain string) (bool, error) {
	if rt.callerHasAbility(r, AbilityRoot) {
		return true, nil
	}
	canSee, err := rt.appVisibilityFilter(r)
	if err != nil {
		return false, err
	}
	owners, err := rt.domainOwners(r.Context())
	if err != nil {
		return false, err
	}
	for _, app := range owners[domain] {
		if canSee(app) {
			return true, nil
		}
	}
	return false, nil
}

// handleDomainActivity handles GET /api/v1/domains/{domain}/activity.
func (rt *Router) handleDomainActivity(w http.ResponseWriter, r *http.Request) {
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	if domain == "" {
		writeError(w, http.StatusBadRequest, "domain is required")
		return
	}
	ok, err := rt.domainVisibleToCaller(r, domain)
	if err != nil {
		rt.internalError(w, "api: domain activity: visibility", err)
		return
	}
	if !ok {
		writeError(w, http.StatusNotFound, "domain not found")
		return
	}
	limit, before, filter, valid := parseAuditLogQuery(w, r)
	if !valid {
		return
	}
	if filter.PathLike, err = auditResourcePatterns("domain:" + domain); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := rt.auditLog.ListAuditEntries(r.Context(), limit, before, filter)
	if err != nil {
		rt.internalError(w, "api: domain activity: list audit log", err)
		return
	}
	page := activityPage{Events: make([]activityEvent, 0, len(rows)+1)}
	for _, e := range rows {
		page.Events = append(page.Events, activityFromAudit(e, domain))
	}
	full := len(rows) == limit
	if full {
		page.NextCursor = rows[len(rows)-1].CreatedAt
	}
	// The synthetic ACME event lands on the page whose time range holds it.
	if ev, ok := acmeActivity(domain, before); ok && len(filter.ActionPrefixes) == 0 &&
		(!full || !ev.At.Before(page.Events[len(page.Events)-1].At)) {
		page.Events = append(page.Events, ev)
	}
	sort.SliceStable(page.Events, func(i, j int) bool { return page.Events[i].At.After(page.Events[j].At) })
	writeJSON(w, http.StatusOK, page)
}
