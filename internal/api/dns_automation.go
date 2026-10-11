package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

// Request values for the add-domain `dns` field.
const (
	dnsModeAuto    = "auto"
	dnsModeOff     = "off"
	dnsModePreview = "preview"
)

// Per-domain DNS outcomes beyond dnsrecords.Outcome*.
const (
	dnsResultSkipped = "skipped"
	dnsResultError   = "error"
	dnsResultPreview = "preview"
)

const (
	dnsAutoTTLCloudflare = time.Second
	dnsAutoTTLDefault    = 5 * time.Minute
	envDNSLookupTimeout  = "APP_DNS_AUTOMATION_TIMEOUT"
	defaultDNSAutoBudget = 20 * time.Second
)

// dnsProxier toggles Cloudflare's proxy flag on a record.
type dnsProxier interface {
	SetProxied(ctx context.Context, zone, fqdn, recType, content string, proxied bool) (int, error)
}

// dnsTarget is the provider, zone and manager a domain's records live in.
type dnsTarget struct {
	Manager  dnsrecords.Manager
	Provider string
	Zone     string
	Proxy    dnsProxier
}

// domainDNSResult is one domain's automatic DNS outcome.
type domainDNSResult struct {
	Domain   string              `json:"domain"`
	DNS      string              `json:"dns"`
	Planned  string              `json:"planned,omitempty"`
	Provider string              `json:"provider,omitempty"`
	Zone     string              `json:"zone,omitempty"`
	Record   *dnsRecordResource  `json:"record,omitempty"`
	Replaced []dnsRecordResource `json:"replaced,omitempty"`
	Proxied  bool                `json:"proxied,omitempty"`
	Message  string              `json:"message,omitempty"`
}

