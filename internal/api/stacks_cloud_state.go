package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/iac"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	auditStackCloudStateRecover = "stack.cloud_state_recover"
	stackEventStateRecovered    = "stacks.cloud_state_recovered"
)

// cloudStateAdminStore is the quarantine slice of the store; it is never given the sealed blob.
type cloudStateAdminStore interface {
	ListCloudStateQuarantineForStack(ctx context.Context, stack string) ([]store.CloudStateQuarantine, error)
	ResolveCloudStateQuarantineForStack(ctx context.Context, stack string, id int64, now time.Time) (string, error)
}

type cloudStateQuarantineRow struct {
	ID        int64  `json:"id"`
	Account   string `json:"account"`
	Reason    string `json:"reason"`
	Fence     int64  `json:"fence"`
	CreatedAt string `json:"created_at"`
}

type cloudStateQuarantineList struct {
	Stack      string                    `json:"stack"`
	Quarantine []cloudStateQuarantineRow `json:"quarantine"`
}

type cloudStateResolveRequest struct {
	Confirm string `json:"confirm"`
}

func (rt *Router) cloudStateAdmin(w http.ResponseWriter, r *http.Request) (cloudStateAdminStore, store.StackRecord, bool) {
	ss, ok := rt.stackStoreOr501(w)
	if !ok {
		return nil, store.StackRecord{}, false
	}
	cs, ok := ss.(cloudStateAdminStore)
	if !ok {
		writeIaCError(w, http.StatusNotImplemented, iac.CodeUnavailable, "cloud state is not available on this store", nil)
		return nil, store.StackRecord{}, false
	}
	if rt.stackCaller(r).isCI {
		writeIaCError(w, http.StatusForbidden, iac.CodePermissionDenied, "a CI token cannot manage cloud state", nil)
		return nil, store.StackRecord{}, false
	}
	s, ok := rt.loadStack(w, r, ss)
	return cs, s, ok
}

func (rt *Router) handleListCloudStateQuarantine(w http.ResponseWriter, r *http.Request) {
	cs, s, ok := rt.cloudStateAdmin(w, r)
	if !ok {
		return
	}
	rows, err := cs.ListCloudStateQuarantineForStack(r.Context(), s.Name)
	if err != nil {
		rt.writeStackError(w, "cloud-state", err)
		return
	}
	out := cloudStateQuarantineList{Stack: s.Name, Quarantine: make([]cloudStateQuarantineRow, 0, len(rows))}
	for _, q := range rows {
		out.Quarantine = append(out.Quarantine, cloudStateQuarantineRow{ID: q.ID, Account: q.Account, Reason: q.Reason, Fence: q.Fence, CreatedAt: fmtStackTime(q.CreatedAt)})
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleResolveCloudStateQuarantine(w http.ResponseWriter, r *http.Request) {
	cs, s, ok := rt.cloudStateAdmin(w, r)
	if !ok {
		return
	}
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeIaCError(w, http.StatusBadRequest, iac.CodeInvalidRequest, "the quarantine id must be a positive number", nil)
		return
	}
	var req cloudStateResolveRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil || req.Confirm != s.Name {
		writeIaCError(w, http.StatusBadRequest, iac.CodeInvalidRequest, "confirm must be the stack name, typed exactly", nil)
		return
	}
	account, err := cs.ResolveCloudStateQuarantineForStack(r.Context(), s.Name, id, time.Now())
	if errors.Is(err, store.ErrCloudStateQuarantineNotFound) {
		writeIaCError(w, http.StatusNotFound, errorCodeForStatus(http.StatusNotFound), "no unresolved quarantine row with this id for the stack", nil)
		return
	}
	if err != nil {
		rt.writeStackError(w, "cloud-state", err)
		return
	}
	rt.auditStack(r, auditStackCloudStateRecover, map[string]any{"stack": s.Name, "quarantine_id": id, "account": account})
	rt.emitStackEvent(r.Context(), stackEventStateRecovered, s.Name, rt.stackCaller(r).name, "", 0, nil, nil)
	writeJSON(w, http.StatusOK, map[string]any{"stack": s.Name, "id": id, "account": account, "resolved": true})
}
