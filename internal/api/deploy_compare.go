package api

import (
	"errors"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// deployCompareSide is one side of a deploy comparison: either a real
// deploy attempt (DeployID set) or the app's current live desired state
// (IsCurrent true, DeployID empty). The current side carries no
// CommitSHA/Source/Status/timestamps, because DesiredService is live
// state, not a historical record: there is nothing to show for "now".
//
// Port/HostPort/Domains/Resources/Env are this side's config snapshot
// (store.DeployAttemptSnapshot, migrations/0086): for a real attempt it's
// that attempt's own historical snapshot; for the current side it's
// built fresh from the app's live DesiredService, so both sides carry
// the identical shape and diffDeployCompareSides/diffDeployCompareEnv
// work unchanged either way. An attempt recorded before migrations/0086
// shows a zero-value snapshot (Port 0, no Env/Domains/Resources), the
// same "unknown, not a real zero" caveat UnsnapshottedFields/Note has
// always carried for every other never-captured field.
type deployCompareSide struct {
	DeployID   string     `json:"deploy_id,omitempty"`
	IsCurrent  bool       `json:"is_current"`
	Image      string     `json:"image"`
	CommitSHA  string     `json:"commit_sha,omitempty"`
	Source     string     `json:"source,omitempty"`
	Status     string     `json:"status,omitempty"`
	StartedAt  *time.Time `json:"started_at,omitempty"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`

	Port      int                         `json:"port,omitempty"`
	HostPort  *int                        `json:"host_port,omitempty"`
	Domains   []string                    `json:"domains,omitempty"`
	Resources *store.ServiceResources     `json:"resources,omitempty"`
	Env       []store.DeployAttemptEnvKey `json:"env,omitempty"`
}

// deployCompareField is one field that differs between From and To.
type deployCompareField struct {
	Field string `json:"field"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// Deploy comparison env-change statuses. Never "changed" for a secret- or
// database-backed key (store.DeployAttemptEnvKindSecret/
// DeployAttemptEnvKindDatabase): this control plane has no way to detect
// a value change for either without decrypting a secret or re-resolving
// a live database reference, and diffDeployCompareEnv deliberately never
// tries, so such a key present on both sides is left out of the response
// entirely rather than reported as a possibly-wrong "unchanged".
const (
	deployCompareEnvAdded   = "added"
	deployCompareEnvRemoved = "removed"
	deployCompareEnvChanged = "changed"
)

// deployCompareEnvChange is one env var key that differs between From and
// To. From/To are only ever populated for Kind
// store.DeployAttemptEnvKindLiteral: a secret- or database-backed key's
// Value is never snapshotted in the first place (store.DeployAttemptEnvKey's
// own doc comment), so there is nothing to echo back here either.
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
	UnsnapshottedFields []string                 `json:"unsnapshotted_fields"`
	Note                string                   `json:"note"`
}

// unsnapshottedDeployFields lists DesiredService fields store.DeployAttempt
// still never captures a per-attempt copy of. Named once so the handler's
// response and its own doc comment can't drift apart.
var unsnapshottedDeployFields = []string{
	"health", "replicas", "strategy", "volumes", "labels",
}

const deployCompareUnsnapshottedNote = "Deploy attempts record image tag, commit, trigger source, outcome, environment variable keys, ports, domains, and resource limits at trigger time (attempts recorded before this was added show an empty snapshot for those fields). A secret- or database-backed env var's value is never recorded: only its key, and whether it was added or removed, ever appears in env_changes, since this control plane cannot know if such a value changed without decrypting it. Health checks, replica count, deploy strategy, volumes, and labels are still not snapshotted per attempt, so those cannot be diffed across past deploys, only the app's current live values are known for them."

// handleCompareDeploys handles
// GET /api/v1/apps/{name}/deploys/compare?from={deployId}&to={deployId}.
// from is required; to is optional and, when omitted, compares from
// against the app's current live desired state. See
// deployCompareUnsnapshottedNote for why the diff is limited to the
// fields store.DeployAttempt actually captures rather than a fabricated
// full config diff.
func (rt *Router) handleCompareDeploys(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	svc, err := rt.apps.GetDesiredService(r.Context(), name)
	if errors.Is(err, store.ErrServiceNotFound) {
		writeError(w, http.StatusNotFound, "app not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: compare deploys: load app failed", slog.String("error", err.Error()), slog.String("name", name))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	fromID := r.URL.Query().Get("from")
	toID := r.URL.Query().Get("to")
	if fromID == "" {
		writeError(w, http.StatusBadRequest, "from is required")
		return
	}

	fromSide, ok := rt.loadDeployCompareSide(w, r, name, fromID)
	if !ok {
		return
	}

	toSide := currentDeployCompareSide(*svc)
	if toID != "" {
		toSide, ok = rt.loadDeployCompareSide(w, r, name, toID)
		if !ok {
			return
		}
	}

	writeJSON(w, http.StatusOK, deployCompareResource{
		ServiceName:         name,
		From:                fromSide,
		To:                  toSide,
		Changes:             diffDeployCompareSides(fromSide, toSide),
		EnvChanges:          diffDeployCompareEnv(fromSide.Env, toSide.Env),
		UnsnapshottedFields: unsnapshottedDeployFields,
		Note:                deployCompareUnsnapshottedNote,
	})
}

// currentDeployCompareSide builds the "current live desired state" side
// from svc, the same snapshot shape a real deploy attempt carries
// (store.NewDeployAttemptSnapshot), so the from/to diff logic never needs
// to know which kind of side it's looking at.
func currentDeployCompareSide(svc store.DesiredService) deployCompareSide {
	snap := store.NewDeployAttemptSnapshot(svc)
	return deployCompareSide{
		IsCurrent: true, Image: svc.Image,
		Port: snap.Port, HostPort: snap.HostPort, Domains: snap.Domains,
		Resources: snap.Resources, Env: snap.Env,
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
		rt.logger.Error("api: compare deploys: load attempt failed", slog.String("error", err.Error()), slog.String("deploy_id", deployID))
		writeError(w, http.StatusInternalServerError, "internal error")
		return deployCompareSide{}, false
	}
	if a.ServiceName != appName {
		writeError(w, http.StatusNotFound, "deploy attempt not found: "+deployID)
		return deployCompareSide{}, false
	}
	return deployCompareSide{
		DeployID:   a.ID,
		Image:      a.Image,
		CommitSHA:  a.CommitSHA,
		Source:     a.Source,
		Status:     a.Status,
		StartedAt:  &a.StartedAt,
		FinishedAt: a.FinishedAt,
		Port:       a.Snapshot.Port,
		HostPort:   a.Snapshot.HostPort,
		Domains:    a.Snapshot.Domains,
		Resources:  a.Snapshot.Resources,
		Env:        a.Snapshot.Env,
	}, true
}

// diffDeployCompareSides lists the captured fields that actually differ
// between from and to. Source is only compared between two real attempts:
// the current side carries no Source, so including it there would read as
// "source changed to empty" rather than "unknown". Env is deliberately
// not in this list: it's key-based, not a single scalar, so it gets its
// own diffDeployCompareEnv/EnvChanges instead.
func diffDeployCompareSides(from, to deployCompareSide) []deployCompareField {
	var changes []deployCompareField
	if from.Image != to.Image {
		changes = append(changes, deployCompareField{Field: "image", From: from.Image, To: to.Image})
	}
	if from.CommitSHA != to.CommitSHA {
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
	if fromDomains, toDomains := strings.Join(from.Domains, ", "), strings.Join(to.Domains, ", "); fromDomains != toDomains {
		changes = append(changes, deployCompareField{Field: "domains", From: fromDomains, To: toDomains})
	}
	changes = append(changes, diffDeployCompareResources(from.Resources, to.Resources)...)
	return changes
}

// diffDeployCompareResources reports one field per store.ServiceResources
// sub-field that differs, the same shape internal/api/apps.go's resource
// limit fields already use on the wire (memory_bytes, nano_cpus,
// swap_memory_bytes, cpuset_cpus). A nil side (no limit configured) is
// treated as the type's zero value, matching how an absent limit already
// behaves everywhere else Resources is read.
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

// diffDeployCompareEnv reports every env key that was added, removed, or
// (for a literal key present on both sides) changed value. A secret- or
// database-backed key present on both sides is never reported: see
// deployCompareEnvChanged's own doc comment for why "unchanged" would be
// a guess this control plane isn't in a position to make.
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
			out = append(out, deployCompareEnvChange{Key: k, Kind: t.Kind, Status: deployCompareEnvChanged, From: f.Value, To: t.Value})
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
