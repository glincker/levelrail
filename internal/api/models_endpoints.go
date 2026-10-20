package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/models"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EndpointService is the external endpoint surface of the model service.
// A ModelService that also implements it enables the endpoint routes.
type EndpointService interface {
	TestEndpointInput(ctx context.Context, in models.ProbeInput) (models.EndpointProbe, error)
	TestEndpoint(ctx context.Context, name string) (models.EndpointProbe, error)
	SetEndpointRouting(ctx context.Context, name, fallback, fallbackRef string, budget time.Duration) error
	SetEndpointCredential(ctx context.Context, name, credential string) (models.EndpointProbe, error)
}

type externalRequest struct {
	BaseURL          string `json:"base_url"`
	APIType          string `json:"api_type"`
	Credential       string `json:"credential"`
	FallbackModel    string `json:"fallback_model"`
	FallbackModelRef string `json:"fallback_model_ref"`
	LatencyBudgetMS  int64  `json:"latency_budget_ms"`
}

type endpointDiagnosis struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix"`
}

type endpointHealthResource struct {
	State     string             `json:"state"`
	LatencyMS int64              `json:"latency_ms"`
	CheckedAt *time.Time         `json:"checked_at,omitempty"`
	LastOKAt  *time.Time         `json:"last_ok_at,omitempty"`
	Failures  int                `json:"consecutive_failures"`
	Diagnosis *endpointDiagnosis `json:"diagnosis,omitempty"`
}

type endpointResource struct {
	BaseURL          string                 `json:"base_url"`
	APIType          string                 `json:"api_type"`
	CredentialSet    bool                   `json:"credential_set"`
	FallbackModel    string                 `json:"fallback_model,omitempty"`
	FallbackModelRef string                 `json:"fallback_model_ref,omitempty"`
	LatencyBudgetMS  int64                  `json:"latency_budget_ms"`
	FallbackCount    int                    `json:"fallback_count"`
	Models           []string               `json:"models"`
	DiscoveryError   string                 `json:"discovery_error,omitempty"`
	PrivateAllowed   bool                   `json:"private_allowed"`
	Health           endpointHealthResource `json:"health"`
}

// fixFor maps a diagnosis code to the typed fix the dashboard offers.
var fixFor = map[string]string{
	models.DiagRefused:         "check_port",
	models.DiagTimeout:         "check_network",
	models.DiagDNS:             "check_host",
	models.DiagBlocked:         "use_other_address",
	models.DiagPrivateDisabled: "enable_internal",
	models.DiagAuth:            "set_credential",
	models.DiagWrongPort:       "check_port",
	models.DiagNotFound:        "check_api_type",
	models.DiagBadResponse:     "check_api_type",
	models.DiagTLS:             "use_http_or_trust",
	models.DiagNoModels:        "pull_model",
	models.DiagUpstream:        "check_server",
}

func diagnosisFor(code, message string) *endpointDiagnosis {
	if code == "" {
		return nil
	}
	return &endpointDiagnosis{Code: code, Message: message, Fix: fixFor[code]}
}

func splitHealthError(s string) (code, message string) {
	code, message, ok := strings.Cut(s, ": ")
	if !ok {
		return "", s
	}
	return code, message
}

func toEndpointResource(e *models.EndpointView) *endpointResource {
	if e == nil {
		return nil
	}
	code, msg := splitHealthError(e.HealthError)
	if code == "" && e.DiscoveryError != "" && len(e.Discovered) == 0 && e.Ready {
		code, msg = models.DiagNoModels, e.DiscoveryError
	}
	return &endpointResource{
		BaseURL: e.BaseURL, APIType: e.APIType, CredentialSet: e.CredentialSet,
		FallbackModel: e.FallbackModel, FallbackModelRef: e.FallbackRef, LatencyBudgetMS: e.LatencyBudget.Milliseconds(),
		FallbackCount: e.FallbackCount, Models: nonNilModels(e.Discovered), DiscoveryError: e.DiscoveryError,
		PrivateAllowed: models.EndpointPrivateAllowed(),
		Health: endpointHealthResource{
			State: e.HealthState, LatencyMS: e.HealthLatency.Milliseconds(), CheckedAt: optTime(e.HealthCheckedAt),
			LastOKAt: optTime(e.LastOKAt), Failures: e.Failures, Diagnosis: diagnosisFor(code, msg),
		},
	}
}

