package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dnsrecords"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/libdns/libdns"
)

// dnsRecordsInternalError is the fixed body every 500 response in this
// file uses, the same "don't leak internals, the log line above already
// has the real cause" reasoning route53DNSInternalError documents.
const dnsRecordsInternalError = "internal error"

// dnsRecordDefaultTTLSeconds is used whenever a create/update request
// omits ttl_seconds or passes 0: libdns.RR's own doc comment treats a
// zero TTL as "do not cache", which several providers either reject or
// silently clamp, so this avoids sending that edge case upstream.
const dnsRecordDefaultTTLSeconds = 300

// dnsRecordTypes is the set of record types this view supports editing.
// NS and SOA are deliberately excluded from both this set and from
// GetRecords's own output below: libdns.RecordDeleter's doc comment
// calls removing a zone's last NS record undefined behavior, and SOA is
// provider-managed, so neither belongs in an operator-facing CRUD table.
var dnsRecordTypes = map[string]bool{
	"A": true, "AAAA": true, "CNAME": true, "TXT": true,
	"MX": true, "SRV": true, "CAA": true,
}

// CloudflareDNSTokenResolver is the narrow surface GET/POST/PUT/DELETE
// .../dns-records need from internal/secrets.Manager to read back the
// plaintext Cloudflare DNS-01 token this control plane already stores:
// CloudflareDNSSecrets (cloudflare_dns.go) only checks presence, it
// never reads the value back, since the settings resource never needs
// to. *secrets.Manager satisfies this structurally, the same shape
// internal/reconcile/ingress's own identically-named interface already
// establishes for the same credential.
type CloudflareDNSTokenResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// Route53DNSCredentialResolver is the same shape as
// CloudflareDNSTokenResolver for the Route53 access key pair.
type Route53DNSCredentialResolver interface {
	Resolve(ctx context.Context, serviceName, envKey string) (string, error)
}

// dnsRecordManagerFunc resolves the record manager, provider name, and
// best-effort zone for domain. A nil Manager with a nil error means
// neither DNS-01 provider is configured and enabled: callers return 501,
// not an error response. Matches lookupHostFunc's own override-seam
// shape (domain_check.go) so this file's tests swap in a fake Manager
// instead of calling Cloudflare or Route53's real APIs.
type dnsRecordManagerFunc func(ctx context.Context, domain string) (mgr dnsrecords.Manager, provider, zone string, err error)

// dnsRecordStatusFunc reports whether record's configured value is
// already visible in live DNS. Same override-seam shape as
// dnsRecordManagerFunc, for the same reason: no real DNS query in tests.
type dnsRecordStatusFunc func(ctx context.Context, domain string, record dnsRecordResource) string

// Record check status values.
const (
	dnsRecordStatusResolved = "resolved"
	dnsRecordStatusPending  = "pending"
	dnsRecordStatusMismatch = "mismatch"
	dnsRecordStatusUnknown  = "unknown"
)

// dnsRecordResource is one record's wire shape, shared by the list
// response and every create/update/delete request body.
type dnsRecordResource struct {
	Name       string `json:"name"`
	Type       string `json:"type"`
	Value      string `json:"value"`
	TTLSeconds int    `json:"ttl_seconds"`
	Status     string `json:"status,omitempty"`
}

// dnsRecordsResponse is GET .../dns-records's wire shape: domain anchors
// the request, but Zone/Records cover the whole zone, since an operator
// managing api.example.com's records very likely also wants to see its
// zone's TXT/MX records (SPF, DKIM, mail routing) in the same table.
type dnsRecordsResponse struct {
	Domain   string              `json:"domain"`
	Provider string              `json:"provider"`
	Zone     string              `json:"zone"`
	Records  []dnsRecordResource `json:"records"`
}

// updateDNSRecordRequest is PUT .../dns-records's body: Original
// identifies the exact existing record (name, type, value all must
// match, the same exact-match contract libdns.RecordDeleter documents),
// Record is what it should become.
type updateDNSRecordRequest struct {
	Original dnsRecordResource `json:"original"`
	Record   dnsRecordResource `json:"record"`
}

// dnsZoneApexGuess returns domain's best-effort DNS zone: its last two
// labels, trailing-dot qualified the way libdns expects. Multi-label
// public suffixes (co.uk, com.au) make this imprecise; it accepts the
// same best-effort tradeoff advertisedHost (domain_check.go) already
// makes for its own inferred host, rather than vendoring a public
// suffix list for one feature. The resolved zone is always echoed back
// in dnsRecordsResponse so an operator can see exactly what was queried.
func dnsZoneApexGuess(domain string) string {
	labels := strings.Split(domain, ".")
	if len(labels) > 2 {
		labels = labels[len(labels)-2:]
	}
	return strings.Join(labels, ".") + "."
}