// undoAction is one reversible effect of an automation run.
type undoAction struct {
	Kind       string `json:"kind"`
	App        string `json:"app,omitempty"`
	Domain     string `json:"domain"`
	Name       string `json:"name,omitempty"`
	RecordType string `json:"record_type,omitempty"`
	Value      string `json:"value,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
}

// Undo action kinds.
const (
	undoKindDNSRecord  = "dns_record"
	undoKindDNSRestore = "dns_restore"
	undoKindAppDomain  = "app_domain"
	undoKindRedirect   = "redirect"
)

// dnsOpts controls one applyDomainDNS call.
type dnsOpts struct {
	Mode     string
	Replace  bool
	CanWrite bool
}

func dnsAutomationBudget() time.Duration {
	if v := os.Getenv(envDNSLookupTimeout); v != "" {
		if d, err := time.ParseDuration(v); err == nil && d > 0 {
			return d
		}
	}
	return defaultDNSAutoBudget
}

// resolveDNSTarget finds the provider and zone for domain. A nil target with
// a nil error means no provider is connected; dnsrecords.ErrZoneNotFound
// means one is but it does not manage the domain's zone.
func (rt *Router) resolveDNSTarget(ctx context.Context, domain string) (*dnsTarget, error) {
	if rt.domainAuto.resolveTarget != nil {
		return rt.domainAuto.resolveTarget(ctx, domain)
	}
	token, err := rt.cloudflareDNSToken(ctx)
	if err != nil {
		return nil, err
	}
	if token != "" {
		api := &dnsrecords.CloudflareAPI{Token: token}
		zone, err := api.FindZone(ctx, domain)
		if err != nil {
			return nil, err
		}
		return &dnsTarget{Manager: dnsrecords.NewCloudflareManager(token), Provider: dnsProviderCloudflare, Zone: zone, Proxy: api}, nil
	}
	mgr, err := rt.resolveRoute53RecordManager(ctx)
	if err != nil || mgr == nil {
		return nil, err
	}
	zone, err := dnsrecords.ManagerZoneFinder{Mgr: mgr}.FindZone(ctx, domain)
	if err != nil {
		return nil, err
	}
	return &dnsTarget{Manager: mgr, Provider: dnsProviderRoute53, Zone: zone}, nil
}

// desiredDNSRecord decides the record that points domain at this server.
func (rt *Router) desiredDNSRecord(ctx context.Context, s store.IngressSettings, t *dnsTarget, domain string) (dnsrecords.Desired, error) {
	ttl := time.Duration(s.DNSTTLSeconds) * time.Second
	if ttl == 0 {
		ttl = dnsAutoTTLDefault
		if t.Provider == dnsProviderCloudflare {
			ttl = dnsAutoTTLCloudflare
		}
	}
	d := dnsrecords.Desired{Name: dnsrecords.RelativeName(domain, t.Zone), TTL: ttl}
	if s.DNSCNAMETarget != "" {
		d.Type, d.Value = "CNAME", s.DNSCNAMETarget
		return d, dnsrecords.ValidateTarget(d, t.Provider)
	}
	var ips []string
	if ip := net.ParseIP(rt.publicHost); ip != nil && ingress.IsPubliclyRoutable(ip) {
		ips = []string{rt.publicHost}
	}
	for _, ip := range rt.detectedPublicIPs(ctx) {
		if p := net.ParseIP(ip); p != nil && ingress.IsPubliclyRoutable(p) {
			ips = append(ips, ip)
		}
	}
	typ, val, err := dnsrecords.TargetFor(ips)
	switch {
	case err == nil:
		d.Type, d.Value = typ, val
	case rt.publicHost != "" && net.ParseIP(rt.publicHost) == nil:
		d.Type, d.Value = "CNAME", rt.publicHost
	default:
		return dnsrecords.Desired{}, err
	}
	return d, dnsrecords.ValidateTarget(d, t.Provider)
}

func recordResource(d dnsrecords.Desired) *dnsRecordResource {
	return &dnsRecordResource{Name: d.Name, Type: d.Type, Value: d.Value, TTLSeconds: int(d.TTL.Seconds())}
}

func rrResource(rr libdns.RR) dnsRecordResource {
	return dnsRecordResource{Name: rr.Name, Type: rr.Type, Value: rr.Data, TTLSeconds: int(rr.TTL.Seconds())}
}

// managedDNSStore is the tracking table behind "only remove what we created".
type managedDNSStore interface {
	SaveManagedDNSRecord(ctx context.Context, r store.ManagedDNSRecord) error
	ListManagedDNSRecords(ctx context.Context, domain string) ([]store.ManagedDNSRecord, error)
	DeleteManagedDNSRecord(ctx context.Context, domain, recordType string) error
}

func (rt *Router) managedDNS() managedDNSStore {
	m, _ := rt.apps.(managedDNSStore)
	return m
}

// applyDomainDNS plans and, unless previewing, writes domain's record.
// Foreign records are never overwritten without opts.Replace, and nothing is
// written without opts.CanWrite (the root ability).
func (rt *Router) applyDomainDNS(ctx context.Context, r *http.Request, app, domain string, opts dnsOpts) (domainDNSResult, []undoAction) {
	res := domainDNSResult{Domain: domain, DNS: dnsResultSkipped}
	if opts.Mode == dnsModeOff {
		res.Message = "automatic DNS is off for this request"
		return res, nil
	}
	ctx, cancel := context.WithTimeout(ctx, dnsAutomationBudget())
	defer cancel()

	target, err := rt.resolveDNSTarget(ctx, domain)
	switch {
	case errors.Is(err, dnsrecords.ErrZoneNotFound):
		res.Message = "the connected DNS provider does not manage a zone for this domain"
		return res, nil
	case err != nil:
		rt.logger.Error("api: resolve dns target failed", slog.String("error", err.Error()), slog.String("domain", domain))
		res.DNS, res.Message = dnsResultError, "could not reach the DNS provider"
		return res, nil
	case target == nil:
		res.Message = "no DNS provider is connected"
		return res, nil
	}
	res.Provider, res.Zone = target.Provider, target.Zone

	settings, err := rt.ingressSettings.GetIngressSettings(ctx)
	if err != nil {
		rt.logger.Error("api: dns automation: ingress settings failed", slog.String("error", err.Error()))
		res.DNS, res.Message = dnsResultError, "internal error"
		return res, nil
	}
	desired, err := rt.desiredDNSRecord(ctx, settings, target, domain)
	if err != nil {
		res.Message = err.Error()
		return res, nil
	}
	res.Record = recordResource(desired)
	res.Proxied = settings.DNSProxied && target.Proxy != nil && desired.Type != "TXT"

	existing, err := target.Manager.GetRecords(ctx, target.Zone)
	if err != nil {
		rt.logger.Error("api: dns automation: list records failed", slog.String("error", err.Error()), slog.String("domain", domain))
		res.DNS, res.Message = dnsResultError, "could not read the zone's records"
		return res, nil
	}
	plan := dnsrecords.PlanRecord(desired, existing, opts.Replace)
	for _, rr := range plan.Replace {
		res.Replaced = append(res.Replaced, rrResource(rr))
	}
	res.Planned = plan.Outcome
	if opts.Mode == dnsModePreview {
		res.DNS = dnsResultPreview
		res.Message = previewMessage(plan.Outcome, desired)
		return res, nil
	}
	switch plan.Outcome {
	case dnsrecords.OutcomeUnchanged:
		res.DNS = dnsrecords.OutcomeUnchanged
		res.Message = "the record already exists"
		return res, nil
	case dnsrecords.OutcomeConflict:
		res.DNS = dnsrecords.OutcomeConflict
		res.Message = "a different record already exists for this name; nothing was changed (pass replace to overwrite)"
		return res, nil
	}
	if !opts.CanWrite {
		res.Message = "creating DNS records needs the root ability; add the record manually or ask an admin"
		return res, nil
	}
	return rt.writeDomainDNS(ctx, r, app, domain, target, desired, plan, res)
}

func previewMessage(outcome string, d dnsrecords.Desired) string {
	switch outcome {
	case dnsrecords.OutcomeCreated:
		return fmt.Sprintf("would create %s %s -> %s", d.Type, d.Name, d.Value)
	case dnsrecords.OutcomeUpdated:
		return fmt.Sprintf("would replace the existing record with %s %s -> %s", d.Type, d.Name, d.Value)
	case dnsrecords.OutcomeUnchanged:
		return "the record already exists, nothing to do"
	default:
		return "a different record exists; it would not be overwritten"
	}
}

func (rt *Router) writeDomainDNS(ctx context.Context, r *http.Request, app, domain string, t *dnsTarget, d dnsrecords.Desired, plan dnsrecords.Plan, res domainDNSResult) (domainDNSResult, []undoAction) {
	rr := libdns.RR{Name: d.Name, Type: d.Type, Data: d.Value, TTL: d.TTL}
	var undo []undoAction
	if len(plan.Replace) > 0 {
		if _, err := t.Manager.DeleteRecords(ctx, t.Zone, recordsOf(plan.Replace)); err != nil {
			rt.logger.Error("api: dns automation: delete replaced failed", slog.String("error", err.Error()), slog.String("domain", domain))
			res.DNS, res.Message = dnsResultError, "could not replace the existing record"
			return res, nil
		}
		for _, old := range plan.Replace {
			undo = append(undo, undoAction{Kind: undoKindDNSRestore, Domain: domain, Name: old.Name, RecordType: old.Type, Value: old.Data, TTLSeconds: int(old.TTL.Seconds())})
			rt.recordDNSAudit(ctx, r, store.AuditActionDNSRecordDeleted, app, domain, old)
		}
	}
	if _, err := t.Manager.AppendRecords(ctx, t.Zone, []libdns.Record{rr}); err != nil {
		rt.logger.Error("api: dns automation: create record failed", slog.String("error", err.Error()), slog.String("domain", domain))
		res.DNS, res.Message = dnsResultError, "the DNS provider rejected the record"
		return res, undo
	}
	action := store.AuditActionDNSRecordCreated
	res.DNS = dnsrecords.OutcomeCreated
	if len(plan.Replace) > 0 {
		action, res.DNS = store.AuditActionDNSRecordUpdated, dnsrecords.OutcomeUpdated
	}
	rt.recordDNSAudit(ctx, r, action, app, domain, rr)
	undo = append(undo, undoAction{Kind: undoKindDNSRecord, Domain: domain, Name: d.Name, RecordType: d.Type, Value: d.Value})
	if ms := rt.managedDNS(); ms != nil {
		err := ms.SaveManagedDNSRecord(ctx, store.ManagedDNSRecord{Domain: domain, Provider: t.Provider, Zone: t.Zone, Name: d.Name, RecordType: d.Type, Value: d.Value, AppName: app})
		if err != nil {
			rt.logger.Warn("api: dns automation: track record failed", slog.String("error", err.Error()), slog.String("domain", domain))
		}
	}
	if res.Proxied {
		fqdn := strings.TrimSuffix(domain, ".")
		if _, err := t.Proxy.SetProxied(ctx, t.Zone, fqdn, d.Type, d.Value, true); err != nil {
			rt.logger.Warn("api: dns automation: set proxied failed", slog.String("error", err.Error()), slog.String("domain", domain))
			res.Proxied = false
			res.Message = "record created, but Cloudflare's proxy could not be enabled"
		} else {
			res.Message = "record created and proxied; set the Cloudflare SSL/TLS mode to Full (strict) or visitors may see redirect loops"
		}
	}
	return res, undo
}

func recordsOf(rrs []libdns.RR) []libdns.Record {
	out := make([]libdns.Record, 0, len(rrs))
	for _, rr := range rrs {
		out = append(out, rr)
	}
	return out
}

// removeManagedDNS deletes the records Levelrail created for domain, matched
// by exact value so a record someone has since changed is left alone.
func (rt *Router) removeManagedDNS(ctx context.Context, r *http.Request, app, domain string) []domainDNSResult {
	ms := rt.managedDNS()
	if ms == nil {
		return nil
	}
	rows, err := ms.ListManagedDNSRecords(ctx, domain)
	if err != nil || len(rows) == 0 {
		return nil
	}
	target, err := rt.resolveDNSTarget(ctx, domain)
	if err != nil || target == nil {
		return []domainDNSResult{{Domain: domain, DNS: dnsResultSkipped, Message: "the DNS provider is not reachable; the record was left in place"}}
	}
	var out []domainDNSResult
	for _, row := range rows {
		rr := libdns.RR{Name: row.Name, Type: row.RecordType, Data: row.Value}
		res := domainDNSResult{Domain: domain, Provider: target.Provider, Zone: target.Zone, Record: &dnsRecordResource{Name: row.Name, Type: row.RecordType, Value: row.Value}}
		if err := deleteExactRecord(ctx, target, rr); err != nil {
			rt.logger.Error("api: dns automation: delete managed record failed", slog.String("error", err.Error()), slog.String("domain", domain))
			res.DNS, res.Message = dnsResultError, "could not delete the record"
			out = append(out, res)
			continue
		}
		if err := ms.DeleteManagedDNSRecord(ctx, domain, row.RecordType); err != nil {
			rt.logger.Warn("api: dns automation: untrack record failed", slog.String("error", err.Error()), slog.String("domain", domain))
		}
		rt.recordDNSAudit(ctx, r, store.AuditActionDNSRecordDeleted, app, domain, rr)
		res.DNS, res.Message = "deleted", "removed the record Levelrail created"
		out = append(out, res)
	}
	return out
}

// recordDNSAudit writes one audit row for a DNS record change.
func (rt *Router) recordDNSAudit(ctx context.Context, r *http.Request, action, app, domain string, rr libdns.RR) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		return
	}
	method := http.MethodPost
	switch action {
	case store.AuditActionDNSRecordUpdated:
		method = http.MethodPut
	case store.AuditActionDNSRecordDeleted:
		method = http.MethodDelete
	}
	entry := store.AuditEntry{
		ID: id, ActorType: auditActorSystem, ActorID: "domain-automation", ActorName: "domain automation",
		Ability: AbilityRoot, Method: method, StatusCode: http.StatusOK,
		Path:       "/api/v1/apps/" + app + "/domains/" + domain + "/dns-records#" + rr.Type + ":" + rr.Name,
		CreatedAt:  store.FormatAuditTime(time.Now()),
		ClientKind: auditClientKindSystem, Action: action,
	}
	if r != nil {
		entry.RemoteAddr = clientIP(r)
		entry.ClientKind = clientKindFromUserAgent(r.Header.Get("User-Agent"))
		if pt, pid, _, err := rt.callerPrincipal(r); err == nil {
			entry.ActorType, entry.ActorID = "session", pid
			if pt == store.PrincipalTypeToken {
				entry.ActorType = "token"
			}
			entry.ActorName = rt.auditActorName(ctx, entry.ActorType, pid, pid)
		}
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save dns audit entry failed", slog.String("error", err.Error()), slog.String("domain", domain))
	}
}

// callerIsRoot reports whether the request's principal holds the root ability.
func (rt *Router) callerIsRoot(r *http.Request) bool {
	if r == nil {
		return false
	}
	_, _, abilities, err := rt.callerPrincipal(r)
	return err == nil && hasAbility(abilities, AbilityRoot)
}

// deleteExactRecord removes the one live record equal to want in name, type
// and data (matching the stored TTL too, which providers require), and does
// nothing when it is gone or has changed.
func deleteExactRecord(ctx context.Context, t *dnsTarget, want libdns.RR) error {
	live, err := t.Manager.GetRecords(ctx, t.Zone)
	if err != nil {
		return err
	}
	for _, rec := range live {
		rr := rec.RR()
		if strings.EqualFold(rr.Name, want.Name) && rr.Type == want.Type &&
			strings.EqualFold(strings.TrimSuffix(rr.Data, "."), strings.TrimSuffix(want.Data, ".")) {
			_, err := t.Manager.DeleteRecords(ctx, t.Zone, []libdns.Record{rec})
			return err
		}
	}
	return nil
}
