package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

// The journal holds only kinds, names, step states and fingerprints: never values, so a leaked journal leaks no secret.
const (
	journalVersion        = 1
	journalKindApp        = "app"
	journalMaxBytesEnv    = "APP_MIGRATE_JOURNAL_MAX_BYTES"
	journalMaxBytesDflt   = 4 << 20
	journalDirName        = "migrate-journals"
	journalLockSuffix     = ".lock"
	journalFingerprintPfx = "sha256:"
)

const (
	stepCreated   = "created"
	stepGitSource = "git_source"
	stepNode      = "node"
	stepIngress   = "ingress"
	stepPreviews  = "previews"
	stepSecrets   = "secrets"
	stepFiles     = "files"
	stepWebhook   = "webhook"
	stepUndone    = "undone"
	stepPaused    = "paused"
)

const (
	stateDone    = "done"
	stateFailed  = "failed"
	stateSkipped = "skipped"
	statePending = "pending"
	stateReview  = "review"
	stateUnknown = "unknown"
)

var (
	journalSteps         = []string{stepCreated, stepGitSource, stepNode, stepIngress, stepPreviews, stepSecrets, stepFiles, stepWebhook, stepUndone, stepPaused}
	journalStates        = []string{stateDone, stateFailed, stateSkipped, statePending, stateReview, stateUnknown}
	journalSources       = []string{"coolify", "dokploy"}
	journalKinds         = []string{journalKindApp, journalKindBackupTarget, journalKindBackupSchedule}
	journalNameRE        = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
	journalTargetIDRE    = regexp.MustCompile(`^[A-Za-z0-9_-]{1,64}$`)
	journalFingerprintRE = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type migrateJournal struct {
	Version    int               `json:"version"`
	Source     string            `json:"source"`
	SourceHost string            `json:"source_host"`
	TargetHost string            `json:"target_host"`
	StartedAt  string            `json:"started_at"`
	UpdatedAt  string            `json:"updated_at"`
	Resources  []journalResource `json:"resources"`
}

type journalResource struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
	// CreatedByRun is true only once this run's create call succeeded; undo never touches anything else.
	CreatedByRun bool          `json:"created_by_run"`
	Steps        []journalStep `json:"steps"`
	Fingerprint  string        `json:"fingerprint,omitempty"`
	Deploys      int           `json:"deploys"`
	HasData      bool          `json:"has_data,omitempty"`
	// TargetID and Cron identify an imported backup target or schedule for undo and resume.
	TargetID string `json:"target_id,omitempty"`
	Cron     string `json:"cron,omitempty"`
}

type journalStep struct {
	Step  string `json:"step"`
	State string `json:"state"`
	At    string `json:"at"`
}

func (r *journalResource) state(step string) string {
	for _, s := range r.Steps {
		if s.Step == step {
			return s.State
		}
	}
	return ""
}

func (r *journalResource) set(step, state string, now time.Time) {
	at := now.UTC().Format(time.RFC3339)
	for i := range r.Steps {
		if r.Steps[i].Step == step {
			r.Steps[i].State, r.Steps[i].At = state, at
			return
		}
	}
	r.Steps = append(r.Steps, journalStep{Step: step, State: state, At: at})
}

// needsRun is true for a step this run has not finished: never attempted, pending or failed.
func (r *journalResource) needsRun(step string) bool {
	s := r.state(step)
	return s == "" || s == statePending || s == stateFailed
}

func (j *migrateJournal) find(kind, name string) *journalResource {
	for i := range j.Resources {
		if j.Resources[i].Kind == kind && j.Resources[i].Name == name {
			return &j.Resources[i]
		}
	}
	return nil
}

func (j *migrateJournal) ensure(kind, name string) *journalResource {
	if r := j.find(kind, name); r != nil {
		return r
	}
	j.Resources = append(j.Resources, journalResource{Kind: kind, Name: name})
	return &j.Resources[len(j.Resources)-1]
}

// originOf keeps only scheme and host, so a URL with userinfo, a path or a query never reaches the journal.
func originOf(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
}

func journalMaxBytes(lookupEnv func(string) (string, bool)) int64 {
	return int64(envIntDefault(lookupEnv, journalMaxBytesEnv, journalMaxBytesDflt))
}

