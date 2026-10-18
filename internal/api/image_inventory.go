package api

import (
	"context"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/imagelife"
)

const (
	envImageKeepPerApp = "APP_IMAGE_KEEP_PER_APP"
	envImageMinAgeHrs  = "APP_IMAGE_MIN_AGE_HOURS"

	defaultImageKeepPerApp = 3
	defaultImageMinAgeHrs  = 24

	// ImageVerifyOff skips signature checks on promotion.
	ImageVerifyOff = "off"
	// ImageVerifyEnforce refuses a promotion whose image is unsigned or signed by another key.
	ImageVerifyEnforce = "enforce"
)

// ImageInventoryResolver returns the image inventory of one node ("" is the
// control plane's own node).
type ImageInventoryResolver func(nodeID string) (docker.ImageInventory, error)

// WithImageInventory enables the /api/v1/images routes.
func WithImageInventory(r ImageInventoryResolver) Option {
	return func(rt *Router) { rt.imageInventory = r }
}

// WithImageVerify sets the public key signatures are checked against and the
// mode promotions run in. A nil key with ImageVerifyEnforce refuses every
// promotion, so callers only pass enforce together with a key.
func WithImageVerify(pub *ecdsa.PublicKey, mode string) Option {
	return func(rt *Router) {
		rt.imageVerifyKey = pub
		rt.imageVerifyMode = mode
		rt.imageVerifier = imagelife.NewVerifier()
	}
}

type imageRetention struct {
	KeepPerApp int `json:"keep_per_app"`
	MinAgeHrs  int `json:"min_age_hours"`
}

type imageTotals struct {
	Count          int   `json:"count"`
	SizeBytes      int64 `json:"size_bytes"`
	ReclaimBytes   int64 `json:"reclaimable_bytes"`
	ProtectedCount int   `json:"protected_count"`
}

type imageListResponse struct {
	Node       string               `json:"node"`
	Images     []imagelife.Entry    `json:"images"`
	Totals     imageTotals          `json:"totals"`
	Retention  imageRetention       `json:"retention"`
	Signatures imageSignatureStatus `json:"signatures"`
}

type imageSignatureStatus struct {
	Mode       string `json:"mode"`
	Configured bool   `json:"configured"`
}

func (rt *Router) imageRetentionDefaults() imageRetention {
	return imageRetention{
		KeepPerApp: envIntDefault(envImageKeepPerApp, defaultImageKeepPerApp),
		MinAgeHrs:  envIntDefault(envImageMinAgeHrs, defaultImageMinAgeHrs),
	}
}

func queryIntOr(r *http.Request, key string, def int) int {
	if n, err := strconv.Atoi(r.URL.Query().Get(key)); err == nil && n >= 0 {
		return n
	}
	return def
}

func (rt *Router) imageSignatureMode() string {
	if rt.imageVerifyMode == ImageVerifyEnforce && rt.imageVerifyKey != nil {
		return ImageVerifyEnforce
	}
	return ImageVerifyOff
}

func (rt *Router) loadImageApps(ctx context.Context, nodeID string) ([]imagelife.AppState, error) {
	services, err := rt.apps.ListDesiredServices(ctx)
	if err != nil {
		return nil, fmt.Errorf("list apps: %w", err)
	}
	out := make([]imagelife.AppState, 0, len(services))
	for _, svc := range services {
		if svc.NodeID != nodeID {
			continue
		}
		attempts, err := rt.deployAttempts.ListDeployAttempts(ctx, svc.Name)
		if err != nil {
			return nil, fmt.Errorf("list deploys of %s: %w", svc.Name, err)
		}
		state := imagelife.AppState{Name: svc.Name, LiveImage: svc.Image, LiveImageID: svc.LocalImageID()}
		for _, a := range attempts {
			state.Attempts = append(state.Attempts, imagelife.Attempt{
				ID: a.ID, Image: a.Image, Digest: a.ImageDigest, Source: a.Source, CommitSHA: a.CommitSHA,
				Branch: a.Branch, Status: a.Status, StartedAt: a.StartedAt,
			})
		}
		if rt.deployGate != nil {
			pins, err := rt.deployGate.ListRollbackPins(ctx, svc.Name)
			if err != nil {
				return nil, fmt.Errorf("list rollback pins of %s: %w", svc.Name, err)
			}
			for _, p := range pins {
				state.Pins = append(state.Pins, imagelife.Pin{DeployID: p.DeployAttemptID, Image: p.Image, ImageID: p.ImageID, PinRef: p.PinRef})
			}
		}
		out = append(out, state)
	}
	return out, nil
}

func (rt *Router) buildImageEntries(ctx context.Context, inv docker.ImageInventory, nodeID string, ret imageRetention) ([]imagelife.Entry, error) {
	records, err := inv.ListAllImages(ctx)
	if err != nil {
		return nil, fmt.Errorf("list images: %w", err)
	}
	apps, err := rt.loadImageApps(ctx, nodeID)
	if err != nil {
		return nil, err
	}
	return imagelife.Build(records, apps, ret.KeepPerApp, time.Duration(ret.MinAgeHrs)*time.Hour, time.Now()), nil
}