// resolveDNSRecordManager is the default dnsRecordManagerFunc. Cloudflare
// takes precedence when both providers are enabled, the same tie-break
// resolveDNSProvider (internal/reconcile/ingress/controller.go) already
// uses for ACME DNS-01 itself.
func (rt *Router) resolveDNSRecordManager(ctx context.Context, domain string) (dnsrecords.Manager, string, string, error) {
	zone := dnsZoneApexGuess(domain)

	mgr, err := rt.resolveCloudflareRecordManager(ctx)
	if err != nil {
		return nil, "", "", err
	}
	if mgr != nil {
		return mgr, "cloudflare", zone, nil
	}

	mgr, err = rt.resolveRoute53RecordManager(ctx)
	if err != nil || mgr == nil {
		return nil, "", "", err
	}
	return mgr, "route53", zone, nil
}

func (rt *Router) resolveCloudflareRecordManager(ctx context.Context) (dnsrecords.Manager, error) {
	token, err := rt.cloudflareDNSToken(ctx)
	if err != nil || token == "" {
		return nil, err
	}
	return dnsrecords.NewCloudflareManager(token), nil
}

// cloudflareDNSToken returns the stored Cloudflare token, or "" when the
// provider is not configured and enabled.
func (rt *Router) cloudflareDNSToken(ctx context.Context) (string, error) {
	if rt.cloudflareDNSTokenResolver == nil {
		return "", nil
	}
	settings, err := rt.cloudflareDNS.GetCloudflareDNSSettings(ctx)
	if err != nil {
		return "", fmt.Errorf("api: get cloudflare dns settings: %w", err)
	}
	if !settings.Enabled {
		return "", nil
	}
	token, err := rt.cloudflareDNSTokenResolver.Resolve(ctx, store.CloudflareDNSSecretsKey(), store.CloudflareDNSTokenEnvKey)
	if err != nil {
		return "", fmt.Errorf("api: resolve cloudflare dns token: %w", err)
	}
	return token, nil
}

func (rt *Router) resolveRoute53RecordManager(ctx context.Context) (dnsrecords.Manager, error) {
	if rt.route53DNSCredentialResolver == nil {
		return nil, nil
	}
	settings, err := rt.route53DNS.GetRoute53DNSSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("api: get route53 dns settings: %w", err)
	}
	if !settings.Enabled {
		return nil, nil
	}
	key := store.Route53DNSSecretsKey()
	accessKeyID, err := rt.route53DNSCredentialResolver.Resolve(ctx, key, store.Route53DNSAccessKeyIDEnvKey)
	if err != nil {
		return nil, fmt.Errorf("api: resolve route53 access key id: %w", err)
	}
	secretAccessKey, err := rt.route53DNSCredentialResolver.Resolve(ctx, key, store.Route53DNSSecretAccessKeyEnvKey)
	if err != nil {
		return nil, fmt.Errorf("api: resolve route53 secret access key: %w", err)
	}
	if accessKeyID == "" || secretAccessKey == "" {
		return nil, nil
	}
	return dnsrecords.NewRoute53Manager(accessKeyID, secretAccessKey, settings.Region, settings.HostedZoneID), nil
}

// defaultDNSRecordStatus is the default dnsRecordStatusFunc: a real,
// bounded-timeout DNS lookup for record's exact type, compared against
// its configured value. "pending" covers both "not resolving yet" and a
// lookup error, since an operator can't act differently on the two; a
// genuinely broken resolver would also fail handleCheckDomain's own
// lookup right next to this one in the UI.
func defaultDNSRecordStatus(ctx context.Context, domain string, record dnsRecordResource) string {
	fqdn := domain
	if record.Name != "" && record.Name != "@" {
		fqdn = record.Name + "." + domain
	}
	lookupCtx, cancel := context.WithTimeout(ctx, domainCheckLookupTimeout)
	defer cancel()

	switch strings.ToUpper(record.Type) {
	case "A", "AAAA":
		hosts, err := net.DefaultResolver.LookupHost(lookupCtx, fqdn)
		return matchStatus(err == nil && len(hosts) > 0, containsFold(hosts, record.Value))
	case "CNAME":
		cname, err := net.DefaultResolver.LookupCNAME(lookupCtx, fqdn)
		return matchStatus(err == nil, strings.EqualFold(strings.TrimSuffix(cname, "."), strings.TrimSuffix(record.Value, ".")))
	case "TXT":
		txts, err := net.DefaultResolver.LookupTXT(lookupCtx, fqdn)
		return matchStatus(err == nil && len(txts) > 0, containsFold(txts, record.Value))
	case "MX":
		mxs, err := net.DefaultResolver.LookupMX(lookupCtx, fqdn)
		if err != nil || len(mxs) == 0 {
			return dnsRecordStatusPending
		}
		for _, mx := range mxs {
			if strings.EqualFold(strings.TrimSuffix(mx.Host, "."), strings.TrimSuffix(record.Value, ".")) {
				return dnsRecordStatusResolved
			}
		}
		return dnsRecordStatusMismatch
	default:
		return dnsRecordStatusUnknown
	}
}

