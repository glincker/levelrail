package api

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/sshprovision"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/version"
)

// sshNodeProvisionJoinTokenTTL mirrors nodeProvisionJoinTokenTTL's own
// reasoning: nobody is watching a background install goroutine the way
// an operator watches the manual copy-paste flow, so this needs more
// headroom than nodeJoinTokenTTL's 15 minutes. An SSH install has no
// provider boot queue to wait through, so it needs materially less than
// the cloud path's 60 minutes.
const sshNodeProvisionJoinTokenTTL = 30 * time.Minute

// sshNodeProvisionTimeoutEnv bounds both the provisioning goroutine's own
// context (sshprovision.Provisioner.Provision aborts once it elapses) and
// how long handleGetSSHNodeProvision waits for enrollment before
// reporting failed, the same dual role nodeProvisionTimeoutEnv plays for
// the cloud path.
const sshNodeProvisionTimeoutEnv = "APP_SSH_NODE_PROVISION_TIMEOUT"

const defaultSSHNodeProvisionTimeout = 10 * time.Minute

func sshNodeProvisionTimeout() time.Duration {
	return envDurationOr(sshNodeProvisionTimeoutEnv, defaultSSHNodeProvisionTimeout)
}

// SSHProvisioner is the surface *sshprovision.Provisioner satisfies,
// narrowed to what this package calls: a seam for this package's own
// tests to inject a fake that never dials a real network, the same
// "consumer-defined interface" shape NodeProvisioner already establishes
// for the cloud path.
type SSHProvisioner interface {
	Provision(ctx context.Context, creds sshprovision.Credentials, params sshprovision.InstallParams, onEvent func(sshprovision.Event)) (sshprovision.DetectedHost, error)
}

// SSHNodeProvisionStore is the store surface the SSH node provisioning
// handlers need.
type SSHNodeProvisionStore interface {
	SaveSSHNodeProvision(ctx context.Context, p store.SSHNodeProvision) error
	GetSSHNodeProvision(ctx context.Context, id string) (store.SSHNodeProvision, error)
	ListSSHNodeProvisions(ctx context.Context) ([]store.SSHNodeProvision, error)
	UpdateSSHNodeProvisionProgress(ctx context.Context, id, status, detectedOS, detectedArch, log, nodeID, failureReason string, updatedAt time.Time) error
}

// sshNodeProvisionResource is the wire shape for an SSH node provision.
// Deliberately no host/username/auth fields: the credential is never
// stored past the one provisioning call, so there is nothing to echo
// back even on the create response.
type sshNodeProvisionResource struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Role          string    `json:"role"`
	Status        string    `json:"status"`
	DetectedOS    string    `json:"detected_os,omitempty"`
	DetectedArch  string    `json:"detected_arch,omitempty"`
	NodeID        string    `json:"node_id,omitempty"`
	FailureReason string    `json:"failure_reason,omitempty"`
	Log           string    `json:"log,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func toSSHNodeProvisionResource(p store.SSHNodeProvision) sshNodeProvisionResource {
	return sshNodeProvisionResource{
		ID: p.ID, Name: p.Name, Role: p.Role, Status: p.Status,
		DetectedOS: p.DetectedOS, DetectedArch: p.DetectedArch, NodeID: p.NodeID,
		FailureReason: p.FailureReason, Log: p.Log,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

// createSSHNodeProvisionAuthRequest is one of two credential shapes: a
// private key (optionally passphrase-protected) or a password. Neither
// is ever written to ssh_node_provisions or logged; both are held only
// for the lifetime of the background provisioning goroutine this
// request starts.
type createSSHNodeProvisionAuthRequest struct {
	Type       string `json:"type"` // "key" or "password"
	PrivateKey string `json:"private_key,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	Password   string `json:"password,omitempty"`
}

type createSSHNodeProvisionRequest struct {
	Host     string                            `json:"host"`
	Port     int                               `json:"port,omitempty"`
	Username string                            `json:"username"`
	Auth     createSSHNodeProvisionAuthRequest `json:"auth"`
	Name     string                            `json:"name"`
	// Role is general or build. Empty defaults to general.
	Role string `json:"role,omitempty"`
	// ControlPlaneAddr is the host:port the adopted machine dials to
	// reach this control plane's agent gRPC listener, the same field
	// createNodeProvisionRequest already requires for the same reason
	// (see its own doc comment).
	ControlPlaneAddr string `json:"control_plane_addr"`
}

