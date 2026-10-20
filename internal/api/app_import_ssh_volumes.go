package api

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/dockerssh"
	"github.com/GLINCKER/levelrail/internal/platformimport"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envVolumeCopyMaxBytes = "APP_MIGRATE_VOLUME_MAX_BYTES"
	envVolumeCopyMaxFiles = "APP_MIGRATE_VOLUME_MAX_FILES"
	envVolumeCopyTimeout  = "APP_MIGRATE_VOLUME_TIMEOUT"
	defaultVolumeMaxBytes = int64(10) << 30
	defaultVolumeMaxFiles = 500000
	defaultVolumeTimeout  = 4 * time.Hour
)

// Volume copy states.
const (
	volumeCopyPending = "pending"
	volumeCopyRunning = "running"
	volumeCopyDone    = "copied"
	volumeCopyFailed  = "failed"
	volumeCopySkipped = "skipped"
)

const rsyncFallback = "use the rsync command from the Volumes step"

type volumeCopyResult struct {
	SourceID      string   `json:"source_id"`
	App           string   `json:"app"`
	Volume        string   `json:"volume"`
	ContainerPath string   `json:"container_path"`
	State         string   `json:"state"`
	Files         int      `json:"files"`
	Bytes         int64    `json:"bytes"`
	Skipped       int      `json:"skipped_entries"`
	Warnings      []string `json:"warnings,omitempty"`
	Error         string   `json:"error,omitempty"`
	Fallback      string   `json:"fallback,omitempty"`
	index         int
}

type volumeCopyRun struct {
	running bool
	cancel  context.CancelFunc
	results []*volumeCopyResult
}

type volumeCopyView struct {
	Running  bool               `json:"running"`
	MaxBytes int64              `json:"max_bytes"`
	MaxFiles int                `json:"max_files"`
	Results  []volumeCopyResult `json:"results"`
}

type volumeMounter interface {
	VolumeMountpoint(ctx context.Context, name string) (string, error)
}

type volumeCopyJob struct {
	item      store.AppImportItem
	vol       store.AppImportVolume
	index     int
	container string
	result    *volumeCopyResult
}

func volumeCopyLimits() dockerssh.Limits {
	return dockerssh.Limits{
		MaxBytes: envInt64Positive(envVolumeCopyMaxBytes, defaultVolumeMaxBytes),
		MaxFiles: envIntDefault(envVolumeCopyMaxFiles, defaultVolumeMaxFiles),
	}
}

func (s *appImportState) volumeView(id string) volumeCopyView {
	s.mu.Lock()
	defer s.mu.Unlock()
	lim := volumeCopyLimits()
	v := volumeCopyView{MaxBytes: lim.MaxBytes, MaxFiles: lim.MaxFiles, Results: []volumeCopyResult{}}
	if run := s.volCopies[id]; run != nil {
		v.Running = run.running
		for _, r := range run.results {
			v.Results = append(v.Results, *r)
		}
	}
	return v
}

func (s *appImportState) startVolumeCopy(id string, cancel context.CancelFunc, results []*volumeCopyResult) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.volCopies == nil {
		s.volCopies = map[string]*volumeCopyRun{}
	}
	if run := s.volCopies[id]; run != nil && run.running {
		return false
	}
	s.volCopies[id] = &volumeCopyRun{running: true, cancel: cancel, results: results}
	return true
}

func (s *appImportState) forgetVolumeCopy(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run := s.volCopies[id]; run != nil {
		if run.cancel != nil {
			run.cancel()
		}
		delete(s.volCopies, id)
	}
}

func (s *appImportState) updateVolume(r *volumeCopyResult, fn func(*volumeCopyResult)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(r)
}

func (s *appImportState) finishVolumeCopy(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if run := s.volCopies[id]; run != nil {
		run.running, run.cancel = false, nil
		for _, r := range run.results {
			if r.State == volumeCopyPending || r.State == volumeCopyRunning {
				r.State, r.Error = volumeCopyFailed, "the copy was cancelled before this volume finished"
			}
		}
	}
}

// volumeCopyJobs lists what the tunnel can copy and records why anything else
// falls back to the rsync command.
func (rt *Router) volumeCopyJobs(items []store.AppImportItem, disc *platformimport.Discovery, only []string) ([]volumeCopyJob, []*volumeCopyResult) {
	containers := map[string]string{}
	for _, a := range disc.Apps {
		containers[a.SourceID] = a.SourceContainer
	}
	var jobs []volumeCopyJob
	var results []*volumeCopyResult
	for _, it := range items {
		if !it.Selected || it.TargetName == "" || stageable(it.State) || !wantedItem(only, it) {
			continue
		}
		for i, v := range it.Volumes {
			res := &volumeCopyResult{SourceID: it.SourceID, App: it.SourceName, Volume: v.Name, ContainerPath: v.ContainerPath, State: volumeCopyPending, index: i}
			results = append(results, res)
			switch {
			case strings.HasPrefix(v.Name, "/"):
				res.State, res.Fallback = volumeCopySkipped, "bind mounts are copied by hand: "+rsyncFallback
			case containers[it.SourceID] == "":
				res.State, res.Fallback = volumeCopySkipped, "the source container is not running, so its volume cannot be read through the tunnel: "+rsyncFallback
			default:
				jobs = append(jobs, volumeCopyJob{item: it, vol: v, index: i, container: containers[it.SourceID], result: res})
			}
		}
	}
	return jobs, results
}

// handleAppImportVolumeCopyStatus handles GET .../sessions/{id}/volumes/copy.
func (rt *Router) handleAppImportVolumeCopyStatus(w http.ResponseWriter, r *http.Request) {
	sess, _, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, rt.appImportLive.volumeView(sess.ID))
}

