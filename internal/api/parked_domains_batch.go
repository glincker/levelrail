package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	domainBatchActionPark         = "park"
	domainBatchActionMove         = "move"
	domainBatchActionAttach       = "attach"
	domainBatchActionDeleteParked = "delete_parked"

	domainBatchBodyLimit = 1 << 20

	errCodeBatchTooLarge   = "batch_too_large"
	errCodeDuplicateDomain = "duplicate_domain"
)

type domainBatchCtxKey struct{}

// withBatchID adds the batch id to a lifecycle audit detail so the entries of
// one bulk request can be grouped; outside a batch the detail is unchanged.
func withBatchID(ctx context.Context, detail string) string {
	id, _ := ctx.Value(domainBatchCtxKey{}).(string)
	if id == "" {
		return detail
	}
	m := map[string]any{}
	if detail != "" {
		_ = json.Unmarshal([]byte(detail), &m)
	}
	m["batch_id"] = id
	b, err := json.Marshal(m)
	if err != nil {
		return detail
	}
	return string(b)
}

// domainBatchItem is one domain of a bulk request; the fields used depend on the action.
type domainBatchItem struct {
	Domain          string `json:"domain"`
	App             string `json:"app,omitempty"`
	From            string `json:"from,omitempty"`
	To              string `json:"to,omitempty"`
	Note            string `json:"note,omitempty"`
	Environment     string `json:"environment,omitempty"`
	TargetKind      string `json:"target_kind,omitempty"`
	FromKind        string `json:"from_kind,omitempty"`
	ToKind          string `json:"to_kind,omitempty"`
	FromEnvironment string `json:"from_environment,omitempty"`
	ToEnvironment   string `json:"to_environment,omitempty"`
}

// domainBatchRequest is the body of POST /api/v1/domains/batch.
type domainBatchRequest struct {
	Action          string            `json:"action"`
	Items           []domainBatchItem `json:"items"`
	Confirm         bool              `json:"confirm,omitempty"`
	DiscardSettings bool              `json:"discard_settings,omitempty"`
}

// domainBatchResult is the outcome of one item; Status is the HTTP status the single route would have returned.
type domainBatchResult struct {
	Domain               string   `json:"domain"`
	OK                   bool     `json:"ok"`
	Status               int      `json:"status"`
	Error                string   `json:"error,omitempty"`
	Code                 string   `json:"code,omitempty"`
	RequiresConfirmation bool     `json:"requires_confirmation,omitempty"`
	Message              string   `json:"message,omitempty"`
	RestoredSettings     []string `json:"restored_settings,omitempty"`
}

type domainBatchSummary struct {
	Total                int `json:"total"`
	Succeeded            int `json:"succeeded"`
	Failed               int `json:"failed"`
	RequiresConfirmation int `json:"requires_confirmation"`
}

// domainBatchResponse is the 200 body: items are independent, so a mixed outcome is not an error.
type domainBatchResponse struct {
	BatchID string              `json:"batch_id"`
	Action  string              `json:"action"`
	Results []domainBatchResult `json:"results"`
	Summary domainBatchSummary  `json:"summary"`
}

// batchItemWriter records what a single-route core would have written so the
// batch can fold it into one result.
type batchItemWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (c *batchItemWriter) Header() http.Header { return c.header }
func (c *batchItemWriter) Write(b []byte) (int, error) {
	if c.status == 0 {
		c.status = http.StatusOK
	}
	return c.body.Write(b)
}
func (c *batchItemWriter) WriteHeader(status int) {
	if c.status == 0 {
		c.status = status
	}
}