func containsFold(vals []string, want string) bool {
	for _, v := range vals {
		if strings.EqualFold(v, want) {
			return true
		}
	}
	return false
}

func matchStatus(resolved, matches bool) string {
	switch {
	case !resolved:
		return dnsRecordStatusPending
	case matches:
		return dnsRecordStatusResolved
	default:
		return dnsRecordStatusMismatch
	}
}

// toDNSRecordResources converts libdns records into the wire shape,
// dropping NS/SOA (see dnsRecordTypes's own doc comment) and annotating
// each with a live-lookup status via rt.dnsRecordStatus.
func (rt *Router) toDNSRecordResources(ctx context.Context, domain string, recs []libdns.Record) []dnsRecordResource {
	out := make([]dnsRecordResource, 0, len(recs))
	for _, rec := range recs {
		rr := rec.RR()
		if rr.Type == "NS" || rr.Type == "SOA" {
			continue
		}
		res := dnsRecordResource{Name: rr.Name, Type: rr.Type, Value: rr.Data, TTLSeconds: int(rr.TTL.Seconds())}
		res.Status = rt.dnsRecordStatus(ctx, domain, res)
		out = append(out, res)
	}
	return out
}

// requireDNSRecordManager runs requireOwnedDomain plus the provider
// resolution every DNS record handler needs, writing the right error
// response itself when either step fails. ok is false if the caller
// should return immediately.
func (rt *Router) requireDNSRecordManager(w http.ResponseWriter, r *http.Request) (domain string, mgr dnsrecords.Manager, provider, zone string, ok bool) {
	domain, ok = rt.requireOwnedDomain(w, r)
	if !ok {
		return "", nil, "", "", false
	}

	mgr, provider, zone, err := rt.dnsRecordManager(r.Context(), domain)
	if err != nil {
		rt.logger.Error("api: resolve dns record manager failed", slog.String("error", err.Error()), slog.String("domain", domain))
		writeError(w, http.StatusInternalServerError, dnsRecordsInternalError)
		return "", nil, "", "", false
	}
	if mgr == nil {
		writeError(w, http.StatusNotImplemented, "no DNS provider is configured for records management (enable Cloudflare DNS or Route53 DNS under Domains)")
		return "", nil, "", "", false
	}
	return domain, mgr, provider, zone, true
}

// writeDNSRecords re-fetches zone's records and writes the full list
// response, shared by all four handlers below so create/update/delete
// never return a response that can drift from what GetRecords itself
// would report right after.
func (rt *Router) writeDNSRecords(w http.ResponseWriter, r *http.Request, mgr dnsrecords.Manager, domain, provider, zone string) {
	recs, err := mgr.GetRecords(r.Context(), zone)
	if err != nil {
		rt.logger.Error("api: list dns records failed", slog.String("error", err.Error()), slog.String("domain", domain), slog.String("zone", zone))
		writeError(w, http.StatusInternalServerError, dnsRecordsInternalError)
		return
	}
	writeJSON(w, http.StatusOK, dnsRecordsResponse{
		Domain: domain, Provider: provider, Zone: zone,
		Records: rt.toDNSRecordResources(r.Context(), domain, recs),
	})
}

// validateDNSRecord checks req and returns the libdns.RR it describes.
// Name empty defaults to "@" (the zone apex, libdns's own convention).
// TTLSeconds <= 0 defaults to dnsRecordDefaultTTLSeconds.
func validateDNSRecord(w http.ResponseWriter, req dnsRecordResource) (libdns.RR, bool) {
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "@"
	}
	typ := strings.ToUpper(strings.TrimSpace(req.Type))
	if !dnsRecordTypes[typ] {
		writeError(w, http.StatusBadRequest, "type must be one of A, AAAA, CNAME, TXT, MX, SRV, CAA")
		return libdns.RR{}, false
	}
	value := strings.TrimSpace(req.Value)
	if value == "" {
		writeError(w, http.StatusBadRequest, "value is required")
		return libdns.RR{}, false
	}
	ttl := req.TTLSeconds
	if ttl <= 0 {
		ttl = dnsRecordDefaultTTLSeconds
	}
	return libdns.RR{Name: name, Type: typ, Data: value, TTL: time.Duration(ttl) * time.Second}, true
}