func (rt *Router) imageInventoryFor(w http.ResponseWriter, nodeID string) (docker.ImageInventory, bool) {
	if rt.imageInventory == nil {
		writeError(w, http.StatusNotImplemented, "image inventory is not configured on this control plane")
		return nil, false
	}
	inv, err := rt.imageInventory(nodeID)
	if err != nil {
		writeError(w, http.StatusBadGateway, "this node cannot be reached: "+err.Error())
		return nil, false
	}
	return inv, true
}

// handleListImageInventory handles GET /api/v1/images: every image on a node with
// its size, owners and retention status.
func (rt *Router) handleListImageInventory(w http.ResponseWriter, r *http.Request) {
	nodeID := r.URL.Query().Get("node")
	inv, ok := rt.imageInventoryFor(w, nodeID)
	if !ok {
		return
	}
	ret := rt.imageRetentionDefaults()
	ret.KeepPerApp = queryIntOr(r, "keep_per_app", ret.KeepPerApp)
	entries, err := rt.buildImageEntries(r.Context(), inv, nodeID, ret)
	if err != nil {
		rt.internalError(w, "api: list images failed", err)
		return
	}
	plan := imagelife.PlanGC(entries)
	totals := imageTotals{Count: len(entries), ReclaimBytes: plan.ReclaimBytes, ProtectedCount: plan.KeptCount}
	for _, e := range entries {
		totals.SizeBytes += e.SizeBytes
	}
	writeJSON(w, http.StatusOK, imageListResponse{
		Node: nodeID, Images: entries, Totals: totals, Retention: ret,
		Signatures: imageSignatureStatus{Mode: rt.imageSignatureMode(), Configured: rt.imageVerifyKey != nil},
	})
}

type imageLayerResource struct {
	ID        string    `json:"id"`
	CreatedBy string    `json:"created_by"`
	SizeBytes int64     `json:"size_bytes"`
	CreatedAt time.Time `json:"created_at"`
}

type imageLayersResponse struct {
	Layers    []imageLayerResource `json:"layers"`
	SizeBytes int64                `json:"size_bytes"`
}

// handleImageLayerBreakdown handles GET /api/v1/images/layers?id=: the layer size
// breakdown of one image.
func (rt *Router) handleImageLayerBreakdown(w http.ResponseWriter, r *http.Request) {
	id := r.URL.Query().Get("id")
	if id == "" {
		writeError(w, http.StatusBadRequest, "id is required")
		return
	}
	inv, ok := rt.imageInventoryFor(w, r.URL.Query().Get("node"))
	if !ok {
		return
	}
	layers, err := inv.ImageLayerHistory(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: image layers failed", err, slog.String("image", id))
		return
	}
	if layers == nil {
		writeError(w, http.StatusNotFound, "image not found on this node")
		return
	}
	out := imageLayersResponse{Layers: make([]imageLayerResource, 0, len(layers))}
	for _, l := range layers {
		out.Layers = append(out.Layers, imageLayerResource{ID: l.ID, CreatedBy: l.CreatedBy, SizeBytes: l.SizeBytes, CreatedAt: l.CreatedAt})
		out.SizeBytes += l.SizeBytes
	}
	writeJSON(w, http.StatusOK, out)
}

type imageGCRequest struct {
	Node       string `json:"node"`
	KeepPerApp *int   `json:"keep_per_app"`
	MinAgeHrs  *int   `json:"min_age_hours"`
	DryRun     bool   `json:"dry_run"`
}

type imageGCFailure struct {
	ID    string `json:"id"`
	Error string `json:"error"`
}

type imageGCResponse struct {
	DryRun         bool              `json:"dry_run"`
	Candidates     []imagelife.Entry `json:"candidates"`
	Removed        []string          `json:"removed"`
	Failed         []imageGCFailure  `json:"failed"`
	ReclaimedBytes int64             `json:"reclaimed_bytes"`
	Retention      imageRetention    `json:"retention"`
}