// handleAppImportVolumeCopy handles POST .../sessions/{id}/volumes/copy. It
// returns at once; poll the GET route for progress.
func (rt *Router) handleAppImportVolumeCopy(w http.ResponseWriter, r *http.Request) {
	sess, items, ok := rt.loadAppImport(w, r)
	if !ok {
		return
	}
	if sess.Platform != string(platformimport.DockerSSH) {
		writeError(w, http.StatusConflict, "only a docker-ssh session can copy volumes through the tunnel; use the rsync commands")
		return
	}
	if rt.execRuntime == nil {
		writeError(w, http.StatusNotImplemented, "this control plane has no node runtime to copy volumes into")
		return
	}
	req, ok := decodeItemsRequest(w, r)
	if !ok {
		return
	}
	live := rt.appImportLive.get(sess.ID)
	cfg, err := rt.sessionSSHConfig(sess.ID)
	if err != nil || live == nil {
		writeError(w, http.StatusConflict, "the ssh login is held in memory only and is not connected: reconnect the source first")
		return
	}
	jobs, results := rt.volumeCopyJobs(items, live.disc, req.Items)
	if len(results) == 0 {
		writeError(w, http.StatusBadRequest, "no staged app has a volume to copy: stage the import first")
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), envDuration(envVolumeCopyTimeout, defaultVolumeTimeout))
	if !rt.appImportLive.startVolumeCopy(sess.ID, cancel, results) {
		cancel()
		writeError(w, http.StatusConflict, "a volume copy is already running for this session")
		return
	}
	rt.setAppImportStep(r.Context(), sess, appImportStepVolumes)
	rt.logger.Info("api: app import: volume copy started", slog.String("session_id", sess.ID), slog.Int("volumes", len(jobs)))
	go rt.runVolumeCopy(ctx, cancel, sess.ID, cfg, jobs)
	writeJSON(w, http.StatusAccepted, rt.appImportLive.volumeView(sess.ID))
}

func (rt *Router) runVolumeCopy(ctx context.Context, cancel context.CancelFunc, sessID string, cfg dockerssh.Config, jobs []volumeCopyJob) {
	defer cancel()
	defer rt.appImportLive.finishVolumeCopy(sessID)
	fail := func(j volumeCopyJob, err error) {
		rt.appImportLive.updateVolume(j.result, func(r *volumeCopyResult) {
			r.State, r.Error, r.Fallback = volumeCopyFailed, err.Error(), rsyncFallback
		})
		rt.logger.Warn("api: app import: volume copy failed", slog.String("session_id", sessID), slog.String("source_id", j.item.SourceID), slog.String("volume", j.vol.Name), slog.String("error", err.Error()))
	}
	if len(jobs) == 0 {
		return
	}
	sess, err := dockerssh.Open(ctx, cfg)
	if err != nil {
		for _, j := range jobs {
			fail(j, err)
		}
		return
	}
	defer sess.Close()
	lim := volumeCopyLimits()
	for _, j := range jobs {
		if ctx.Err() != nil {
			return
		}
		rt.appImportLive.updateVolume(j.result, func(r *volumeCopyResult) { r.State = volumeCopyRunning })
		dir, err := rt.volumeTargetDir(ctx, j)
		if err != nil {
			fail(j, err)
			continue
		}
		st, err := sess.CopyMount(ctx, j.container, j.vol.ContainerPath, dir, lim)
		if err != nil {
			fail(j, fmt.Errorf("copy %s: %w", j.vol.ContainerPath, err))
			continue
		}
		rt.appImportLive.updateVolume(j.result, func(r *volumeCopyResult) {
			r.State, r.Files, r.Bytes, r.Skipped, r.Warnings, r.Error = volumeCopyDone, st.Files, st.Bytes, st.Skipped, st.Warnings, ""
		})
		rt.markVolumeCopied(ctx, sessID, j)
		rt.logger.Info("api: app import: volume copied", slog.String("session_id", sessID), slog.String("source_id", j.item.SourceID),
			slog.String("volume", j.vol.Name), slog.Int("files", st.Files), slog.Int64("bytes", st.Bytes), slog.Int("skipped", st.Skipped))
	}
}

func (rt *Router) volumeTargetDir(ctx context.Context, j volumeCopyJob) (string, error) {
	node := rt.imageNode(ctx, j.item.TargetName)
	r, err := rt.execRuntime(node)
	if err != nil {
		return "", fmt.Errorf("reach the target node: %w", err)
	}
	vm, ok := r.(volumeMounter)
	if !ok {
		return "", fmt.Errorf("the app runs on a node whose volumes this control plane cannot write to: %s", rsyncFallback)
	}
	dir, err := vm.VolumeMountpoint(ctx, j.vol.Name)
	if err != nil {
		return "", err
	}
	if fi, serr := os.Stat(dir); serr != nil || !fi.IsDir() {
		return "", fmt.Errorf("the volume directory %s is not reachable from the control plane process: %s", dir, rsyncFallback)
	}
	return dir, nil
}

func (rt *Router) markVolumeCopied(ctx context.Context, sessID string, j volumeCopyJob) {
	items, err := rt.appImports.ListAppImportItems(ctx, sessID)
	if err != nil {
		return
	}
	for _, it := range items {
		if it.SourceID == j.item.SourceID && j.index < len(it.Volumes) {
			it.Volumes[j.index].Copied = true
			it.Volumes[j.index].CopiedAt = time.Now().UTC().Format(time.RFC3339)
			rt.saveAppImportItem(ctx, it)
		}
	}
}
