package appsleep

import (
	"fmt"
	"net/http"
	"strings"
)

// Blocker and warning codes returned by Evaluate.
const (
	CodePinnedPort        = "pinned_port"
	CodePortInUse         = "port_wake_port_in_use"
	CodeDatabase          = "database"
	CodeNodeUnschedulable = "node_unschedulable"
	CodeCanaryTarget      = "canary_target"

	CodeNoDomain       = "no_domain"
	CodeDirectTraffic  = "direct_traffic_only"
	CodeBackgroundWork = "background_work_invisible"
	CodeTrial          = "trial_deployment"
	CodeReplicas       = "multiple_replicas"
	CodeDependency     = "dependency_of_other_apps"
	CodeStatefulImage  = "stateful_image"
	CodeLastWakeFailed = "last_wake_failed"
)

// Fix kinds a suggested fix can carry.
const (
	FixEnablePortWake    = "enable_port_wake"
	FixRemovePinnedPort  = "remove_pinned_port"
	FixSetHoldRequests   = "set_hold_requests"
	FixAttachDomain      = "attach_domain"
	fixMethodLinkOnly    = "GET"
	defaultFixIdleMinute = 15
)

// Fix is a one-click remedy: the API call that applies it, or a link when
// there is nothing to call.
type Fix struct {
	Kind   string         `json:"kind"`
	Label  string         `json:"label"`
	Method string         `json:"method"`
	Path   string         `json:"path"`
	Body   map[string]any `json:"body,omitempty"`
}

