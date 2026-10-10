package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/GLINCKER/levelrail/internal/imagemove"
	"github.com/GLINCKER/levelrail/internal/store"
)

const imageMoveInspectTimeout = 5 * time.Second

type appImportImagesTransferRequest struct {
	SSH        string   `json:"ssh,omitempty"`
	Port       int      `json:"port,omitempty"`
	PrivateKey string   `json:"private_key,omitempty"`
	Passphrase string   `json:"passphrase,omitempty"`
	UseAgent   bool     `json:"use_agent,omitempty"`
	Items      []string `json:"items,omitempty"`
}

func imageMoveMaxBytes() int64 {
	if n, err := strconv.ParseInt(os.Getenv(envImageMoveMaxBytes), 10, 64); err == nil && n > 0 {
		return n
	}
	return defaultImageMoveMax
}

// hostBuiltImages lists the selected items whose image exists only on the
// source host. It is the allowlist: no other ref is ever sent to the source.
func hostBuiltImages(items []store.AppImportItem) []appImportImage {
	var out []appImportImage
	for _, it := range items {
		if !it.Selected || it.State == store.AppImportRolledBack {
			continue
		}
		e := decodeAppImportRecord(it.EntryJSON).Entry
		if !e.HostBuilt || e.Image == "" {
			continue
		}
		out = append(out, appImportImage{SourceID: it.SourceID, App: it.SourceName, Target: it.TargetName, Image: e.Image,
			SourceImageID: e.ImageID, State: imageMovePending})
	}
	sortImages(out)
	return out
}

func (rt *Router) imageNode(ctx context.Context, target string) string {
	if target == "" {
		return ""
	}
	svc, err := rt.apps.GetDesiredService(ctx, target)
	if err != nil || svc == nil {
		return ""
	}
	return svc.NodeID
}

// imageRuntime resolves the node runtime an image is loaded into.
func (rt *Router) imageRuntime(node string) (imagemove.Runtime, error) {
	if rt.execRuntime == nil {
		return nil, errors.New("this control plane has no node runtime to load images into")
	}
	r, err := rt.execRuntime(node)
	if err != nil {
		return nil, fmt.Errorf("reach the target node: %w", err)
	}
	ir, ok := r.(imagemove.Runtime)
	if !ok {
		return nil, errors.New("the target node cannot load images over its agent connection yet, place the app on the control plane node")
	}
	return ir, nil
}

// imagesView merges the planned images with the in-memory run. With
// inspect it also asks the target whether each image is already there,
// so a finished move is shown as verified after a restart.
func (rt *Router) imagesView(ctx context.Context, sessID string, items []store.AppImportItem, inspect bool) appImportImagesResource {
	running, source, held, mem := rt.appImportLive.moveSnapshot(sessID)
	view := appImportImagesResource{Running: running, Source: source, CredentialsHeld: held, Supported: rt.execRuntime != nil,
		MaxBytes: imageMoveMaxBytes(), Images: []appImportImage{}}
	for _, img := range hostBuiltImages(items) {
		if m, ok := mem[img.SourceID]; ok && m.Image == img.Image {
			m.App, m.Target = img.App, img.Target
			view.Images = append(view.Images, m)
			continue
		}
		if inspect && !running && img.SourceImageID != "" {
			img.Node = rt.imageNode(ctx, img.Target)
			if r, err := rt.imageRuntime(img.Node); err == nil {
				ictx, cancel := context.WithTimeout(ctx, imageMoveInspectTimeout)
				id, ierr := r.InspectImageID(ictx, img.Image)
				cancel()
				if ierr == nil && id == img.SourceImageID {
					img.State, img.Verified, img.LoadedImageID = imageMoveVerified, true, id
				}
			}
		}
		view.Images = append(view.Images, img)
	}
	return view
}

// handleAppImportImages handles GET .../sessions/{id}/images.
func (rt *Router) handleAppImportImages(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.imagesView(r.Context(), sess.ID, items, true))
}

// handleAppImportImagesStatus handles GET .../sessions/{id}/images/status:
// the in-memory progress only, cheap enough to poll.
func (rt *Router) handleAppImportImagesStatus(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.imagesView(r.Context(), sess.ID, items, false))
}

// handleCancelAppImportImages handles POST .../sessions/{id}/images/cancel.
func (rt *Router) handleCancelAppImportImages(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	if !rt.appImportLive.cancelMove(sess.ID) {
		writeError(w, http.StatusConflict, "no image move is running for this session")
		return
	}
	rt.logger.Info("api: app import: image move cancelled", slog.String("session_id", sess.ID))
	writeJSON(w, http.StatusAccepted, rt.imagesView(r.Context(), sess.ID, items, false))
}

