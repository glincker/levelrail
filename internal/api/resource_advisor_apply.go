package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/resourceadvisor"
	"github.com/GLINCKER/levelrail/internal/store"
)

type advisorApplyRequest struct {
	Confirm bool `json:"confirm"`
}

// advisorSettings is the slice of a resource an advisor action changes.
type advisorSettings struct {
	MemoryBytes      int64 `json:"memory_bytes"`
	NanoCPUs         int64 `json:"nano_cpus"`
	SleepIdleMinutes int   `json:"sleep_idle_minutes,omitempty"`
}

type advisorApplyResponse struct {
	FindingID       string                  `json:"finding_id"`
	Resource        resourceadvisor.Ref     `json:"resource"`
	Action          *resourceadvisor.Action `json:"action"`
	Before          advisorSettings         `json:"before"`
	After           advisorSettings         `json:"after"`
	ConfirmRequired bool                    `json:"confirm_required"`
	Applied         bool                    `json:"applied"`
	AppliedLive     bool                    `json:"applied_live"`
}

var errAdvisorStale = errors.New("the setting changed since this suggestion was made; refresh the advisor and review it again")

// handleResourceAdvisorApply serves POST .../findings/{id}/apply. Without
// confirm it only previews; nothing changes until confirm is true.
func (rt *Router) handleResourceAdvisorApply(w http.ResponseWriter, r *http.Request) {
	var req advisorApplyRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	if r.URL.Query().Get("confirm") == "true" {
		req.Confirm = true
	}
	f, ok := rt.advisorFindingForWrite(w, r)
	if !ok {
		return
	}
	if f.Action == nil || !f.Action.Appliable() || f.Resource.Kind == resourceadvisor.ResourceNode {
		writeError(w, http.StatusConflict, "this finding has no action the advisor can apply; follow its instructions instead")
		return
	}
	resp := advisorApplyResponse{FindingID: f.ID, Resource: f.Resource, Action: f.Action, ConfirmRequired: !req.Confirm}
	var err error
	switch f.Resource.Kind {
	case resourceadvisor.ResourceApp:
		err = rt.applyAdvisorToApp(r, f, &resp, req.Confirm)
	case resourceadvisor.ResourceDatabase:
		err = rt.applyAdvisorToDatabase(r, f, &resp, req.Confirm)
	}
	switch {
	case errors.Is(err, errAdvisorStale):
		writeError(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, store.ErrServiceNotFound), errors.Is(err, store.ErrDatabaseNotFound):
		writeError(w, http.StatusNotFound, f.Resource.String()+" not found")
		return
	case err != nil:
		rt.internalError(w, "api: resource advisor apply", err, slog.String("finding", f.ID))
		return
	}
	if resp.Applied {
		rt.advisor.invalidate()
		rt.logger.Info("resource advisor: applied", slog.String("finding", f.ID), slog.String("resource", f.Resource.String()), slog.String("actor", rt.eventActor(r)))
	}
	writeJSON(w, http.StatusOK, resp)
}

// withAction returns the settings after f's action, or errAdvisorStale when
// the current value no longer matches what the finding saw.
func withAction(before advisorSettings, a resourceadvisor.Action) (advisorSettings, error) {
	after := before
	switch a.Type {
	case resourceadvisor.ActionSetMemoryLimit:
		if before.MemoryBytes != a.Current {
			return after, errAdvisorStale
		}
		after.MemoryBytes = a.Value
	case resourceadvisor.ActionSetCPULimit:
		if before.NanoCPUs != a.Current {
			return after, errAdvisorStale
		}
		after.NanoCPUs = a.Value
	case resourceadvisor.ActionEnableSleep:
		if before.SleepIdleMinutes != 0 {
			return after, errAdvisorStale
		}
		after.SleepIdleMinutes = int(a.Value)
	}
	return after, nil
}

func resourcesWith(r *store.ServiceResources, s advisorSettings) *store.ServiceResources {
	out := store.ServiceResources{}
	if r != nil {
		out = *r
	}
	out.MemoryBytes, out.NanoCPUs = s.MemoryBytes, s.NanoCPUs
	return &out
}

func (rt *Router) applyAdvisorToApp(r *http.Request, f resourceadvisor.Finding, resp *advisorApplyResponse, confirm bool) error {
	ctx := r.Context()
	existing, err := rt.apps.GetDesiredService(ctx, f.Resource.Name)
	if err != nil {
		return err
	}
	if existing.Resources != nil {
		resp.Before.MemoryBytes, resp.Before.NanoCPUs = existing.Resources.MemoryBytes, existing.Resources.NanoCPUs
	}
	if sl, err := rt.appSleep.GetAppSleep(ctx, f.Resource.Name); err == nil {
		resp.Before.SleepIdleMinutes = sl.IdleMinutes
	}
	if resp.After, err = withAction(resp.Before, *f.Action); err != nil || !confirm {
		return err
	}
	if f.Action.Type == resourceadvisor.ActionEnableSleep {
		if err := rt.appSleep.SetAppSleepIdle(ctx, f.Resource.Name, resp.After.SleepIdleMinutes, time.Now()); err != nil {
			return err
		}
		resp.Applied = true
		return nil
	}
	desired := *existing
	desired.Resources = resourcesWith(existing.Resources, resp.After)
	if err := rt.apps.SaveDesiredService(ctx, desired); err != nil {
		return err
	}
	resp.Applied = true
	for _, ev := range updateEvents(f.Resource.Name, *existing, desired) {
		rt.recordAppEvent(r, ev)
	}
	if saved, err := rt.apps.GetDesiredService(ctx, f.Resource.Name); err == nil {
		resp.AppliedLive = rt.applyResourcesLiveToReplicas(ctx, *saved)
	}
	return nil
}

func (rt *Router) applyAdvisorToDatabase(r *http.Request, f resourceadvisor.Finding, resp *advisorApplyResponse, confirm bool) error {
	ctx := r.Context()
	existing, err := rt.databases.GetDesiredDatabase(ctx, f.Resource.Name)
	if err != nil {
		return err
	}
	if existing.Resources != nil {
		resp.Before.MemoryBytes, resp.Before.NanoCPUs = existing.Resources.MemoryBytes, existing.Resources.NanoCPUs
	}
	if f.Action.Type == resourceadvisor.ActionEnableSleep {
		return errAdvisorStale
	}
	if resp.After, err = withAction(resp.Before, *f.Action); err != nil || !confirm {
		return err
	}
	desired := *existing
	desired.Resources = resourcesWith(existing.Resources, resp.After)
	if err := rt.databases.SaveDesiredDatabase(ctx, desired); err != nil {
		return err
	}
	resp.Applied = true
	if saved, err := rt.databases.GetDesiredDatabase(ctx, f.Resource.Name); err == nil {
		resp.AppliedLive = rt.applyResourcesLive(ctx, saved.NodeID, databaseContainerName(f.Resource.Name), saved.Resources)
	}
	return nil
}