// handleListDNSRecords handles GET
// /api/v1/apps/{name}/domains/{domain}/dns-records: every record in
// domain's best-effort zone (see dnsZoneApexGuess), from whichever
// DNS-01 provider is configured, each annotated with a live resolution
// status. AbilityRead: a live read against the provider's API plus a
// DNS lookup, no write. Returns 501 when neither Cloudflare DNS nor
// Route53 DNS is configured and enabled.
func (rt *Router) handleListDNSRecords(w http.ResponseWriter, r *http.Request) {
	domain, mgr, provider, zone, ok := rt.requireDNSRecordManager(w, r)
	if !ok {
		return
	}
	rt.writeDNSRecords(w, r, mgr, domain, provider, zone)
}

// handleCreateDNSRecord handles POST
// /api/v1/apps/{name}/domains/{domain}/dns-records: appends a new record
// to domain's zone via libdns.RecordAppender, leaving any existing
// record with the same name and type untouched. AbilityRoot: a write
// against the external DNS provider's live zone, the same "real
// infrastructure, high blast radius" tier PUT .../tls-cert and
// .../auth already reserve for a credential-bearing or externally
// visible change.
func (rt *Router) handleCreateDNSRecord(w http.ResponseWriter, r *http.Request) {
	domain, mgr, provider, zone, ok := rt.requireDNSRecordManager(w, r)
	if !ok {
		return
	}

	var req dnsRecordResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rr, ok := validateDNSRecord(w, req)
	if !ok {
		return
	}

	if _, err := mgr.AppendRecords(r.Context(), zone, []libdns.Record{rr}); err != nil {
		rt.logger.Error("api: create dns record failed", slog.String("error", err.Error()), slog.String("domain", domain), slog.String("zone", zone))
		writeError(w, http.StatusInternalServerError, dnsRecordsInternalError)
		return
	}
	rt.writeDNSRecords(w, r, mgr, domain, provider, zone)
}

// handleUpdateDNSRecord handles PUT
// /api/v1/apps/{name}/domains/{domain}/dns-records: replaces one exact
// record (body.original) with a new value (body.record), implemented as
// an exact-match delete followed by an append rather than
// libdns.RecordSetter, since SetRecords replaces an entire name+type
// RRset and would silently drop sibling records (e.g. a second MX or
// TXT) that body.original never mentioned. AbilityRoot, same tier as
// handleCreateDNSRecord.
func (rt *Router) handleUpdateDNSRecord(w http.ResponseWriter, r *http.Request) {
	domain, mgr, provider, zone, ok := rt.requireDNSRecordManager(w, r)
	if !ok {
		return
	}

	var req updateDNSRecordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	oldRR, ok := validateDNSRecord(w, req.Original)
	if !ok {
		return
	}
	newRR, ok := validateDNSRecord(w, req.Record)
	if !ok {
		return
	}

	if _, err := mgr.DeleteRecords(r.Context(), zone, []libdns.Record{oldRR}); err != nil {
		rt.logger.Error("api: update dns record: delete original failed", slog.String("error", err.Error()), slog.String("domain", domain), slog.String("zone", zone))
		writeError(w, http.StatusInternalServerError, dnsRecordsInternalError)
		return
	}
	if _, err := mgr.AppendRecords(r.Context(), zone, []libdns.Record{newRR}); err != nil {
		rt.logger.Error("api: update dns record: append replacement failed", slog.String("error", err.Error()), slog.String("domain", domain), slog.String("zone", zone))
		writeError(w, http.StatusInternalServerError, dnsRecordsInternalError)
		return
	}
	rt.writeDNSRecords(w, r, mgr, domain, provider, zone)
}

// handleDeleteDNSRecord handles DELETE
// /api/v1/apps/{name}/domains/{domain}/dns-records: removes one exact
// record (name, type, and value must all match, libdns.RecordDeleter's
// own contract). Idempotent: deleting a record that no longer exists is
// not an error. AbilityRoot, same tier as handleCreateDNSRecord.
func (rt *Router) handleDeleteDNSRecord(w http.ResponseWriter, r *http.Request) {
	domain, mgr, provider, zone, ok := rt.requireDNSRecordManager(w, r)
	if !ok {
		return
	}

	var req dnsRecordResource
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rr, ok := validateDNSRecord(w, req)
	if !ok {
		return
	}

	if _, err := mgr.DeleteRecords(r.Context(), zone, []libdns.Record{rr}); err != nil {
		rt.logger.Error("api: delete dns record failed", slog.String("error", err.Error()), slog.String("domain", domain), slog.String("zone", zone))
		writeError(w, http.StatusInternalServerError, dnsRecordsInternalError)
		return
	}
	rt.writeDNSRecords(w, r, mgr, domain, provider, zone)
}