func defaultJournalPath(prog, source string, now time.Time) (string, error) {
	dir, err := configDir(prog)
	if err != nil {
		return "", fmt.Errorf("find the config dir for the journal (pass --journal): %w", err)
	}
	return filepath.Join(dir, journalDirName, source+"-"+now.UTC().Format("20060102T150405Z")+".json"), nil
}

// appFingerprint hashes the app's shape without env or secrets, so any later edit, placement or volume change shows.
func appFingerprint(app apiclient.AppResource) string {
	domains := slices.Clone(app.Domains)
	slices.Sort(domains)
	shape := struct {
		Name, Image, TemplateID, NodeID, ProjectID, EnvironmentID string
		Port, Replicas                                            int
		Domains, Command                                          []string
		Volumes                                                   []apiclient.AppVolumeResource
		BindMounts                                                []apiclient.AppBindMountResource
	}{app.Name, app.Image, app.TemplateID, app.NodeID, app.ProjectID, app.EnvironmentID,
		app.Port, app.Replicas, domains, app.Command, app.Volumes, app.BindMounts}
	b, _ := json.Marshal(shape)
	sum := sha256.Sum256(b)
	return journalFingerprintPfx + hex.EncodeToString(sum[:])
}

func appHasData(app apiclient.AppResource) bool {
	return len(app.Volumes) > 0
}

func validateJournal(j *migrateJournal, source string) error {
	if j.Version != journalVersion {
		return fmt.Errorf("journal version %d is not supported", j.Version)
	}
	if !slices.Contains(journalSources, j.Source) || (source != "" && j.Source != source) {
		return fmt.Errorf("journal source %q does not match %q", j.Source, source)
	}
	for _, h := range []string{j.SourceHost, j.TargetHost} {
		if h != "" && originOf(h) != h {
			return errors.New("journal host fields must be a bare scheme and host")
		}
	}
	for _, ts := range []string{j.StartedAt, j.UpdatedAt} {
		if _, err := time.Parse(time.RFC3339, ts); err != nil {
			return errors.New("journal timestamps must be RFC 3339")
		}
	}
	seen := map[string]bool{}
	for i, r := range j.Resources {
		if !slices.Contains(journalKinds, r.Kind) {
			return fmt.Errorf("resource %d: kind %q is not one this importer creates", i, r.Kind)
		}
		if !journalNameRE.MatchString(r.Name) || sanitizeServiceName(r.Name) != r.Name {
			return fmt.Errorf("resource %d: %q is not a valid %s name", i, r.Name, r.Kind)
		}
		if seen[r.Kind+"/"+r.Name] {
			return fmt.Errorf("resource %q is listed twice", r.Name)
		}
		seen[r.Kind+"/"+r.Name] = true
		if r.Fingerprint != "" && !journalFingerprintRE.MatchString(r.Fingerprint) {
			return fmt.Errorf("resource %q: malformed fingerprint", r.Name)
		}
		if r.Deploys < 0 {
			return fmt.Errorf("resource %q: negative deploy count", r.Name)
		}
		if (r.TargetID != "" && !journalTargetIDRE.MatchString(r.TargetID)) || len(r.Cron) > 128 || hasControlChar(r.Cron) {
			return fmt.Errorf("resource %q: malformed target id or cron", r.Name)
		}
		steps := map[string]bool{}
		for _, s := range r.Steps {
			if !slices.Contains(journalSteps, s.Step) || !slices.Contains(journalStates, s.State) || steps[s.Step] {
				return fmt.Errorf("resource %q: unknown or repeated step %q", r.Name, s.Step)
			}
			steps[s.Step] = true
		}
		if r.CreatedByRun && r.state(stepCreated) != stateDone {
			return fmt.Errorf("resource %q: marked created without a done create step", r.Name)
		}
	}
	return nil
}