func (rt *Router) resolveImageMoveCreds(sessID string, req appImportImagesTransferRequest) (imagemove.Target, imagemove.Credentials, error) {
	heldTarget, held := rt.appImportLive.heldCreds(sessID)
	var t imagemove.Target
	switch {
	case req.SSH != "":
		parsed, err := imagemove.ParseTarget(req.SSH, req.Port)
		if err != nil {
			return t, imagemove.Credentials{}, err
		}
		t = parsed
	case held != nil:
		t = heldTarget
	default:
		return t, imagemove.Credentials{}, errors.New("ssh is required: the source login as user@host")
	}
	c := imagemove.Credentials{PrivateKey: []byte(req.PrivateKey), Passphrase: []byte(req.Passphrase), UseAgent: req.UseAgent}
	if len(c.PrivateKey) == 0 && !c.UseAgent {
		if held == nil || heldTarget != t {
			return t, c, errors.New("supply a private key for the source login, or use the control plane's SSH agent")
		}
		c = *held
	}
	return t, c, nil
}

// handleTransferAppImportImages handles POST .../sessions/{id}/images/transfer.
// It streams each host-built image from the source over SSH into the
// target node and verifies the loaded image ID. It returns at once; poll
// images/status. Images already verified on the target are skipped.
func (rt *Router) handleTransferAppImportImages(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "this control plane has no node runtime to load images into")
		return
	}
	var req appImportImagesTransferRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAppImportBody)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	target, creds, err := rt.resolveImageMoveCreds(sess.ID, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	var queued []appImportImage
	for _, img := range hostBuiltImages(items) {
		if len(req.Items) > 0 && !stringIn(req.Items, img.SourceID) && !stringIn(req.Items, img.Target) && !stringIn(req.Items, img.App) {
			continue
		}
		img.Node = rt.imageNode(r.Context(), img.Target)
		queued = append(queued, img)
	}
	if len(queued) == 0 {
		writeError(w, http.StatusBadRequest, "no selected app has an image built on the source host")
		return
	}
	ctx, cancel := context.WithCancel(context.WithoutCancel(r.Context()))
	if !rt.appImportLive.startMove(sess.ID, target, creds, cancel, queued) {
		cancel()
		writeError(w, http.StatusConflict, "an image move is already running for this session")
		return
	}
	rt.setAppImportStep(r.Context(), sess, appImportStepImages)
	rt.logger.Info("api: app import: image move started", slog.String("session_id", sess.ID), slog.String("source", target.String()), slog.Int("images", len(queued)))
	go rt.runImageMove(ctx, cancel, sess.ID, target, creds, queued)
	writeJSON(w, http.StatusAccepted, rt.imagesView(r.Context(), sess.ID, items, false))
}

func (rt *Router) runImageMove(ctx context.Context, cancel context.CancelFunc, sessID string, target imagemove.Target, creds imagemove.Credentials, queued []appImportImage) {
	defer cancel()
	defer rt.appImportLive.finishMove(sessID)
	saver := rt.appImportLive.saverFor(target, creds, rt.appImportLive.hostKeyStore(rt.dataDir))
	maxBytes := imageMoveMaxBytes()
	timeout := envDuration(envImageMoveTimeout, defaultImageMoveTimeout)
	for _, img := range queued {
		if ctx.Err() != nil {
			return
		}
		rt.moveOneImage(ctx, sessID, saver, img, maxBytes, timeout)
	}
}

func (rt *Router) moveOneImage(ctx context.Context, sessID string, saver imagemove.Saver, img appImportImage, maxBytes int64, timeout time.Duration) {
	set := func(fn func(*appImportImage)) { rt.appImportLive.updateImage(sessID, img.SourceID, fn) }
	set(func(m *appImportImage) { m.State, m.Error, m.Bytes = imageMoveRunning, "", 0 })
	fail := func(err error) {
		set(func(m *appImportImage) { m.State, m.Error = imageMoveFailed, err.Error() })
		rt.logger.Warn("api: app import: image move failed", slog.String("session_id", sessID), slog.String("source_id", img.SourceID), slog.String("image", img.Image), slog.String("error", err.Error()))
	}
	if img.SourceImageID != "" {
		if err := imagemove.ValidateImageID(img.SourceImageID); err != nil {
			fail(err)
			return
		}
	}
	dst, err := rt.imageRuntime(img.Node)
	if err != nil {
		fail(err)
		return
	}
	ictx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	res, err := imagemove.Transfer(ictx, saver, dst, imagemove.Request{Ref: img.Image, WantID: img.SourceImageID, MaxBytes: maxBytes,
		Progress: func(n int64) { set(func(m *appImportImage) { m.Bytes = n }) }})
	if err != nil {
		if ctx.Err() != nil {
			set(func(m *appImportImage) {
				m.State, m.Error, m.Bytes = imageMoveCancelled, "the move was cancelled", res.Bytes
			})
			return
		}
		fail(err)
		return
	}
	set(func(m *appImportImage) {
		m.Bytes, m.LoadedImageID, m.Verified = res.Bytes, res.LoadedID, res.Verified
		m.State = imageMoveVerified
		if !res.Verified {
			m.State = imageMoveLoaded
		}
	})
	rt.logger.Info("api: app import: image moved", slog.String("session_id", sessID), slog.String("source_id", img.SourceID), slog.String("image", img.Image),
		slog.Int64("bytes", res.Bytes), slog.Bool("verified", res.Verified), slog.Bool("already_present", res.AlreadyPresent))
}
