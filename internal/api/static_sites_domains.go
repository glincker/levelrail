package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/glincker/levelrail/internal/dnsrecords"
	"github.com/glincker/levelrail/internal/mailrecords"
	"github.com/glincker/levelrail/internal/spec"
	"github.com/glincker/levelrail/internal/store"
)

// Modes of a Pages domain attach: records were written, or the caller copies them.
const (
	siteDomainModeAuto   = "auto"
	siteDomainModeManual = "manual"
)

type staticSiteDomainRequest struct {
	Domain string `json:"domain"`
}

type siteDomainRecord struct {
	Type  string `json:"type"`
	Name  string `json:"name"`
	Value string `json:"value"`
}

type staticSiteDomainResponse struct {
	Domain   string             `json:"domain"`
	Mode     string             `json:"mode"`
	Records  []siteDomainRecord `json:"records"`
	Verified bool               `json:"verified"`
	Cert     string             `json:"cert"`
	Detail   string             `json:"detail,omitempty"`
	// Diagnosis is only set by the status route.
	Diagnosis []siteDiagnosis `json:"diagnosis,omitempty"`
}

// handleAddStaticSiteDomain handles POST /api/v1/static-sites/{name}/domains:
// attaches the domain, then creates its DNS record when a provider manages
// the zone, otherwise returns the record to copy. It never waits on probes.
func (rt *Router) handleAddStaticSiteDomain(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.loadGoLiveApp(w, r)
	if !ok {
		return
	}
	var req staticSiteDomainRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, goLiveBodyLimit)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	domain, derr := spec.ValidateAppDomain(strings.TrimSpace(req.Domain))
	if derr != nil || strings.HasPrefix(domain, "*.") || mailrecords.ValidHost(domain) != nil {
		writeError(w, http.StatusBadRequest, "invalid domain")
		return
	}
	owns, err := rt.appOwnsDomain(r.Context(), name, domain)
	if err != nil {
		rt.internalError(w, "api: static site domain: ownership check failed", err)
		return
	}
	if !owns {
		editor, ok := rt.apps.(serviceDomainsEditor)
		if !ok {
			writeError(w, http.StatusNotImplemented, "editing domains is not supported by this store")
			return
		}
		_, _, err := editor.EditServiceDomains(r.Context(), name, func(cur []string) ([]string, error) {
			return applyDomainEdit(cur, []string{domain}, nil)
		})
		var taken *store.ErrDomainTaken
		switch {
		case errors.As(err, &taken):
			writeError(w, http.StatusConflict, taken.Error())
			return
		case err != nil:
			rt.internalError(w, "api: static site domain: attach failed", err, slog.String("name", name))
			return
		}
		rt.nudgeReconciler()
	}

	policy := rt.effectiveAutomation(r.Context(), r, nil)
	res := rt.goLive(r.Context(), r, name, domain, policy, goLiveRequest{skipVerify: true}, goLiveModeApply)
	rt.recordGoLiveRun(r.Context(), &res)

	out := staticSiteDomainResponse{Domain: domain, Mode: siteDomainModeManual, Records: []siteDomainRecord{}, Cert: "pending"}
	for _, d := range res.DNS {
		if d.Domain != domain {
			continue
		}
		switch d.DNS {
		case dnsrecords.OutcomeCreated, dnsrecords.OutcomeUpdated, dnsrecords.OutcomeUnchanged:
			out.Mode = siteDomainModeAuto
		default:
			out.Detail = d.Message
		}
		if d.Record != nil {
			out.Records = append(out.Records, siteDomainRecord{Type: d.Record.Type, Name: d.Record.Name, Value: d.Record.Value})
		}
	}
	if len(out.Records) == 0 {
		// No provider zone: still say what to point at, from the server's public address.
		if s, serr := rt.ingressSettings.GetIngressSettings(r.Context()); serr == nil {
			if typ, val, terr := rt.serverDNSTarget(r.Context(), s); terr == nil {
				out.Records = append(out.Records, siteDomainRecord{Type: typ, Name: domain, Value: strings.TrimSuffix(val, ".")})
			}
		}
	}
	if len(out.Records) == 0 && out.Detail == "" {
		out.Detail = "this server's public address is unknown; set APP_PUBLIC_HOST"
	}
	writeJSON(w, http.StatusOK, out)
}

// handleStaticSiteDomainStatus handles GET /api/v1/static-sites/{name}/domains/{domain}/status:
// a read-only probe of DNS propagation and the served certificate. The
// diagnosis rows are cached; ?diagnose=1 forces a fresh doctor run.
func (rt *Router) handleStaticSiteDomainStatus(w http.ResponseWriter, r *http.Request) {
	name, ok := rt.loadGoLiveApp(w, r)
	if !ok {
		return
	}
	domain := strings.ToLower(strings.TrimSpace(r.PathValue("domain")))
	owns, err := rt.appOwnsDomain(r.Context(), name, domain)
	if err != nil {
		rt.internalError(w, "api: static site domain status: ownership check failed", err)
		return
	}
	if !owns {
		writeError(w, http.StatusNotFound, "domain is not one of this site's domains")
		return
	}
	policy := rt.effectiveAutomation(r.Context(), r, nil)
	policy.VerifyAfter = true
	res := rt.goLive(r.Context(), r, name, domain, policy, goLiveRequest{}, goLiveModeStatus)

	out := staticSiteDomainResponse{Domain: domain, Mode: siteDomainModeManual, Records: []siteDomainRecord{}, Cert: "pending"}
	for _, s := range res.Steps {
		switch s.ID {
		case stepDNS:
			if s.State == stepDone {
				out.Mode = siteDomainModeAuto
			}
		case stepPropagation:
			out.Verified = s.State == stepDone
			if !out.Verified {
				out.Detail = s.Detail
			}
		case stepCertificate:
			if s.State == stepDone {
				out.Cert = "issued"
			}
		}
	}
	out.Diagnosis = rt.siteDomainDiagnosis(r.Context(), name, domain, r.URL.Query().Get("diagnose") == "1")
	writeJSON(w, http.StatusOK, out)
}