// credentials builds the sshprovision.Credentials this request describes,
// or an operator-facing validation message.
func (req createSSHNodeProvisionRequest) credentials() (sshprovision.Credentials, string) {
	creds := sshprovision.Credentials{Host: req.Host, Port: req.Port, Username: req.Username}
	switch req.Auth.Type {
	case string(sshprovision.AuthKey):
		if req.Auth.PrivateKey == "" {
			return sshprovision.Credentials{}, "auth.private_key is required for key auth"
		}
		creds.Auth = sshprovision.AuthKey
		creds.PrivateKey = req.Auth.PrivateKey
		creds.Passphrase = req.Auth.Passphrase
	case string(sshprovision.AuthPassword):
		if req.Auth.Password == "" {
			return sshprovision.Credentials{}, "auth.password is required for password auth"
		}
		creds.Auth = sshprovision.AuthPassword
		creds.Password = req.Auth.Password
	default:
		return sshprovision.Credentials{}, `auth.type must be "key" or "password"`
	}
	return creds, ""
}

// handleCreateSSHNodeProvision handles POST /api/v1/nodes/ssh-provision:
// adopts a machine the operator already has by dialing it over SSH and
// installing the node agent there, the SSH-driven counterpart to
// handleCreateNodeProvision's cloud-VM path. Mints a join token through
// the same code path both other enrollment routes use, records an
// ssh_node_provisions row, and returns immediately (201): the actual SSH
// session runs in a background goroutine a caller polls via
// handleGetSSHNodeProvision, the same "create returns fast, poll for
// progress" shape handleCreateNodeProvision already establishes.
func (rt *Router) handleCreateSSHNodeProvision(w http.ResponseWriter, r *http.Request) {
	var req createSSHNodeProvisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.Host == "" {
		writeError(w, http.StatusBadRequest, "host is required")
		return
	}
	if req.Username == "" {
		writeError(w, http.StatusBadRequest, "username is required")
		return
	}
	if !controlPlaneAddrRe.MatchString(req.ControlPlaneAddr) {
		writeError(w, http.StatusBadRequest, "control_plane_addr must be host:port")
		return
	}
	if !nodeProvisionNameRe.MatchString(req.Name) {
		writeError(w, http.StatusBadRequest, "name must start with a lowercase letter and contain only lowercase letters, digits and hyphens")
		return
	}
	role, msg := normalizeNodeRole(req.Role)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	creds, msg := req.credentials()
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}

	if taken, err := rt.nodeNameTaken(r.Context(), req.Name); err != nil {
		rt.logger.Error("api: ssh node provision: check name collision failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	} else if taken {
		writeError(w, http.StatusConflict, "a node or an in-progress provision already uses this name")
		return
	}

	token, err := rt.mintNodeJoinToken(r.Context(), sshNodeProvisionJoinTokenTTL)
	if err != nil {
		rt.logger.Error("api: ssh node provision: mint join token failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	id, err := randomSSHNodeProvisionID()
	if err != nil {
		rt.logger.Error("api: ssh node provision: generate id failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	now := time.Now()
	rec := store.SSHNodeProvision{
		ID: id, Name: req.Name, Role: role,
		Status: store.SSHNodeProvisionStatusConnecting, CreatedAt: now, UpdatedAt: now,
	}
	if err := rt.sshProvisions.SaveSSHNodeProvision(r.Context(), rec); err != nil {
		rt.logger.Error("api: ssh node provision: save failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	params := sshprovision.InstallParams{
		ControlPlaneAddr: req.ControlPlaneAddr,
		JoinToken:        token.plaintext,
		CAFingerprint:    rt.agentCAFingerprint,
		NodeName:         req.Name,
		AgentImage:       sshProvisionAgentImage(),
		MeshEnabled:      rt.mesh != nil,
	}
	go rt.runSSHNodeProvision(id, creds, params) //nolint:gosec // deliberate: the request context is gone by the time this goroutine finishes, runSSHNodeProvision derives its own bounded context (sshNodeProvisionTimeout) instead

	rt.logger.Info("api: ssh node provision created", slog.String("provision_id", id), slog.String("name", req.Name))
	writeJSON(w, http.StatusCreated, toSSHNodeProvisionResource(rec))
}

// runSSHNodeProvision runs the SSH install in the background: creds live
// only on this goroutine's stack for its own lifetime, never reaching
// the store or a log line. Bounded by sshNodeProvisionTimeout via its own
// context rather than the request's (which is already gone by the time
// this runs), so a stuck remote command can't leak the goroutine
// forever.
func (rt *Router) runSSHNodeProvision(id string, creds sshprovision.Credentials, params sshprovision.InstallParams) {
	ctx, cancel := context.WithTimeout(context.Background(), sshNodeProvisionTimeout())
	defer cancel()

	var log logAccumulator
	provisioner := rt.sshProvisioner
	if provisioner == nil {
		provisioner = sshprovision.New()
	}

	host, err := provisioner.Provision(ctx, creds, params, func(e sshprovision.Event) {
		log.add(string(e.Step) + ": " + e.Message)
		status := store.SSHNodeProvisionStatusInstalling
		switch e.Step {
		case sshprovision.StepConnect:
			status = store.SSHNodeProvisionStatusConnecting
		case sshprovision.StepDetect, sshprovision.StepPrereqs:
			status = store.SSHNodeProvisionStatusDetecting
		}
		// StepDone stays Installing here (no DetectedHost yet); Enrolling
		// is set exactly once below, with the real host, to avoid a
		// window where a reader sees Enrolling with blank detected_os/arch.
		rt.updateSSHNodeProvision(ctx, id, status, "", "", log.String(), "", "")
	})

	if err != nil {
		log.add("failed: " + err.Error())
		rt.updateSSHNodeProvision(context.Background(), id, store.SSHNodeProvisionStatusFailed, host.OS, host.Arch, log.String(), "", err.Error())
		return
	}
	rt.updateSSHNodeProvision(context.Background(), id, store.SSHNodeProvisionStatusEnrolling, host.OS, host.Arch, log.String(), "", "")
}

// updateSSHNodeProvision persists a progress write, logging (not
// panicking or blocking the goroutine on) a failure: this runs from a
// background goroutine with nothing else to report a store error to.
func (rt *Router) updateSSHNodeProvision(ctx context.Context, id, status, detectedOS, detectedArch, log, nodeID, failureReason string) {
	if err := rt.sshProvisions.UpdateSSHNodeProvisionProgress(ctx, id, status, detectedOS, detectedArch, log, nodeID, failureReason, time.Now()); err != nil {
		rt.logger.Error("api: ssh node provision: update progress failed", slog.String("provision_id", id), slog.String("error", err.Error()))
	}
}

// logAccumulator collects sshprovision.Event messages into one
// newline-joined log a store row's Log column holds in full each write
// (store.SSHNodeProvision.Log's own doc comment explains why replace,
// not append, is safe here): the provisioning goroutine is this value's
// only writer for the whole request, so no mutex is needed either.
type logAccumulator struct {
	lines []string
}

func (l *logAccumulator) add(line string) { l.lines = append(l.lines, line) }

func (l *logAccumulator) String() string {
	out := ""
	for i, line := range l.lines {
		if i > 0 {
			out += "\n"
		}
		out += line
	}
	return out
}

// sshProvisionAgentImage mirrors provision.CloudInitParams.agentImage's
// own fallback logic (internal/provision/cloudinit.go): that package is
// cloud-VM creation, a different feature internal/sshprovision must not
// import from (see that package's doc comment), so this small piece of
// version-to-tag logic is duplicated here rather than shared.
func sshProvisionAgentImage() string {
	tag := version.Version
	if tag == "" || tag == "dev" {
		tag = "edge"
	}
	return "ghcr.io/glincker/levelrail-agent:" + tag
}

// handleListSSHNodeProvisions handles GET /api/v1/nodes/ssh-provisions:
// every SSH provision, last known status, the same "not live-refreshed,
// that's the show route's job" shape handleListNodeProvisions
// establishes for the cloud path.
func (rt *Router) handleListSSHNodeProvisions(w http.ResponseWriter, r *http.Request) {
	provisions, err := rt.sshProvisions.ListSSHNodeProvisions(r.Context())
	if err != nil {
		rt.logger.Error("api: list ssh node provisions failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := make([]sshNodeProvisionResource, 0, len(provisions))
	for _, p := range provisions {
		out = append(out, toSSHNodeProvisionResource(p))
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetSSHNodeProvision handles GET /api/v1/nodes/ssh-provisions/{id}:
// checks the node registry for the expected enrollment by name before
// returning, the same live-refresh shape handleGetNodeProvision uses,
// simplified since there is no provider API to also poll here: the
// install's own status/log is already kept current by the background
// goroutine's own writes (runSSHNodeProvision), this only adds the
// "has it actually enrolled yet" check that goroutine has no way to see.
func (rt *Router) handleGetSSHNodeProvision(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	p, err := rt.sshProvisions.GetSSHNodeProvision(r.Context(), id)
	if errors.Is(err, store.ErrSSHNodeProvisionNotFound) {
		writeError(w, http.StatusNotFound, "ssh node provision not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get ssh node provision failed", slog.String("provision_id", id), slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	if p.Status == store.SSHNodeProvisionStatusReady || p.Status == store.SSHNodeProvisionStatusFailed {
		writeJSON(w, http.StatusOK, toSSHNodeProvisionResource(p))
		return
	}

	updated := rt.refreshSSHNodeProvision(r.Context(), p)
	writeJSON(w, http.StatusOK, toSSHNodeProvisionResource(updated))
}

// refreshSSHNodeProvision checks whether a node named p.Name has
// enrolled yet, applying p.Role the same way refreshNodeProvision does
// (enrollment itself always starts a node as a plain app node, with no
// way to carry an operator's chosen role through the join-token
// exchange). Past sshNodeProvisionTimeout with no match, marks failed;
// otherwise returns p unchanged (the background goroutine's own writes
// already keep its status/log current while it's still running).
func (rt *Router) refreshSSHNodeProvision(ctx context.Context, p store.SSHNodeProvision) store.SSHNodeProvision {
	nodes, err := rt.nodes.ListNodes(ctx)
	if err != nil {
		rt.logger.Warn("api: ssh node provision: list nodes failed", slog.String("provision_id", p.ID), slog.String("error", err.Error()))
		return p
	}
	if n, ok := findNodeByName(nodes, p.Name); ok {
		acceptsApp, acceptsBuild := p.Role != "build", p.Role == "build"
		if werr := rt.nodes.UpdateNodeWorkloads(ctx, n.ID, acceptsApp, acceptsBuild); werr != nil {
			rt.logger.Error("api: ssh node provision: apply role to node failed", slog.String("provision_id", p.ID), slog.String("node_id", n.ID), slog.String("error", werr.Error()))
		}
		return rt.saveSSHNodeProvisionUpdate(ctx, p, store.SSHNodeProvisionStatusReady, n.ID, p.FailureReason)
	}

	if p.Status != store.SSHNodeProvisionStatusFailed && time.Since(p.CreatedAt) > sshNodeProvisionTimeout() {
		return rt.saveSSHNodeProvisionUpdate(ctx, p, store.SSHNodeProvisionStatusFailed, "", "timed out waiting for the node to enroll")
	}
	return p
}

func (rt *Router) saveSSHNodeProvisionUpdate(ctx context.Context, p store.SSHNodeProvision, status, nodeID, failureReason string) store.SSHNodeProvision {
	if status == p.Status && nodeID == p.NodeID && failureReason == p.FailureReason {
		return p
	}
	now := time.Now()
	if err := rt.sshProvisions.UpdateSSHNodeProvisionProgress(ctx, p.ID, status, p.DetectedOS, p.DetectedArch, p.Log, nodeID, failureReason, now); err != nil {
		rt.logger.Error("api: ssh node provision: update status failed", slog.String("provision_id", p.ID), slog.String("error", err.Error()))
		return p
	}
	p.Status, p.NodeID, p.FailureReason, p.UpdatedAt = status, nodeID, failureReason, now
	return p
}

// findNodeByName returns the first node in nodes named name. Both
// NodeProvision and SSHNodeProvision correlate to the real store.Node
// they produce purely by Name (their own doc comments explain why), so
// both live-enrollment checks (this file's refreshSSHNodeProvision and
// node_provision.go's refreshNodeProvision) share this lookup.
func findNodeByName(nodes []store.Node, name string) (store.Node, bool) {
	for _, n := range nodes {
		if n.Name == name {
			return n, true
		}
	}
	return store.Node{}, false
}

// randomSSHNodeProvisionID mirrors randomNodeProvisionID's exact shape.
func randomSSHNodeProvisionID() (string, error) {
	buf := make([]byte, 9)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("api: generate ssh node provision id: %w", err)
	}
	return "sshp_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
