// Package takeover moves a server's ports 80 and 443 from the proxy of the
// platform it is migrating off (Coolify's Traefik, a Caddy or nginx container)
// to this control plane's own ingress, with a recorded way back. It only acts
// through the Docker Engine API wrappers, the settings store and the existing
// restart: there is no host shell and no file writer here by construction.
package takeover

import "time"

// Run states.
const (
	StatePreparing   = "preparing"
	StateApplying    = "applying"
	StateVerifying   = "verifying"
	StateDone        = "done"
	StateRollingBack = "rolling_back"
	StateRolledBack  = "rolled_back"
	StateFailed      = "failed"
)

// Check statuses.
const (
	StatusPass  = "pass"
	StatusWarn  = "warn"
	StatusBlock = "block"
)

// Plan verdicts.
const (
	VerdictReady   = "ready"
	VerdictWarn    = "warnings"
	VerdictBlocked = "blocked"
	// VerdictDone means this server already listens on the target ports.
	VerdictDone = "done"
)

// Step states.
const (
	StepPending = "pending"
	StepRunning = "running"
	StepDone    = "done"
	StepFailed  = "failed"
	StepSkipped = "skipped"
)

// Step ids, in plan order.
const (
	StepStopProxy = "stop_old_proxy"
	StepTLS       = "tls_settings"
	StepListen    = "listen_addresses"
	StepRestart   = "restart"
	StepVerify    = "verify"
	StepRollback  = "rollback_record"
	StepRestore   = "rollback"
)

// Check ids.
const (
	CheckPorts      = "ports"
	CheckHealth     = "apps_healthy"
	CheckDNS        = "dns"
	CheckCerts      = "certificates"
	CheckACME       = "acme"
	CheckConflict   = "duplicate_domain"
	CheckUnmanaged  = "unmanaged_domains"
	CheckResources  = "resources"
	CheckRestart    = "restart"
	CheckDocker     = "docker"
	CheckSnippet    = "saved_not_enforced"
	CheckPlatform   = "platform_restarts_proxy"
	CheckRestartPol = "restart_policy"
	CheckSocket     = "socket_activation"
)

// Fix action kinds.
const (
	ActionRequest = "request"
	ActionLink    = "link"
	ActionCopy    = "copy"
)

// Action is the one primary thing a fix offers.
type Action struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	API   string `json:"api,omitempty"`
	Value string `json:"value,omitempty"`
}

// Fix is the remedy for a warning or blocking check.
type Fix struct {
	Summary string  `json:"summary"`
	Action  *Action `json:"action,omitempty"`
}

// Check is one preflight item.
type Check struct {
	ID     string   `json:"id"`
	Title  string   `json:"title"`
	Status string   `json:"status"`
	Detail string   `json:"detail,omitempty"`
	Items  []string `json:"items,omitempty"`
	Fix    *Fix     `json:"fix,omitempty"`
}

// PlanStep is one thing a run does, in order.
type PlanStep struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	// Reversible is false only for steps rollback cannot undo on its own.
	Reversible bool `json:"reversible"`
}

// Plan is the preflight result plus the exact steps a run would take.
type Plan struct {
	Verdict string      `json:"verdict"`
	Checks  []Check     `json:"checks"`
	Steps   []PlanStep  `json:"steps"`
	Domains []DomainRow `json:"domains"`
	// Rollback says what the undo button restores.
	Rollback      []string `json:"rollback"`
	WindowSeconds int      `json:"window_seconds"`
	TargetHTTP    string   `json:"target_http"`
	TargetHTTPS   string   `json:"target_https"`
	CurrentHTTP   string   `json:"current_http"`
	CurrentHTTPS  string   `json:"current_https"`
	ProxyKind     string   `json:"proxy_kind,omitempty"`
	ProxyNames    []string `json:"proxy_containers"`
	CheckedAt     string   `json:"checked_at"`
}

// DomainRow is one domain the takeover has to keep serving.
type DomainRow struct {
	Domain  string `json:"domain"`
	App     string `json:"app"`
	Healthy bool   `json:"healthy"`
	Detail  string `json:"detail,omitempty"`
	DNS     string `json:"dns"`
	Cert    string `json:"certificate"`
}

// Step is one timed entry of a run.
type Step struct {
	ID         string    `json:"id"`
	Title      string    `json:"title"`
	State      string    `json:"state"`
	Detail     string    `json:"detail,omitempty"`
	StartedAt  time.Time `json:"started_at,omitzero"`
	FinishedAt time.Time `json:"finished_at,omitzero"`
}

// Proxy is one container the run stopped, so rollback starts exactly it.
type Proxy struct {
	Name       string `json:"name"`
	Image      string `json:"image"`
	WasRunning bool   `json:"was_running"`
	Stopped    bool   `json:"stopped"`
	Restored   bool   `json:"restored"`
}

// Previous is everything rollback restores. It holds no secrets.
type Previous struct {
	HTTPAddr              string  `json:"http_addr"`
	HTTPSAddr             string  `json:"https_addr"`
	TLSTerminatedUpstream bool    `json:"tls_terminated_upstream"`
	PublicHTTPSPort       int     `json:"public_https_port"`
	SettingsChanged       bool    `json:"settings_changed"`
	Proxies               []Proxy `json:"proxies"`
}

// DomainCheck is the outcome of verifying one domain over real HTTPS.
type DomainCheck struct {
	Domain    string `json:"domain"`
	OK        bool   `json:"ok"`
	Status    int    `json:"status,omitempty"`
	Delivered bool   `json:"delivered"`
	Detail    string `json:"detail,omitempty"`
}

// Run is one takeover and, when it fails or is undone, its rollback.
type Run struct {
	ID            string        `json:"id"`
	State         string        `json:"state"`
	Actor         string        `json:"actor,omitempty"`
	Previous      Previous      `json:"previous"`
	TargetHTTP    string        `json:"target_http"`
	TargetHTTPS   string        `json:"target_https"`
	Steps         []Step        `json:"steps"`
	Verify        []DomainCheck `json:"verify"`
	Error         string        `json:"error,omitempty"`
	Boots         int           `json:"boots"`
	WindowSeconds int           `json:"window_seconds"`
	DeadlineAt    time.Time     `json:"deadline_at,omitzero"`
	CreatedAt     time.Time     `json:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at"`
	FinishedAt    time.Time     `json:"finished_at,omitzero"`
}

// InFlight reports whether the run is still moving by itself.
func (r Run) InFlight() bool {
	switch r.State {
	case StatePreparing, StateApplying, StateVerifying, StateRollingBack:
		return true
	}
	return false
}

// Rollbackable reports whether the undo button still does something.
func (r Run) Rollbackable() bool {
	switch r.State {
	case StatePreparing, StateApplying, StateVerifying, StateDone:
		return true
	}
	return false
}

// Active reports whether the run's addresses are the ones this process must
// listen on at start.
func (r Run) Active() bool {
	switch r.State {
	case StateApplying, StateVerifying, StateDone:
		return true
	}
	return false
}
