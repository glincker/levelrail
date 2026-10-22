package api

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/repolayout"
	"github.com/GLINCKER/levelrail/internal/store"
)

// The current side has no commit or status, since live desired state is not a historical record.
// An attempt recorded before a snapshot field existed shows zero for it, not a real value.
type deployCompareSide struct {
	DeployID    string     `json:"deploy_id,omitempty"`
	IsCurrent   bool       `json:"is_current"`
	Image       string     `json:"image"`
	ImageDigest string     `json:"image_digest,omitempty"`
	CommitSHA   string     `json:"commit_sha,omitempty"`
	Source      string     `json:"source,omitempty"`
	Status      string     `json:"status,omitempty"`
	StartedAt   *time.Time `json:"started_at,omitempty"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`

	Port        int                         `json:"port,omitempty"`
	HostPort    *int                        `json:"host_port,omitempty"`
	BindAddress string                      `json:"bind_address,omitempty"`
	Domains     []string                    `json:"domains,omitempty"`
	Resources   *store.ServiceResources     `json:"resources,omitempty"`
	Env         []store.DeployAttemptEnvKey `json:"env,omitempty"`
	Health      *store.ServiceHealth        `json:"health,omitempty"`
	Replicas    int                         `json:"replicas,omitempty"`
	Strategy    string                      `json:"strategy,omitempty"`
	Volumes     []store.ServiceVolume       `json:"volumes,omitempty"`
	Labels      map[string]string           `json:"labels,omitempty"`
}

// deployCompareField is one field that differs between From and To.
type deployCompareField struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// Secret and database env keys are never reported as changed: their values cannot be compared without decrypting or resolving them.
const (
	deployCompareEnvAdded   = "added"
	deployCompareEnvRemoved = "removed"
	deployCompareEnvChanged = "changed"
)

// From and To are only populated for literal env keys, since secret and database values are never snapshotted.
type deployCompareEnvChange struct {
	Key    string                     `json:"key"`
	Kind   store.DeployAttemptEnvKind `json:"kind"`
	Status string                     `json:"status"`
	From   string                     `json:"from,omitempty"`
	To     string                     `json:"to,omitempty"`
}

// deployCompareResource is GET .../deploys/compare's response shape.
type deployCompareResource struct {
	ServiceName         string                   `json:"service_name"`
	From                deployCompareSide        `json:"from"`
	To                  deployCompareSide        `json:"to"`
	Changes             []deployCompareField     `json:"changes"`
	EnvChanges          []deployCompareEnvChange `json:"env_changes,omitempty"`
	Commits             []repolayout.Commit      `json:"commits"`
	CommitsNote         string                   `json:"commits_note,omitempty"`
	UnsnapshottedFields []string                 `json:"unsnapshotted_fields"`
	Note                string                   `json:"note"`
}

// Kept as a field so a future DesiredService field without a snapshot has a place to be listed.
var unsnapshottedDeployFields = []string{}

const deployCompareUnsnapshottedNote = "Deploy attempts record image tag, commit, trigger source, outcome, environment variable keys, ports, bind addresses, domains, resource limits, health check config, replica count, deploy strategy, volumes, and labels at trigger time (attempts recorded before a given field was added show an empty snapshot for it). A secret- or database-backed env var's value is never recorded: only its key, and whether it was added or removed, ever appears in env_changes, since this control plane cannot know if such a value changed without decrypting it."