func nonNilModels(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

type endpointTestRequest struct {
	BaseURL    string `json:"base_url"`
	APIType    string `json:"api_type"`
	Credential string `json:"credential"`
}

type endpointTestResponse struct {
	State     string             `json:"state"`
	LatencyMS int64              `json:"latency_ms"`
	Models    []string           `json:"models"`
	Diagnosis *endpointDiagnosis `json:"diagnosis,omitempty"`
}

func toEndpointTestResponse(p models.EndpointProbe) endpointTestResponse {
	return endpointTestResponse{State: p.State, LatencyMS: p.Latency.Milliseconds(), Models: nonNilModels(p.Models), Diagnosis: diagnosisFor(p.Code, p.Message)}
}

func (rt *Router) endpointsConfigured(w http.ResponseWriter) (EndpointService, bool) {
	if !rt.modelsConfigured(w) {
		return nil, false
	}
	es, ok := rt.models.(EndpointService)
	if !ok {
		writeError(w, http.StatusNotImplemented, "external inference endpoints are not configured on this control plane")
		return nil, false
	}
	return es, true
}

func (rt *Router) writeEndpointError(w http.ResponseWriter, op string, err error) {
	switch {
	case errors.Is(err, models.ErrEndpointBlocked), errors.Is(err, models.ErrEndpointPrivate):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, store.ErrEndpointNotFound):
		writeError(w, http.StatusNotFound, "model is not an external endpoint")
	default:
		rt.writeModelError(w, op, err)
	}
}

// handleTestEndpointInput handles POST /api/v1/model-endpoints/test, probing
// an endpoint before it is saved.
func (rt *Router) handleTestEndpointInput(w http.ResponseWriter, r *http.Request) {
	es, ok := rt.endpointsConfigured(w)
	if !ok {
		return
	}
	var req endpointTestRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	res, err := es.TestEndpointInput(r.Context(), models.ProbeInput{BaseURL: req.BaseURL, APIType: req.APIType, Credential: req.Credential})
	if err != nil {
		rt.writeEndpointError(w, "test endpoint", err)
		return
	}
	writeJSON(w, http.StatusOK, toEndpointTestResponse(res))
}

// handleTestEndpoint handles POST /api/v1/models/{name}/endpoint/test.
func (rt *Router) handleTestEndpoint(w http.ResponseWriter, r *http.Request) {
	es, ok := rt.endpointsConfigured(w)
	if !ok {
		return
	}
	res, err := es.TestEndpoint(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writeEndpointError(w, "test endpoint", err)
		return
	}
	writeJSON(w, http.StatusOK, toEndpointTestResponse(res))
}

type endpointRoutingRequest struct {
	FallbackModel    string `json:"fallback_model"`
	FallbackModelRef string `json:"fallback_model_ref"`
	LatencyBudgetMS  int64  `json:"latency_budget_ms"`
}

// handleSetEndpointRouting handles PUT /api/v1/models/{name}/endpoint/routing.
// An empty fallback_model turns the policy off.
func (rt *Router) handleSetEndpointRouting(w http.ResponseWriter, r *http.Request) {
	es, ok := rt.endpointsConfigured(w)
	if !ok {
		return
	}
	var req endpointRoutingRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.authorizeRefs(w, r, AbilityWrite, notFoundAs(http.StatusBadRequest, "unknown fallback_model"), modelRef(req.FallbackModel)) {
		return
	}
	budget := time.Duration(req.LatencyBudgetMS) * time.Millisecond
	if err := es.SetEndpointRouting(r.Context(), r.PathValue("name"), req.FallbackModel, req.FallbackModelRef, budget); err != nil {
		rt.writeEndpointError(w, "set endpoint routing", err)
		return
	}
	rt.getModelResponse(w, r)
}

type endpointCredentialRequest struct {
	Credential string `json:"credential"`
}

// handleSetEndpointCredential handles PUT /api/v1/models/{name}/endpoint/credential.
// An empty credential removes it.
func (rt *Router) handleSetEndpointCredential(w http.ResponseWriter, r *http.Request) {
	es, ok := rt.endpointsConfigured(w)
	if !ok {
		return
	}
	var req endpointCredentialRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if _, err := es.SetEndpointCredential(r.Context(), r.PathValue("name"), req.Credential); err != nil {
		rt.writeEndpointError(w, "set endpoint credential", err)
		return
	}
	rt.getModelResponse(w, r)
}

func (rt *Router) getModelResponse(w http.ResponseWriter, r *http.Request) {
	view, err := rt.models.Get(r.Context(), r.PathValue("name"))
	if err != nil {
		rt.writeModelError(w, "reload model", err)
		return
	}
	writeJSON(w, http.StatusOK, toModelResource(view))
}
