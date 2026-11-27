package api

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/certfallback"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/tlsowner"
)

// annotateDomainFallback flags ACME-owned domains whose stored certificate is
// the untrusted local-issuer fallback. Best effort: a read failure omits it.
func (rt *Router) annotateDomainFallback(ctx context.Context, rows []domainResource) {
	if rt.certs == nil || len(rows) == 0 {
		return
	}
	held, err := certfallback.Held(ctx, rt.certs)
	if err != nil {
		rt.logger.Warn("api: list domains: fallback certificates skipped", slog.String("error", err.Error()))
		return
	}
	if len(held) == 0 {
		return
	}
	snap := rt.tlsSnapshot(ctx)
	for i := range rows {
		d := strings.ToLower(rows[i].Domain)
		rows[i].Fallback = len(held[d]) > 0 && snap.Owner(d) == tlsowner.ACME
	}
}

// CertFallbackCandidates lists the ACME-owned app domains the fallback retry may
// act on; internal, proxy and custom-certificate domains are left alone.
func (rt *Router) CertFallbackCandidates(ctx context.Context) []string {
	domains, err := rt.domains.ListServiceDomains(ctx)
	if err != nil {
		return nil
	}
	snap := rt.tlsSnapshot(ctx)
	var out []string
	for _, d := range domains {
		if snap.Owner(d.Domain) == tlsowner.ACME {
			out = append(out, d.Domain)
		}
	}
	return out
}

// AuditCertFallbackDrop records that a stored fallback certificate was dropped.
func (rt *Router) AuditCertFallbackDrop(ctx context.Context, actorType, actorID, actorName, domain, detail string) {
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: cert fallback audit id failed", slog.String("error", err.Error()))
		return
	}
	entry := store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: actorName,
		Ability: AbilityRoot, Method: http.MethodDelete, Path: "/api/v1/domains/" + domain + "/cert#fallback: " + detail,
		StatusCode: http.StatusOK, CreatedAt: store.FormatAuditTime(time.Now()),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(ctx), entry); err != nil {
		rt.logger.Warn("api: cert fallback audit save failed", slog.String("error", err.Error()), slog.String("domain", domain))
	}
}