// Compares only the fields an attempt snapshot captures, not a fabricated full config diff.
func (rt *Router) handleCompareDeploys(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.internalError(w, "api: compare deploys: load app failed", err, slog.String("name", name))
		return
	}

	q := r.URL.Query()
	fromID := q.Get("from")
	toID := q.Get("to")
	pending := q.Get("pending") == "true"
	if fromID == "" && pending {
		fromID = rt.latestSucceededAttemptID(r, name)
		if fromID == "" {
			writeError(w, http.StatusNotFound, "no earlier successful deploy to compare against")
			return
		}
	}
	if fromID == "" {
		writeError(w, http.StatusBadRequest, "from is required")
		return
	}

	fromSide, ok := rt.loadDeployCompareSide(w, r, name, fromID)
	if !ok {
		return
	}

	toSide := currentDeployCompareSide(*svc)
	if img := strings.TrimSpace(q.Get("image")); pending && img != "" {
		toSide.Image = img
		toSide.ImageDigest = ""
	}
	if toID != "" {
		toSide, ok = rt.loadDeployCompareSide(w, r, name, toID)
		if !ok {
			return
		}
	}

	commits, commitsNote := rt.commitsForRange(r.Context(), name, fromSide.CommitSHA, toSide.CommitSHA)
	out := deployCompareResource{
		ServiceName:         name,
		Commits:             commits,
		CommitsNote:         commitsNote,
		From:                maskDeploySideEnv(fromSide),
		To:                  maskDeploySideEnv(toSide),
		Changes:             diffDeployCompareSides(fromSide, toSide),
		EnvChanges:          diffDeployCompareEnv(fromSide.Env, toSide.Env),
		UnsnapshottedFields: unsnapshottedDeployFields,
		Note:                deployCompareUnsnapshottedNote,
	}
	if pending {
		out = keysOnlyDeployCompare(out)
	}
	writeJSON(w, http.StatusOK, out)
}

// latestSucceededAttemptID returns the newest succeeded attempt, which is what is live now; empty when there is none.
func (rt *Router) latestSucceededAttemptID(r *http.Request, name string) string {
	attempts, err := rt.deployAttempts.ListDeployAttempts(r.Context(), name)
	if err != nil {
		rt.logger.Error("api: compare deploys: list attempts failed", slog.String("error", err.Error()), slog.String("name", name))
		return ""
	}
	for _, a := range attempts {
		if a.Status == store.DeployAttemptStatusSucceeded {
			return a.ID
		}
	}
	return ""
}

// keysOnlyDeployCompare drops every env value: a pre-deploy diff shows which keys change, never what they hold.
func keysOnlyDeployCompare(c deployCompareResource) deployCompareResource {
	strip := func(side deployCompareSide) deployCompareSide {
		env := make([]store.DeployAttemptEnvKey, len(side.Env))
		for i, e := range side.Env {
			e.Value = ""
			env[i] = e
		}
		side.Env = env
		return side
	}
	c.From, c.To = strip(c.From), strip(c.To)
	changes := make([]deployCompareEnvChange, len(c.EnvChanges))
	for i, e := range c.EnvChanges {
		e.From, e.To = "", ""
		changes[i] = e
	}
	c.EnvChanges = changes
	return c
}

// Shaped like a real attempt snapshot, so the diff logic never needs to know which side is live.
func currentDeployCompareSide(svc store.DesiredService) deployCompareSide {
	snap := store.NewDeployAttemptSnapshot(svc)
	return deployCompareSide{
		IsCurrent: true, Image: svc.Image, ImageDigest: appImageDigest(svc),
		Port: snap.Port, HostPort: snap.HostPort, BindAddress: snap.BindAddress, Domains: snap.Domains,
		Resources: snap.Resources, Env: snap.Env,
		Health: snap.Health, Replicas: snap.Replicas, Strategy: snap.Strategy,
		Volumes: snap.Volumes, Labels: snap.Labels,
	}
}

