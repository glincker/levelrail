package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/GLINCKER/levelrail/internal/ingress"
	"github.com/GLINCKER/levelrail/internal/splittest"
	"github.com/GLINCKER/levelrail/internal/store"
)

type splitVariantResource struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	Kind     string  `json:"kind"`
	Ref      string  `json:"ref,omitempty"`
	Weight   int     `json:"weight"`
	Status   string  `json:"status"`
	Host     string  `json:"host,omitempty"`
	Requests uint64  `json:"requests"`
	Share    float64 `json:"share"`
	QALink   string  `json:"qa_link,omitempty"`
}

type splitBucketResource struct {
	Bucket time.Time         `json:"bucket"`
	Counts map[string]uint64 `json:"counts"`
}

type splitTestResource struct {
	Active        bool                   `json:"active"`
	Supported     bool                   `json:"supported"`
	Unsupported   string                 `json:"unsupported_reason,omitempty"`
	TestID        string                 `json:"test_id,omitempty"`
	Status        string                 `json:"status,omitempty"`
	CookieName    string                 `json:"cookie_name,omitempty"`
	CookieTTLSecs int                    `json:"cookie_ttl_secs,omitempty"`
	OverrideParam string                 `json:"override_param,omitempty"`
	CreatedAt     *time.Time             `json:"created_at,omitempty"`
	EndedAt       *time.Time             `json:"ended_at,omitempty"`
	EndReason     string                 `json:"end_reason,omitempty"`
	TotalRequests uint64                 `json:"total_requests"`
	Variants      []splitVariantResource `json:"variants"`
	Hourly        []splitBucketResource  `json:"hourly,omitempty"`
}

// Hourly buckets older than this are left out of the response.
const splitHourlyWindow = 48 * time.Hour

// Get returns the running test, or the latest ended one, with live stats.
func (s *splitService) Get(ctx context.Context, app string) (splitTestResource, error) {
	res := splitTestResource{Supported: true, Variants: []splitVariantResource{}}
	if _, err := s.appDomains(ctx, app); err != nil {
		var se splitError
		if errors.As(err, &se) && se.status == http.StatusBadRequest {
			res.Supported, res.Unsupported = false, se.msg
			return res, nil
		}
		return splitTestResource{}, err
	}
	s.Flush(ctx)
	t, ok, err := s.st.GetLatestSplitTest(ctx, app)
	if err != nil || !ok {
		return res, err
	}
	stats, err := s.st.ListSplitStats(ctx, t.ID)
	if err != nil {
		return splitTestResource{}, err
	}
	return s.fill(ctx, res, t, stats), nil
}

func (s *splitService) fill(ctx context.Context, res splitTestResource, t store.SplitTest, stats []store.SplitStat) splitTestResource {
	res.Active = t.Status == splittest.StatusRunning
	res.TestID, res.Status, res.CookieName, res.CookieTTLSecs = t.ID, t.Status, t.CookieName, t.CookieTTLSecs
	res.OverrideParam, res.EndReason = s.defaults.OverrideParam, t.EndReason
	created := t.CreatedAt
	res.CreatedAt = &created
	if !t.EndedAt.IsZero() {
		ended := t.EndedAt
		res.EndedAt = &ended
	}
	totals := map[string]uint64{}
	buckets := map[time.Time]map[string]uint64{}
	cutoff := s.now().Add(-splitHourlyWindow)
	for _, st := range stats {
		totals[st.VariantID] += st.Requests
		res.TotalRequests += st.Requests
		if st.Bucket.After(cutoff) {
			if buckets[st.Bucket] == nil {
				buckets[st.Bucket] = map[string]uint64{}
			}
			buckets[st.Bucket][st.VariantID] += st.Requests
		}
	}
	for _, st := range stats {
		if b, ok := buckets[st.Bucket]; ok {
			res.Hourly = append(res.Hourly, splitBucketResource{Bucket: st.Bucket, Counts: b})
			delete(buckets, st.Bucket)
		}
	}
	primary := ""
	if domains, err := s.appDomains(ctx, t.AppName); err == nil && len(domains) > 0 {
		primary = domains[0]
	}
	for _, v := range t.Variants {
		vr := splitVariantResource{ID: v.ID, Label: v.Label, Kind: v.Kind, Ref: v.Ref, Weight: v.Weight, Requests: totals[v.ID], Status: "live"}
		if res.TotalRequests > 0 {
			vr.Share = float64(vr.Requests) / float64(res.TotalRequests)
		}
		if v.Kind != splittest.KindProduction {
			vr.Host, _ = s.splitHostFor(ctx, t.AppName, v)
			vr.Status = "ready"
			if vr.Host == "" {
				vr.Status = "unavailable"
			}
		}
		if primary != "" && res.Active {
			q := url.Values{s.defaults.OverrideParam: {v.ID}}
			vr.QALink = s.publicURL(ctx, primary) + "/?" + q.Encode()
		}
		res.Variants = append(res.Variants, vr)
	}
	return res
}