// loadJournal refuses a symlink, a non-regular file, a file swapped between stat and open, an oversized
// file and (outside Windows) one readable by others, then validates every field it decodes.
func loadJournal(path, source string, maxBytes int64) (*migrateJournal, error) {
	fi, err := os.Lstat(path)
	if err != nil {
		return nil, fmt.Errorf("read journal: %w", err)
	}
	if fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular() {
		return nil, fmt.Errorf("journal %s is not a regular file (symlinks are refused)", path)
	}
	if fi.Size() > maxBytes {
		return nil, fmt.Errorf("journal %s is %d bytes, over the %d byte limit (%s)", path, fi.Size(), maxBytes, journalMaxBytesEnv)
	}
	if runtime.GOOS != "windows" && fi.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("journal %s is readable or writable by others (mode %o), chmod 600 it first", path, fi.Mode().Perm())
	}
	if err := checkJournalParent(path); err != nil {
		return nil, err
	}
	f, err := openJournalNoFollow(path)
	if err != nil {
		return nil, fmt.Errorf("read journal: %w", err)
	}
	defer func() { _ = f.Close() }()
	if open, err := f.Stat(); err != nil || !open.Mode().IsRegular() || !os.SameFile(fi, open) {
		return nil, fmt.Errorf("journal %s changed while it was opened", path)
	}
	data, err := io.ReadAll(io.LimitReader(f, maxBytes+1))
	if err != nil {
		return nil, fmt.Errorf("read journal: %w", err)
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("journal %s is over the %d byte limit (%s)", path, maxBytes, journalMaxBytesEnv)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var j migrateJournal
	if err := dec.Decode(&j); err != nil {
		return nil, fmt.Errorf("journal %s is not valid: %w", path, err)
	}
	if err := validateJournal(&j, source); err != nil {
		return nil, fmt.Errorf("journal %s is not valid: %w", path, err)
	}
	return &j, nil
}

// saveJournal writes a 0600 temp file opened with O_EXCL beside the target, then renames it over the
// target; rename replaces a planted symlink instead of following it, and a symlink target is refused anyway.
func saveJournal(path string, j *migrateJournal, now time.Time) error {
	j.UpdatedAt = now.UTC().Format(time.RFC3339)
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return fmt.Errorf("encode journal: %w", err)
	}
	if fi, err := os.Lstat(path); err == nil && (fi.Mode()&os.ModeSymlink != 0 || !fi.Mode().IsRegular()) {
		return fmt.Errorf("journal %s is not a regular file (symlinks are refused)", path)
	}
	var suffix [8]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		return fmt.Errorf("write journal: %w", err)
	}
	tmp := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+"."+hex.EncodeToString(suffix[:])+".tmp")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // temp name is random, dir is the operator's journal dir
	if err != nil {
		return fmt.Errorf("write journal: %w", err)
	}
	_, werr := f.Write(append(data, '\n'))
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil {
		werr = os.Rename(tmp, path)
	}
	if werr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("write journal: %w", werr)
	}
	return nil
}

// lockJournal takes an O_EXCL lock file so two runs never write or undo the same journal at once.
func lockJournal(path string) (func(), error) {
	lock := path + journalLockSuffix
	f, err := os.OpenFile(lock, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600) //nolint:gosec // lock beside the operator's journal
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil, fmt.Errorf("journal %s is in use by another run%s (remove %s if that run is gone)", path, lockOwner(lock), lock)
		}
		return nil, fmt.Errorf("lock journal: %w", err)
	}
	_, _ = f.WriteString(strconv.Itoa(os.Getpid()) + "\n")
	_ = f.Close()
	return func() { _ = os.Remove(lock) }, nil
}

// lockOwner describes the lock holder by pid and age; it never follows a symlinked lock.
func lockOwner(lock string) string {
	fi, err := os.Lstat(lock)
	if err != nil || !fi.Mode().IsRegular() || fi.Size() > 64 {
		return ""
	}
	b, err := os.ReadFile(lock) //nolint:gosec // lock beside the operator's journal, checked above
	if err != nil {
		return ""
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(b)))
	if err != nil {
		return ""
	}
	return fmt.Sprintf(" (pid %d, locked %s ago)", pid, time.Since(fi.ModTime()).Round(time.Second))
}

// prepareJournalDir creates the default journal folder owner-only; an operator-chosen folder must already exist.
func prepareJournalDir(path string, isDefault bool) error {
	dir := filepath.Dir(path)
	if isDefault {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create journal dir: %w", err)
		}
	}
	return checkJournalParent(path)
}

// checkJournalParent refuses a symlinked journal folder, so a swapped folder cannot redirect writes.
func checkJournalParent(path string) error {
	dir := filepath.Dir(path)
	fi, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("journal dir: %w", err)
	}
	if !fi.IsDir() {
		return fmt.Errorf("journal dir %s is not a directory (a symlinked folder is refused, pass its real path)", dir)
	}
	return nil
}