// loadDeployCompareSide loads one deploy attempt by id, scoped to
// appName, writing the 404/500 response itself (ok=false) on failure so
// handleCompareDeploys's two call sites (from, to) share one error path.
func (rt *Router) loadDeployCompareSide(w http.ResponseWriter, r *http.Request, appName, deployID string) (side deployCompareSide, ok bool) {
	a, err := rt.deployAttempts.GetDeployAttempt(r.Context(), deployID)
	if errors.Is(err, store.ErrDeployAttemptNotFound) {
		writeError(w, http.StatusNotFound, "deploy attempt not found: "+deployID)
		return deployCompareSide{}, false
	}
	if err != nil {
		rt.internalError(w, "api: compare deploys: load attempt failed", err, slog.String("deploy_id", deployID))
		return deployCompareSide{}, false
	}
	if a.ServiceName != appName {
		writeError(w, http.StatusNotFound, "deploy attempt not found: "+deployID)
		return deployCompareSide{}, false
	}
	return deployCompareSide{
		DeployID:    a.ID,
		Image:       a.Image,
		ImageDigest: a.ImageDigest,
		CommitSHA:   a.CommitSHA,
		Source:      a.Source,
		Status:      a.Status,
		StartedAt:   &a.StartedAt,
		FinishedAt:  a.FinishedAt,
		Port:        a.Snapshot.Port,
		HostPort:    a.Snapshot.HostPort,
		BindAddress: a.Snapshot.BindAddress,
		Domains:     a.Snapshot.Domains,
		Resources:   a.Snapshot.Resources,
		Env:         a.Snapshot.Env,
		Health:      a.Snapshot.Health,
		Replicas:    a.Snapshot.Replicas,
		Strategy:    a.Snapshot.Strategy,
		Volumes:     a.Snapshot.Volumes,
		Labels:      a.Snapshot.Labels,
	}, true
}

// Source is compared only between real attempts; the current side has none, which would read as a change.
func diffDeployCompareSides(from, to deployCompareSide) []deployCompareField {
	var changes []deployCompareField
	if from.Image != to.Image {
		changes = append(changes, deployCompareField{Field: "image", From: from.Image, To: to.Image})
	}
	if from.ImageDigest != to.ImageDigest && (from.ImageDigest != "" || to.ImageDigest != "") {
		changes = append(changes, deployCompareField{Field: "image_digest", From: from.ImageDigest, To: to.ImageDigest})
	}
	if from.CommitSHA != to.CommitSHA && (!to.IsCurrent || to.CommitSHA != "") {
		changes = append(changes, deployCompareField{Field: "commit_sha", From: from.CommitSHA, To: to.CommitSHA})
	}
	if !to.IsCurrent && from.Source != to.Source {
		changes = append(changes, deployCompareField{Field: "source", From: from.Source, To: to.Source})
	}
	if from.Port != to.Port {
		changes = append(changes, deployCompareField{Field: "port", From: strconv.Itoa(from.Port), To: strconv.Itoa(to.Port)})
	}
	if fromHostPort, toHostPort := hostPortString(from.HostPort), hostPortString(to.HostPort); fromHostPort != toHostPort {
		changes = append(changes, deployCompareField{Field: "host_port", From: fromHostPort, To: toHostPort})
	}
	if from.BindAddress != to.BindAddress {
		changes = append(changes, deployCompareField{Field: "bind_address", From: from.BindAddress, To: to.BindAddress})
	}
	if fromDomains, toDomains := strings.Join(from.Domains, ", "), strings.Join(to.Domains, ", "); fromDomains != toDomains {
		changes = append(changes, deployCompareField{Field: "domains", From: fromDomains, To: toDomains})
	}
	if from.Strategy != to.Strategy {
		changes = append(changes, deployCompareField{Field: "strategy", From: from.Strategy, To: to.Strategy})
	}
	if from.Replicas != to.Replicas {
		changes = append(changes, deployCompareField{Field: "replicas", From: strconv.Itoa(from.Replicas), To: strconv.Itoa(to.Replicas)})
	}
	if fromVolumes, toVolumes := volumesString(from.Volumes), volumesString(to.Volumes); fromVolumes != toVolumes {
		changes = append(changes, deployCompareField{Field: "volumes", From: fromVolumes, To: toVolumes})
	}
	if fromLabels, toLabels := labelsString(from.Labels), labelsString(to.Labels); fromLabels != toLabels {
		changes = append(changes, deployCompareField{Field: "labels", From: fromLabels, To: toLabels})
	}
	changes = append(changes, diffDeployCompareResources(from.Resources, to.Resources)...)
	changes = append(changes, diffDeployCompareHealth(from.Health, to.Health)...)
	return changes
}