// Item is one reason an app cannot sleep (blocker) or may sleep badly (warning).
type Item struct {
	Code   string `json:"code"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Fix    *Fix   `json:"fix,omitempty"`
}

// Eligibility is the result of Evaluate. Blockers refuse enabling, warnings do not.
type Eligibility struct {
	Blockers []Item
	Warnings []Item
}

// Eligible reports whether sleep can be enabled.
func (e Eligibility) Eligible() bool { return len(e.Blockers) == 0 }

// Facts is everything Evaluate needs, gathered by the caller so the decision
// stays a pure function.
type Facts struct {
	App string
	// IdleMinutes is the current setting, 0 when off; fixes keep it.
	IdleMinutes int
	// PinnedPorts are the host ports the app pins; empty means none.
	PinnedPorts []int
	// PortWakeSupported says Levelrail can hold those ports while the app sleeps.
	PortWakeSupported bool
	// PortWakeUnsupportedReason explains why not, shown when it matters.
	PortWakeUnsupportedReason string
	// PortWakeEnabled is the per-app port_wake setting.
	PortWakeEnabled bool
	// PortWakeError is the last bind failure for the pinned ports, if any.
	PortWakeError string
	// IsManagedDatabase is true when the app name is a managed database.
	IsManagedDatabase bool
	// StatefulImage is true for a well known database image.
	StatefulImage bool
	// NodeUnschedulable is true when the app's node is cordoned or drained.
	NodeUnschedulable bool
	CanaryTarget      bool
	// DependedOnBy lists sibling apps that start after, and reach, this one.
	DependedOnBy []string
	HoldRequests bool
	HasDomain    bool
	HasStreams   bool
	IsTrial      bool
	Replicas     int
	// LastWakeError is the last failed wake, empty when the last wake worked.
	LastWakeError string
}

// Evaluate lists why app cannot sleep and what may surprise the operator.
func Evaluate(f Facts) Eligibility {
	var e Eligibility
	exposure := fmt.Sprintf("/api/v1/apps/%s/exposure", f.App)
	sleepPath := fmt.Sprintf("/api/v1/apps/%s/sleep", f.App)
	idle := f.IdleMinutes
	if idle <= 0 {
		idle = defaultFixIdleMinute
	}

	if len(f.PinnedPorts) > 0 {
		ports := joinInts(f.PinnedPorts)
		switch {
		case f.PortWakeError != "":
			e.Blockers = append(e.Blockers, Item{Code: CodePortInUse, Title: "Pinned port is held by something else",
				Detail: "Port wake could not take host port " + ports + " while the app sleeps: " + f.PortWakeError + ". Free the port or unpin it.",
				Fix:    &Fix{Kind: FixRemovePinnedPort, Label: "Stop pinning the host port", Method: http.MethodPut, Path: exposure, Body: map[string]any{"mode": "proxied"}}})
		case f.PortWakeSupported && f.PortWakeEnabled:
		case f.PortWakeSupported:
			e.Blockers = append(e.Blockers, Item{Code: CodePinnedPort, Title: "Pinned host port needs port wake",
				Detail: "Traffic on host port " + ports + " bypasses the ingress, so it could not wake the app. Turn on port wake so Levelrail holds the port while the app sleeps.",
				Fix:    &Fix{Kind: FixEnablePortWake, Label: "Turn on port wake", Method: http.MethodPut, Path: sleepPath, Body: map[string]any{"idle_minutes": idle, "port_wake": true}}})
		default:
			why := f.PortWakeUnsupportedReason
			if why == "" {
				why = "port wake is not available for this app"
			}
			e.Blockers = append(e.Blockers, Item{Code: CodePinnedPort, Title: "Pinned host port cannot wake the app",
				Detail: "Traffic on host port " + ports + " bypasses the ingress, so it could not wake the app, and " + why + ". Serve the app through a domain instead.",
				Fix:    &Fix{Kind: FixRemovePinnedPort, Label: "Stop pinning the host port", Method: http.MethodPut, Path: exposure, Body: map[string]any{"mode": "proxied"}}})
		}
	}
	if f.IsManagedDatabase {
		e.Blockers = append(e.Blockers, Item{Code: CodeDatabase, Title: "Databases never sleep",
			Detail: "A managed database holds state other apps depend on, so stopping it when idle risks lost writes and failed connections."})
	}
	if f.NodeUnschedulable {
		e.Blockers = append(e.Blockers, Item{Code: CodeNodeUnschedulable, Title: "The app's server is cordoned or drained",
			Detail: "A sleeping app could not be started again on a server that accepts no new work. Uncordon the server first."})
	}
	if f.CanaryTarget {
		e.Blockers = append(e.Blockers, Item{Code: CodeCanaryTarget, Title: "A canary release is running",
			Detail: "Sleeping would stop one side of the canary and skew the result. Finish or cancel the canary first."})
	}

	if len(f.DependedOnBy) > 0 && !f.HoldRequests {
		e.Warnings = append(e.Warnings, Item{Code: CodeDependency, Title: "Other apps reach this one internally",
			Detail: "Requests from " + strings.Join(f.DependedOnBy, ", ") + " over the internal network do not pass the ingress, so they would not wake it. Hold requests makes the first request wait for the wake.",
			Fix:    &Fix{Kind: FixSetHoldRequests, Label: "Hold requests while waking", Method: http.MethodPut, Path: sleepPath, Body: map[string]any{"idle_minutes": idle, "hold_requests": true}}})
	}
	if !f.HasDomain {
		e.Warnings = append(e.Warnings, Item{Code: CodeNoDomain, Title: "No domain attached",
			Detail: "Without a domain no ingress request is ever seen, so only network activity keeps the app awake and nothing wakes it by request.",
			Fix:    &Fix{Kind: FixAttachDomain, Label: "Attach a domain", Method: fixMethodLinkOnly, Path: "/apps/" + f.App + "/domains"}})
	}
	if len(f.PinnedPorts) > 0 || f.HasStreams {
		e.Warnings = append(e.Warnings, Item{Code: CodeDirectTraffic, Title: "Direct port or TCP traffic",
			Detail: "Connections that skip the ingress are only noticed through network activity and open connections, so a quiet but open client can be missed."})
	}
	e.Warnings = append(e.Warnings, Item{Code: CodeBackgroundWork, Title: "Scheduled jobs and workers are invisible",
		Detail: "Cron jobs, queue consumers and background work make no requests. Only their network traffic counts, so a job that works offline can be stopped mid run."})
	if f.IsTrial {
		e.Warnings = append(e.Warnings, Item{Code: CodeTrial, Title: "Trial deployment",
			Detail: "A trial app may be removed or replaced soon; sleeping it saves little."})
	}
	if f.Replicas > 1 {
		e.Warnings = append(e.Warnings, Item{Code: CodeReplicas, Title: "More than one replica",
			Detail: fmt.Sprintf("All %d replicas stop together and start together, which makes the first request after a sleep slower.", f.Replicas)})
	}
	if f.StatefulImage && !f.IsManagedDatabase {
		e.Warnings = append(e.Warnings, Item{Code: CodeStatefulImage, Title: "Looks like a database",
			Detail: "This image is a known database engine. Stopping it when idle can drop connections other apps keep open."})
	}
	if f.LastWakeError != "" {
		e.Warnings = append(e.Warnings, Item{Code: CodeLastWakeFailed, Title: "The last wake failed",
			Detail: f.LastWakeError})
	}
	return e
}

func joinInts(v []int) string {
	parts := make([]string, len(v))
	for i, n := range v {
		parts[i] = fmt.Sprint(n)
	}
	return strings.Join(parts, ", ")
}