// handleDomainsBatch handles POST /api/v1/domains/batch. Items run in order
// through the same cores as the single routes, each authorized on its own, and
// one failing item never stops or rolls back the others.
func (rt *Router) handleDomainsBatch(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.requireParkedStore(w)
	if !ok {
		return
	}
	var req domainBatchRequest
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, domainBatchBodyLimit))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	switch req.Action {
	case domainBatchActionPark, domainBatchActionMove, domainBatchActionAttach, domainBatchActionDeleteParked:
	default:
		writeError(w, http.StatusBadRequest, "action must be park, move, attach or delete_parked")
		return
	}
	if len(req.Items) == 0 {
		writeError(w, http.StatusBadRequest, "items must not be empty")
		return
	}
	if limit := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeyDomainsBatchMaxItems); limit > 0 && len(req.Items) > limit {
		writeCodedError(w, http.StatusBadRequest, errCodeBatchTooLarge, "a batch holds at most "+strconv.Itoa(limit)+" items")
		return
	}
	batchID, err := store.NewAuditEntryID()
	if err != nil {
		rt.internalError(w, "api: domain batch id", err)
		return
	}
	br := r.WithContext(context.WithValue(r.Context(), domainBatchCtxKey{}, batchID))
	out := domainBatchResponse{BatchID: batchID, Action: req.Action, Results: make([]domainBatchResult, 0, len(req.Items))}
	seen := map[string]bool{}
	for _, item := range req.Items {
		domain := normalizeStandaloneDomain(item.Domain)
		var res domainBatchResult
		switch {
		case domain == "":
			res = domainBatchResult{Domain: item.Domain, Status: http.StatusBadRequest, Error: "domain is required", Code: errorCodeForStatus(http.StatusBadRequest)}
		case seen[domain]:
			res = domainBatchResult{Domain: domain, Status: http.StatusBadRequest, Error: "domain appears more than once in this batch", Code: errCodeDuplicateDomain}
		default:
			seen[domain] = true
			res = rt.runBatchItem(br, st, req, item, domain)
		}
		out.Results = append(out.Results, res)
		out.Summary.Total++
		switch {
		case res.OK:
			out.Summary.Succeeded++
		case res.RequiresConfirmation:
			out.Summary.RequiresConfirmation++
		default:
			out.Summary.Failed++
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) runBatchItem(r *http.Request, st parkedDomainStore, req domainBatchRequest, item domainBatchItem, domain string) domainBatchResult {
	cw := &batchItemWriter{header: http.Header{}}
	switch req.Action {
	case domainBatchActionPark:
		rt.parkDomain(cw, r, st, domainMoveRequest{Domain: domain, App: item.App, Note: item.Note, Environment: item.Environment, TargetKind: item.TargetKind})
	case domainBatchActionMove:
		rt.moveDomain(cw, r, st, domainMoveRequest{
			Domain: domain, From: item.From, To: item.To, FromKind: item.FromKind, ToKind: item.ToKind,
			FromEnvironment: item.FromEnvironment, ToEnvironment: item.ToEnvironment, Confirm: req.Confirm,
		})
	case domainBatchActionAttach:
		rt.attachParkedDomain(cw, r, st, domain, domainMoveRequest{App: item.App, Environment: item.Environment, TargetKind: item.TargetKind, DiscardSettings: req.DiscardSettings})
	default:
		rt.deleteParkedDomain(cw, r, st, domain)
	}
	if cw.status == 0 {
		cw.status = http.StatusOK
	}
	res := domainBatchResult{Domain: domain, Status: cw.status, OK: cw.status >= 200 && cw.status < 300}
	var body struct {
		Error                string   `json:"error"`
		Code                 string   `json:"code"`
		Message              string   `json:"message"`
		RequiresConfirmation bool     `json:"requires_confirmation"`
		RestoredSettings     []string `json:"restored_settings"`
	}
	if cw.body.Len() > 0 && json.Unmarshal(cw.body.Bytes(), &body) == nil {
		res.Code, res.RestoredSettings, res.Message = body.Code, body.RestoredSettings, body.Message
		res.RequiresConfirmation = body.RequiresConfirmation
		if !res.OK && !res.RequiresConfirmation {
			res.Error, res.Message = strings.TrimSpace(body.Error), ""
		}
	}
	return res
}