// newSplitService wires the service to this router's stores and deploy path.
func (rt *Router) newSplitService() *splitService {
	if rt.splitStore == nil {
		return nil
	}
	return &splitService{
		st: rt.splitStore, reg: ingress.DefaultSplits(), defaults: splittest.DefaultsFromEnv(splittest.OSEnv),
		logger: rt.logger, now: time.Now,
		appDomains: func(ctx context.Context, app string) ([]string, error) {
			svc, err := rt.apps.GetDesiredService(ctx, app)
			if errors.Is(err, store.ErrServiceNotFound) {
				return nil, splitErr(http.StatusBadRequest, "split testing is available for container apps only, static sites are not supported yet")
			}
			if err != nil {
				return nil, err
			}
			return svc.Domains, nil
		},
		serviceImage: func(ctx context.Context, service string) (string, error) {
			svc, err := rt.apps.GetDesiredService(ctx, service)
			if err != nil {
				return "", err
			}
			return svc.Image, nil
		},
		lockReason: func(ctx context.Context, app string) (string, bool) {
			if rt.publishLocks == nil {
				return "", false
			}
			l, err := rt.publishLocks.GetPublishLock(ctx, app)
			if err != nil || l == nil {
				return "", false
			}
			return l.Reason, true
		},
		deployImage: rt.splitDeployImage,
		publicURL:   rt.publicHTTPSURL,
		nudge:       rt.nudgeReconciler,
	}
}

// splitDeployImage deploys image to app like a canary promote: the normal
// image path with its readiness gate, freeze and protection checks. The
// built-from-source refusal is skipped because the variant image is already built.
func (rt *Router) splitDeployImage(ctx context.Context, app, image string) error {
	svc, err := rt.apps.GetDesiredService(ctx, app)
	if err != nil {
		return err
	}
	if svc.Image == image {
		return nil
	}
	check := *svc
	check.ImageIDRef = ""
	if why := rt.imageUpdateSkipReason(ctx, check); why != "" {
		return splitErr(http.StatusConflict, "%s", why)
	}
	trigger := rt.imageTrigger(ctx, *svc, true, "split test promote")
	trigger.Source = store.DeployAttemptSourceImage
	trigger.Ordered, trigger.Sequencer = nil, nil
	_, err = trigger.Deploy(ctx, *svc, image)
	return err
}

// StartSplitTesting restores running tests into ingress and persists request
// counts until ctx ends.
func (rt *Router) StartSplitTesting(ctx context.Context) {
	svc := rt.newSplitService()
	if svc == nil {
		return
	}
	if svc.Sync(ctx) {
		svc.nudge()
	}
	go func() {
		t := time.NewTicker(svc.defaults.FlushInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				svc.Flush(context.WithoutCancel(ctx))
				return
			case <-t.C:
				svc.Flush(ctx)
				if svc.Sync(ctx) {
					svc.nudge()
				}
			}
		}
	}()
}

func (rt *Router) splitServiceOrNotFound(w http.ResponseWriter) *splitService {
	svc := rt.newSplitService()
	if svc == nil {
		writeError(w, http.StatusNotFound, "split testing is not available")
	}
	return svc
}

func (rt *Router) writeSplit(w http.ResponseWriter, res splitTestResource, err error, op, app string) {
	var se splitError
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, res)
	case errors.As(err, &se):
		writeError(w, se.status, se.msg)
	default:
		rt.internalError(w, "api: split test "+op, err, slog.String("name", app))
	}
}

func (rt *Router) handleGetSplit(w http.ResponseWriter, r *http.Request) {
	if svc := rt.splitServiceOrNotFound(w); svc != nil {
		name := r.PathValue("name")
		res, err := svc.Get(r.Context(), name)
		rt.writeSplit(w, res, err, "get", name)
	}
}

func (rt *Router) handleStartSplit(w http.ResponseWriter, r *http.Request) {
	svc := rt.splitServiceOrNotFound(w)
	if svc == nil {
		return
	}
	var req splitStartRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	name := r.PathValue("name")
	res, err := svc.Start(r.Context(), name, req)
	rt.writeSplit(w, res, err, "start", name)
}

func (rt *Router) handleSetSplitWeights(w http.ResponseWriter, r *http.Request) {
	svc := rt.splitServiceOrNotFound(w)
	if svc == nil {
		return
	}
	var req splitWeightsRequest
	if err := decodeJSONBody(r, &req); err != nil || len(req.Weights) == 0 {
		writeError(w, http.StatusBadRequest, "weights is required")
		return
	}
	name := r.PathValue("name")
	res, err := svc.SetWeights(r.Context(), name, req)
	rt.writeSplit(w, res, err, "weights", name)
}

func (rt *Router) handleStopSplit(w http.ResponseWriter, r *http.Request) {
	if svc := rt.splitServiceOrNotFound(w); svc != nil {
		name := r.PathValue("name")
		res, err := svc.Stop(r.Context(), name, "stopped")
		rt.writeSplit(w, res, err, "stop", name)
	}
}

func (rt *Router) handlePromoteSplit(w http.ResponseWriter, r *http.Request) {
	svc := rt.splitServiceOrNotFound(w)
	if svc == nil {
		return
	}
	var req splitPromoteRequest
	if err := decodeJSONBody(r, &req); err != nil || req.Variant == "" {
		writeError(w, http.StatusBadRequest, "variant is required")
		return
	}
	name := r.PathValue("name")
	res, err := svc.Promote(r.Context(), name, req.Variant)
	rt.writeSplit(w, res, err, "promote", name)
}