// Joined into one field: volume lists are small and order-stable, unlike env.
func volumesString(vols []store.ServiceVolume) string {
	parts := make([]string, len(vols))
	for i, v := range vols {
		parts[i] = v.Name + ":" + v.ContainerPath
	}
	return strings.Join(parts, ", ")
}

// Sorted by key so the comparison does not depend on map iteration order.
func labelsString(labels map[string]string) string {
	if len(labels) == 0 {
		return ""
	}
	keys := make([]string, 0, len(labels))
	for k := range labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = k + "=" + labels[k]
	}
	return strings.Join(parts, ", ")
}

// A nil probe counts as the zero value, matching how a nil Resources side is treated.
func diffDeployCompareHealth(from, to *store.ServiceHealth) []deployCompareField {
	f, t := healthOrZero(from), healthOrZero(to)
	var changes []deployCompareField
	changes = append(changes, diffDeployCompareProbe("health.readiness", f.Readiness, t.Readiness)...)
	changes = append(changes, diffDeployCompareProbe("health.liveness", f.Liveness, t.Liveness)...)
	if f.ReadyTimeout != t.ReadyTimeout {
		changes = append(changes, deployCompareField{Field: "health.readyTimeout", From: f.ReadyTimeout.String(), To: t.ReadyTimeout.String()})
	}
	return changes
}

func healthOrZero(h *store.ServiceHealth) store.ServiceHealth {
	if h == nil {
		return store.ServiceHealth{}
	}
	return *h
}

func diffDeployCompareProbe(field string, from, to *store.ServiceProbe) []deployCompareField {
	f, t := probeOrZero(from), probeOrZero(to)
	var changes []deployCompareField
	if f.Path != t.Path {
		changes = append(changes, deployCompareField{Field: field + ".path", From: f.Path, To: t.Path})
	}
	if f.Interval != t.Interval {
		changes = append(changes, deployCompareField{Field: field + ".interval", From: f.Interval.String(), To: t.Interval.String()})
	}
	if f.Timeout != t.Timeout {
		changes = append(changes, deployCompareField{Field: field + ".timeout", From: f.Timeout.String(), To: t.Timeout.String()})
	}
	if f.Failures != t.Failures {
		changes = append(changes, deployCompareField{Field: field + ".failures", From: strconv.Itoa(f.Failures), To: strconv.Itoa(t.Failures)})
	}
	for _, sub := range []struct{ name, from, to string }{
		{"scheme", f.Scheme, t.Scheme},
		{"host", f.Host, t.Host},
		{"tls_skip_verify", strconv.FormatBool(f.TLSSkipVerify), strconv.FormatBool(t.TLSSkipVerify)},
		{"follow_redirects", optionalBool(f.FollowRedirects), optionalBool(t.FollowRedirects)},
		{"expected_status", f.ExpectedStatus, t.ExpectedStatus},
		{"exec", strings.Join(f.Exec, " "), strings.Join(t.Exec, " ")},
	} {
		if sub.from != sub.to {
			changes = append(changes, deployCompareField{Field: field + "." + sub.name, From: sub.from, To: sub.to})
		}
	}
	return changes
}

func optionalBool(b *bool) string {
	if b == nil {
		return ""
	}
	return strconv.FormatBool(*b)
}

func probeOrZero(p *store.ServiceProbe) store.ServiceProbe {
	if p == nil {
		return store.ServiceProbe{}
	}
	return *p
}

