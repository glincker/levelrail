package api

import (
	"context"
	"log/slog"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/objectstore"
	"github.com/GLINCKER/levelrail/internal/store"
)

// Warning codes returned by the asset probe. They never block anything: ADR 044 open question 3 resolved to warn only.
const (
	warnExposurePrivateButPublic = "private_but_publicly_readable"
	warnExposurePublicButClosed  = "public_but_not_readable"
	warnExposureUnchecked        = "exposure_unchecked"
	warnCORSMissing              = "cors_missing"
	warnCORSUnverified           = "cors_unverified"
	warnNoOrigin                 = "cors_origin_unknown"
)

type assetProbeWarning struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Fix     string `json:"fix,omitempty"`
}

type assetProbeRequest struct {
	// Origin is the dashboard origin browsers upload from; it defaults to the configured dashboard URL.
	Origin string `json:"origin"`
	// FixCORS adds the upload rule at the provider when the key may write bucket CORS.
	FixCORS bool `json:"fix_cors"`
}

type assetProbeResource struct {
	objectstore.AssetProbeResult
	Warnings []assetProbeWarning `json:"warnings"`
}

// handleProbeAssetBucket handles POST /api/v1/assets/buckets/{id}/probe: the put/get/delete round trip, the browser
// upload CORS step and, with a public base URL, an anonymous HEAD that checks the bucket matches its visibility.
func (rt *Router) handleProbeAssetBucket(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	var req assetProbeRequest
	if r.ContentLength != 0 && !decodeReq(w, r, &req) {
		return
	}
	origin, msg := rt.probeOrigin(r.Context(), req.Origin)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.assetClient(ctx, w, d, ab)
	if !ok {
		return
	}
	res := objectstore.ProbeAssets(ctx, client, objectstore.AssetProbeOptions{
		Origin: origin, ApplyCORS: req.FixCORS, PublicBaseURL: ab.Settings.PublicBaseURL, KeyPrefix: ab.Bucket.Prefix,
		Timeout: envDuration(envAssetsExposureTimeout, defaultAssetsExposureTO),
	})
	out := assetProbeResource{AssetProbeResult: res, Warnings: []assetProbeWarning{}}
	if res.OK {
		out.Warnings = probeWarnings(ab.Settings, origin, res)
	}
	if res.CORS != nil && res.CORS.Status == objectstore.CORSApplied {
		rt.auditAsset(r, "asset_bucket.cors_applied", ab.Bucket.ID, map[string]any{"origin": origin})
	}
	if !res.OK {
		rt.logger.Info("api: asset bucket probe failed", slog.String("bucket_id", ab.Bucket.ID), slog.String("reason", res.Reason))
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) probeOrigin(ctx context.Context, given string) (string, string) {
	given = strings.TrimRight(strings.TrimSpace(given), "/")
	if given == "" {
		dash, err := rt.ingressSettings.GetDashboardURL(ctx)
		if err != nil || dash == "" {
			return "", ""
		}
		given = strings.TrimRight(dash, "/")
	}
	if err := objectstore.ValidateEndpoint(given); err != nil {
		return "", "origin: " + err.Error()
	}
	return given, ""
}

func probeWarnings(s store.AssetBucketSettings, origin string, res objectstore.AssetProbeResult) []assetProbeWarning {
	out := []assetProbeWarning{}
	if origin == "" {
		out = append(out, assetProbeWarning{Code: warnNoOrigin, Message: "no dashboard URL is configured, so the browser upload CORS rule was not checked. Pass origin to check it."})
	}
	if c := res.CORS; c != nil && c.Status != objectstore.CORSOK && c.Status != objectstore.CORSApplied {
		code := warnCORSMissing
		if c.Status == objectstore.CORSUnverified {
			code = warnCORSUnverified
		}
		out = append(out, assetProbeWarning{Code: code, Message: c.Message, Fix: "fix_cors"})
	}
	e := res.Exposure
	switch {
	case e == nil && s.Visibility == store.AssetVisibilityPublic:
		out = append(out, assetProbeWarning{Code: warnExposureUnchecked, Message: "the bucket is public but has no public_base_url, so anonymous access was not checked"})
	case e == nil:
	case e.Error != "":
		out = append(out, assetProbeWarning{Code: warnExposureUnchecked, Message: e.Error})
	case s.Visibility == store.AssetVisibilityPrivate && e.Public:
		out = append(out, assetProbeWarning{
			Code:    warnExposurePrivateButPublic,
			Message: "this bucket is private here, but an anonymous request to " + e.URL + " was answered. Turn off public access at the provider, or make the bucket public here to match.",
			Fix:     "close_provider_access_or_make_public",
		})
	case s.Visibility == store.AssetVisibilityPublic && !e.Public:
		out = append(out, assetProbeWarning{
			Code:    warnExposurePublicButClosed,
			Message: "this bucket is public here, but an anonymous request to " + e.URL + " got status " + strconv.Itoa(e.Status) + ". Allow public reads at the provider or check the public base URL.",
			Fix:     "open_provider_access",
		})
	}
	return out
}

type assetUsageResource struct {
	Bytes           int64                    `json:"bytes"`
	Objects         int64                    `json:"objects"`
	AtLeast         bool                     `json:"at_least"`
	QuotaBytes      int64                    `json:"quota_bytes"`
	ServingMode     string                   `json:"serving_mode"`
	TrafficMeasured bool                     `json:"traffic_measured"`
	Cost            objectstore.CostEstimate `json:"cost"`
}

// handleAssetUsage handles GET /api/v1/assets/buckets/{id}/usage?egress_gb=. Size comes from a bounded listing; egress is
// the number the caller supplies because Direct serving does not pass through levelrail.
func (rt *Router) handleAssetUsage(w http.ResponseWriter, r *http.Request) {
	d, ab, ok := rt.loadAssetBucket(w, r)
	if !ok {
		return
	}
	egress := 0.0
	if v := r.URL.Query().Get("egress_gb"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			writeError(w, http.StatusBadRequest, "egress_gb must be a non-negative number")
			return
		}
		egress = f
	}
	ctx, cancel := context.WithTimeout(r.Context(), bucketOpTimeout)
	defer cancel()
	client, ok := rt.assetClient(ctx, w, d, ab)
	if !ok {
		return
	}
	bytes, objects, truncated, err := client.UsageBounded(ctx, ab.Bucket.Prefix, envPositiveInt64(envAssetsUsageMaxObjects, defaultAssetsUsageMaxObjs))
	if err != nil {
		rt.internalError(w, "api: asset usage failed", err, slog.String("bucket_id", ab.Bucket.ID))
		return
	}
	writeJSON(w, http.StatusOK, assetUsageResource{
		Bytes: bytes, Objects: objects, AtLeast: truncated, QuotaBytes: ab.Bucket.QuotaBytes, ServingMode: ab.Settings.ServingMode,
		Cost: objectstore.EstimateAssetCost(rt.destinationPreset(ctx, d, ab.Bucket.TargetID), bytes, egress, os.LookupEnv),
	})
}