// handleImageGC handles POST /api/v1/images/gc: removes images no app can
// still need. Live images, rollback pins, the newest successful deploys of
// every app and images used by a container are never candidates.
func (rt *Router) handleImageGC(w http.ResponseWriter, r *http.Request) {
	var req imageGCRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	inv, ok := rt.imageInventoryFor(w, req.Node)
	if !ok {
		return
	}
	ret := rt.imageRetentionDefaults()
	if req.KeepPerApp != nil && *req.KeepPerApp >= 0 {
		ret.KeepPerApp = *req.KeepPerApp
	}
	if req.MinAgeHrs != nil && *req.MinAgeHrs >= 0 {
		ret.MinAgeHrs = *req.MinAgeHrs
	}
	entries, err := rt.buildImageEntries(r.Context(), inv, req.Node, ret)
	if err != nil {
		rt.internalError(w, "api: image gc plan failed", err)
		return
	}
	plan := imagelife.PlanGC(entries)
	resp := imageGCResponse{DryRun: req.DryRun, Candidates: plan.Remove, Removed: []string{}, Failed: []imageGCFailure{}, Retention: ret}
	if !req.DryRun {
		for _, e := range plan.Remove {
			if err := removeImageEntry(r.Context(), inv, e); err != nil {
				resp.Failed = append(resp.Failed, imageGCFailure{ID: e.ID, Error: err.Error()})
				continue
			}
			resp.Removed = append(resp.Removed, e.ID)
			resp.ReclaimedBytes += e.SizeBytes - e.SharedBytes
		}
		rt.logger.Info("api: image gc finished", slog.Int("removed", len(resp.Removed)), slog.Int("failed", len(resp.Failed)))
	}
	writeJSON(w, http.StatusOK, resp)
}

func removeImageEntry(ctx context.Context, inv docker.ImageInventory, e imagelife.Entry) error {
	for _, ref := range imagelife.RemoveRefs(e) {
		if err := inv.RemoveImage(ctx, ref); err != nil {
			return fmt.Errorf("remove %s: %w", ref, err)
		}
	}
	return nil
}

type imageVerifyRequest struct {
	Node string `json:"node"`
	ID   string `json:"id"`
	Ref  string `json:"ref"`
}

type imageVerifyResponse struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

const (
	verifyStatusVerified      = "verified"
	verifyStatusUnsigned      = "unsigned"
	verifyStatusFailed        = "failed"
	verifyStatusNotConfigured = "not_configured"
	verifyStatusNoDigest      = "no_registry_digest"
)

// handleImageVerify handles POST /api/v1/images/verify: checks the cosign
// signature of one image against the configured public key.
func (rt *Router) handleImageVerify(w http.ResponseWriter, r *http.Request) {
	var req imageVerifyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || (req.ID == "" && req.Ref == "") {
		writeError(w, http.StatusBadRequest, "id or ref is required")
		return
	}
	if rt.imageVerifyKey == nil {
		writeJSON(w, http.StatusOK, imageVerifyResponse{Status: verifyStatusNotConfigured, Message: "no signature public key is configured"})
		return
	}
	ref := req.Ref
	if ref == "" {
		inv, ok := rt.imageInventoryFor(w, req.Node)
		if !ok {
			return
		}
		records, err := inv.ListAllImages(r.Context())
		if err != nil {
			rt.internalError(w, "api: image verify: list failed", err)
			return
		}
		for _, rec := range records {
			if rec.ID == req.ID && len(rec.RepoDigests) > 0 {
				ref = rec.RepoDigests[0]
			}
		}
		if ref == "" {
			writeJSON(w, http.StatusOK, imageVerifyResponse{Status: verifyStatusNoDigest, Message: "this image has no registry digest to verify, it was built locally"})
			return
		}
	}
	writeJSON(w, http.StatusOK, rt.verifyImageRef(r.Context(), ref))
}

func (rt *Router) verifyImageRef(ctx context.Context, ref string) imageVerifyResponse {
	err := rt.imageVerifier.Verify(ctx, ref, rt.imageVerifyKey, imagelife.Credentials{})
	switch {
	case err == nil:
		return imageVerifyResponse{Status: verifyStatusVerified, Message: "signature matches the configured public key"}
	case errors.Is(err, imagelife.ErrNoSignature):
		return imageVerifyResponse{Status: verifyStatusUnsigned, Message: err.Error()}
	default:
		return imageVerifyResponse{Status: verifyStatusFailed, Message: err.Error()}
	}
}

// enforceImageSignature returns a non-zero HTTP status when enforcement is on
// and image is not a digest-pinned, correctly signed reference.
func (rt *Router) enforceImageSignature(ctx context.Context, image string) (int, string) {
	if rt.imageSignatureMode() != ImageVerifyEnforce {
		return 0, ""
	}
	if docker.ImageDigestOf(image) == "" {
		return http.StatusConflict, "signature enforcement is on and this image is not pinned to a registry digest, so it cannot be verified"
	}
	if res := rt.verifyImageRef(ctx, image); res.Status != verifyStatusVerified {
		return http.StatusConflict, "signature enforcement is on and the image failed verification: " + res.Message
	}
	return 0, ""
}

// ImageVerifyConfigFromEnv loads the signature public key from the file named
// by keyFile. An empty keyFile disables verification.
func ImageVerifyConfigFromEnv(keyFile string) (*ecdsa.PublicKey, error) {
	if keyFile == "" {
		return nil, nil
	}
	data, err := os.ReadFile(keyFile) //nolint:gosec // operator-supplied path from configuration
	if err != nil {
		return nil, fmt.Errorf("read image verify key: %w", err)
	}
	pub, err := imagelife.ParsePublicKey(data)
	if err != nil {
		return nil, fmt.Errorf("image verify key: %w", err)
	}
	return pub, nil
}