// A nil side counts as the zero value, matching how an absent limit is read elsewhere.
func diffDeployCompareResources(from, to *store.ServiceResources) []deployCompareField {
	f, t := resourcesOrZero(from), resourcesOrZero(to)
	var changes []deployCompareField
	if f.MemoryBytes != t.MemoryBytes {
		changes = append(changes, deployCompareField{Field: "resources.memory_bytes", From: strconv.FormatInt(f.MemoryBytes, 10), To: strconv.FormatInt(t.MemoryBytes, 10)})
	}
	if f.NanoCPUs != t.NanoCPUs {
		changes = append(changes, deployCompareField{Field: "resources.nano_cpus", From: strconv.FormatInt(f.NanoCPUs, 10), To: strconv.FormatInt(t.NanoCPUs, 10)})
	}
	if f.SwapMemoryBytes != t.SwapMemoryBytes {
		changes = append(changes, deployCompareField{Field: "resources.swap_memory_bytes", From: strconv.FormatInt(f.SwapMemoryBytes, 10), To: strconv.FormatInt(t.SwapMemoryBytes, 10)})
	}
	if f.CPUSetCPUs != t.CPUSetCPUs {
		changes = append(changes, deployCompareField{Field: "resources.cpuset_cpus", From: f.CPUSetCPUs, To: t.CPUSetCPUs})
	}
	return changes
}

func resourcesOrZero(r *store.ServiceResources) store.ServiceResources {
	if r == nil {
		return store.ServiceResources{}
	}
	return *r
}

func hostPortString(p *int) string {
	if p == nil {
		return ""
	}
	return strconv.Itoa(*p)
}

// Secret or database keys present on both sides are omitted, not reported as unchanged.
func diffDeployCompareEnv(from, to []store.DeployAttemptEnvKey) []deployCompareEnvChange {
	fromByKey := envKeysByName(from)
	toByKey := envKeysByName(to)

	keySet := make(map[string]struct{}, len(fromByKey)+len(toByKey))
	for k := range fromByKey {
		keySet[k] = struct{}{}
	}
	for k := range toByKey {
		keySet[k] = struct{}{}
	}
	keys := make([]string, 0, len(keySet))
	for k := range keySet {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var out []deployCompareEnvChange
	for _, k := range keys {
		f, hadFrom := fromByKey[k]
		t, hadTo := toByKey[k]
		switch {
		case hadFrom && !hadTo:
			out = append(out, deployCompareEnvChange{Key: k, Kind: f.Kind, Status: deployCompareEnvRemoved})
		case !hadFrom && hadTo:
			out = append(out, deployCompareEnvChange{Key: k, Kind: t.Kind, Status: deployCompareEnvAdded})
		case f.Kind == store.DeployAttemptEnvKindLiteral && t.Kind == store.DeployAttemptEnvKindLiteral && f.Value != t.Value:
			from, _ := envredact.Value(k, f.Value, nil)
			to, _ := envredact.Value(k, t.Value, nil)
			out = append(out, deployCompareEnvChange{Key: k, Kind: t.Kind, Status: deployCompareEnvChanged, From: from, To: to})
		}
	}
	return out
}

func envKeysByName(keys []store.DeployAttemptEnvKey) map[string]store.DeployAttemptEnvKey {
	out := make(map[string]store.DeployAttemptEnvKey, len(keys))
	for _, k := range keys {
		out[k.Key] = k
	}
	return out
}

// maskDeploySideEnv masks sensitive literal values in a snapshot side.
func maskDeploySideEnv(side deployCompareSide) deployCompareSide {
	env := make([]store.DeployAttemptEnvKey, len(side.Env))
	for i, e := range side.Env {
		if e.Kind == store.DeployAttemptEnvKindLiteral {
			e.Value, _ = envredact.Value(e.Key, e.Value, nil)
		}
		env[i] = e
	}
	side.Env = env
	return side
}
