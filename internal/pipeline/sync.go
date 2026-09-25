package pipeline

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"path"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// Sync outcomes reported per pipeline file.
const (
	SyncCreated   = "created"
	SyncUpdated   = "updated"
	SyncUnchanged = "unchanged"
	// SyncDiverged means the stored definition was edited after the last
	// sync (or was never synced) and repo_is_truth is off, so it was kept.
	SyncDiverged = "diverged"
	SyncInvalid  = "invalid"
	SyncRefused  = "refused"

	// SourceRepo marks a pipeline whose definition comes from the repository.
	SourceRepo = "repo"

	maxSyncFiles    = 64
	maxSyncFileSize = 256 * 1024
)

var syncNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,63}$`)

// RepoFiles is the pipeline directory of a repository at a branch tip.
type RepoFiles struct {
	SHA   string
	Dir   string
	Files map[string][]byte
}

// DirFetcher reads the first existing pipeline directory of a repository.
type DirFetcher interface {
	FetchFiles(ctx context.Context, url, token, branch string, dirs []string) (RepoFiles, error)
}

// SyncStore is the persistence surface a Syncer needs. *store.DB satisfies it.
type SyncStore interface {
	ListPipelines(ctx context.Context, app string) ([]store.Pipeline, error)
	SavePipeline(ctx context.Context, p store.Pipeline) (store.Pipeline, error)
	GetPipelineSync(ctx context.Context, app string) (store.PipelineSync, error)
	SavePipelineSync(ctx context.Context, s store.PipelineSync) error
	GetGitSource(ctx context.Context, serviceName string) (*store.GitSource, error)
}

// SyncItem is the outcome for one pipeline file.
type SyncItem struct {
	File    string `json:"file"`
	Name    string `json:"name"`
	Outcome string `json:"outcome"`
	Message string `json:"message,omitempty"`
}

// SyncResult is the outcome of one repository sync.
type SyncResult struct {
	SHA   string     `json:"sha"`
	Dir   string     `json:"dir,omitempty"`
	Items []SyncItem `json:"items"`
}

// SyncConfig wires a Syncer.
type SyncConfig struct {
	Store   SyncStore
	Source  Source
	Fetcher DirFetcher
	// BrandName is the brand short name for the branded pipeline directory.
	BrandName string
	Logger    *slog.Logger
	Now       func() time.Time
	NewID     func() string
}

// Syncer copies an app's repository pipeline directory into its saved
// pipeline definitions.
type Syncer struct{ cfg SyncConfig }

// NewSyncer builds a Syncer, defaulting the logger, clock, and ID source.
func NewSyncer(cfg SyncConfig) *Syncer {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.NewID == nil {
		cfg.NewID = randomID
	}
	return &Syncer{cfg: cfg}
}

// HashYAML is the fingerprint stored as a definition's synced hash.
func HashYAML(y string) string {
	sum := sha256.Sum256([]byte(y))
	return hex.EncodeToString(sum[:])
}

// Diverged reports whether a repo-sourced definition was edited after its
// last sync.
func Diverged(p store.Pipeline) bool {
	return p.Source == SourceRepo && p.SyncedHash != "" && HashYAML(p.YAML) != p.SyncedHash
}

type syncAction int

const (
	actionCreate syncAction = iota
	actionUpdate
	actionUnchanged
	actionDiverged
)

// decideSync picks what a repo file does to the stored definition.
func decideSync(existing *store.Pipeline, hash string, repoIsTruth bool) syncAction {
	if existing == nil {
		return actionCreate
	}
	cur := HashYAML(existing.YAML)
	if cur == hash {
		if existing.Source == SourceRepo && existing.SyncedHash == hash {
			return actionUnchanged
		}
		return actionUpdate
	}
	editedLocally := existing.Source != SourceRepo || existing.SyncedHash != cur
	if editedLocally && !repoIsTruth {
		return actionDiverged
	}
	return actionUpdate
}

// Sync reads the repository at the app's tracked branch and upserts the
// definitions it finds. The outcome, or the error, is recorded so the
// dashboard can show it.
func (s *Syncer) Sync(ctx context.Context, app string) (SyncResult, error) {
	st := s.cfg.Store
	gs, err := st.GetGitSource(ctx, app)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		return SyncResult{}, ErrNoRepo
	}
	if err != nil {
		return SyncResult{}, fmt.Errorf("pipeline sync: load git source for %q: %w", app, err)
	}
	state, err := st.GetPipelineSync(ctx, app)
	if err != nil {
		return SyncResult{}, fmt.Errorf("pipeline sync: %w", err)
	}
	res, err := s.run(ctx, app, gs, state.RepoIsTruth)
	state.LastSyncAt = s.cfg.Now().UTC()
	if err != nil {
		state.LastError = err.Error()
	} else {
		state.LastError, state.LastSHA = "", res.SHA
	}
	if serr := st.SavePipelineSync(ctx, state); serr != nil {
		s.cfg.Logger.Warn("pipeline sync: record outcome failed", slog.String("app", app), slog.String("error", serr.Error()))
	}
	return res, err
}

// SyncOnPush syncs when ref is a push to the app's tracked branch. ran is
// false for any other ref or an app with no repository.
func (s *Syncer) SyncOnPush(ctx context.Context, app, ref string) (res SyncResult, ran bool, err error) {
	branch, ok := strings.CutPrefix(ref, "refs/heads/")
	if !ok {
		return SyncResult{}, false, nil
	}
	gs, err := s.cfg.Store.GetGitSource(ctx, app)
	if errors.Is(err, store.ErrGitSourceNotFound) {
		return SyncResult{}, false, nil
	}
	if err != nil {
		return SyncResult{}, false, fmt.Errorf("pipeline sync: load git source for %q: %w", app, err)
	}
	if gs.Branch != "" && gs.Branch != branch {
		return SyncResult{}, false, nil
	}
	res, err = s.Sync(ctx, app)
	return res, true, err
}

func (s *Syncer) run(ctx context.Context, app string, gs *store.GitSource, repoIsTruth bool) (SyncResult, error) {
	url, token, err := s.cfg.Source.RepoInfo(ctx, app)
	if err != nil {
		return SyncResult{}, fmt.Errorf("pipeline sync: resolve repository: %w", err)
	}
	files, err := s.cfg.Fetcher.FetchFiles(ctx, url, token, gs.Branch, DiscoverDirs(s.cfg.BrandName))
	if err != nil {
		return SyncResult{}, fmt.Errorf("pipeline sync: read repository: %w", err)
	}
	existing, err := s.cfg.Store.ListPipelines(ctx, app)
	if err != nil {
		return SyncResult{}, fmt.Errorf("pipeline sync: list pipelines: %w", err)
	}
	byName := map[string]*store.Pipeline{}
	for i := range existing {
		byName[existing[i].Name] = &existing[i]
	}

	res := SyncResult{SHA: files.SHA, Dir: files.Dir, Items: []SyncItem{}}
	names := make([]string, 0, len(files.Files))
	for f := range files.Files {
		names = append(names, f)
	}
	sort.Strings(names)
	seen := map[string]bool{}
	for i, file := range names {
		if i >= maxSyncFiles {
			res.Items = append(res.Items, SyncItem{File: file, Outcome: SyncInvalid, Message: fmt.Sprintf("more than %d pipeline files, the rest are ignored", maxSyncFiles)})
			break
		}
		item := s.syncFile(ctx, app, file, files.Files[file], files.SHA, byName, seen, repoIsTruth)
		res.Items = append(res.Items, item)
	}
	return res, nil
}

func (s *Syncer) syncFile(ctx context.Context, app, file string, data []byte, sha string, byName map[string]*store.Pipeline, seen map[string]bool, repoIsTruth bool) SyncItem {
	item := SyncItem{File: file}
	if len(data) > maxSyncFileSize {
		item.Outcome, item.Message = SyncInvalid, fmt.Sprintf("file is larger than %d KiB", maxSyncFileSize/1024)
		return item
	}
	def, issues := Validate(data)
	if len(issues) > 0 {
		item.Outcome, item.Message = SyncInvalid, issues[0].String()
		return item
	}
	item.Name = def.Name
	if item.Name == "" {
		item.Name = strings.TrimSuffix(file, path.Ext(file))
	}
	switch {
	case !syncNameRe.MatchString(item.Name):
		item.Outcome, item.Message = SyncInvalid, "name must be 1-64 characters: letters, digits, dash, underscore"
		return item
	case seen[item.Name]:
		item.Outcome, item.Message = SyncInvalid, "another file already defines this pipeline name"
		return item
	}
	seen[item.Name] = true
	if targets := TargetApps(def, app); len(targets) > 0 {
		item.Outcome, item.Message = SyncRefused, fmt.Sprintf("steps act on other apps (%s), which only an operator can save", strings.Join(targets, ", "))
		return item
	}

	yaml := string(data)
	hash := HashYAML(yaml)
	cur := byName[item.Name]
	action := decideSync(cur, hash, repoIsTruth)
	now := s.cfg.Now().UTC()
	if action == actionDiverged {
		item.Outcome, item.Message = SyncDiverged, "edited since the last sync; turn on repository as source of truth to overwrite it"
		return item
	}
	if action == actionUnchanged && cur.SourceSHA == sha {
		item.Outcome = SyncUnchanged
		return item
	}
	p := store.Pipeline{ID: s.cfg.NewID(), AppName: app, Name: item.Name, Enabled: true, CreatedAt: now}
	item.Outcome = SyncCreated
	if cur != nil {
		p, item.Outcome = *cur, SyncUpdated
		if action == actionUnchanged {
			item.Outcome = SyncUnchanged
		}
	}
	p.Source, p.YAML, p.SourceSHA, p.SyncedHash, p.UpdatedAt = SourceRepo, yaml, sha, hash, now
	if _, err := s.cfg.Store.SavePipeline(ctx, p); err != nil {
		item.Outcome, item.Message = SyncInvalid, "could not save: "+err.Error()
		s.cfg.Logger.Warn("pipeline sync: save failed", slog.String("app", app), slog.String("pipeline", item.Name), slog.String("error", err.Error()))
	}
	return item
}
