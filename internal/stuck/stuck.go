// Package stuck explains why a reconciled resource is not ready, by mapping
// condition reasons plus nearby context to a plain-language cause, evidence
// lines and typed next actions. Pure and deterministic: no AI, no I/O.
package stuck

import (
	"sort"
	"strings"
	"time"
)

// Resource kinds Explain understands.
const (
	KindApp      = "app"
	KindDatabase = "database"
	KindDomain   = "domain"
	KindNode     = "node"
)

// Action types a Cause can carry.
const (
	ActionFix         = "fix"
	ActionRecheck     = "recheck"
	ActionLogs        = "logs"
	ActionRetryDeploy = "retry_deploy"
	ActionLink        = "link"
)

// Confidence values.
const (
	ConfidenceHigh   = "high"
	ConfidenceMedium = "medium"
)

// Cause codes.
const (
	CauseDiskFull        = "DISK_FULL"
	CauseNodeOffline     = "NODE_OFFLINE"
	CauseNodePending     = "NODE_PENDING"
	CauseImagePull       = "IMAGE_PULL_FAILED"
	CausePortConflict    = "PORT_CONFLICT"
	CauseReadiness       = "READINESS_FAILING"
	CauseCrashloop       = "CRASHLOOP"
	CauseMissingSecret   = "MISSING_SECRET"
	CauseDependency      = "DEPENDENCY_NOT_READY"
	CausePlacement       = "PLACEMENT_IMPOSSIBLE"
	CauseOperationFailed = "OPERATION_FAILED"
	CauseNotReconciled   = "NOT_RECONCILED_YET"
)

const diskReclaimFixID = "disk.reclaim"

// Condition is one reconcile condition with the time it entered its state.
type Condition struct {
	Type    string    `json:"type"`
	Status  string    `json:"status"`
	Reason  string    `json:"reason"`
	Message string    `json:"message,omitempty"`
	Since   time.Time `json:"since"`
}

// Context is everything beyond the conditions that can explain a stall.
type Context struct {
	LastError          string
	RecentEvents       []string
	NodeStatus         string
	DiskCritical       bool
	DiskDetail         string
	Crashloop          bool
	DependencyNotReady string
	// Href maps a destination name (logs, node, environment, registries) to a dashboard path.
	Href map[string]string
}

// Input is one resource to explain.
type Input struct {
	Kind, Name string
	Conditions []Condition
	Ctx        Context
	Now        time.Time
	Threshold  time.Duration
}

// Action is one typed next step.
type Action struct {
	Type  string `json:"type"`
	Label string `json:"label"`
	FixID string `json:"fix_id,omitempty"`
	Href  string `json:"href,omitempty"`
}

// Cause is one ranked likely reason with its supporting evidence.
type Cause struct {
	Code        string   `json:"code"`
	Title       string   `json:"title"`
	Explanation string   `json:"explanation"`
	Confidence  string   `json:"confidence"`
	Evidence    []string `json:"evidence"`
	Actions     []Action `json:"actions"`
}

// Report is the full explanation.
type Report struct {
	Kind            string      `json:"kind"`
	Name            string      `json:"name"`
	Ready           bool        `json:"ready"`
	Stuck           bool        `json:"stuck"`
	NotReadySince   *time.Time  `json:"not_ready_since,omitempty"`
	NotReadySeconds int64       `json:"not_ready_seconds"`
	ThresholdSecs   int64       `json:"threshold_seconds"`
	Timeline        []Condition `json:"timeline"`
	Causes          []Cause     `json:"causes"`
}

type row struct {
	code        string
	title       string
	explanation string
	reasons     []string
	patterns    []string
	ctxMatch    func(Context) (string, bool)
	actions     func(in Input) []Action
}

func href(in Input, key, fallback string) string {
	if h := in.Ctx.Href[key]; h != "" {
		return h
	}
	return fallback
}

func recheck() Action { return Action{Type: ActionRecheck, Label: "Re-check"} }

func logsAction(in Input) []Action {
	if h := in.Ctx.Href["logs"]; h != "" {
		return []Action{{Type: ActionLogs, Label: "Open logs", Href: h}}
	}
	return nil
}

func retryApp(in Input) []Action {
	if in.Kind == KindApp {
		return []Action{{Type: ActionRetryDeploy, Label: "Retry deploy"}}
	}
	return nil
}

func join(parts ...[]Action) []Action {
	var out []Action
	for _, p := range parts {
		out = append(out, p...)
	}
	return out
}

func nodeActions(in Input) []Action {
	return []Action{{Type: ActionLink, Label: "Open node", Href: href(in, "node", "/nodes")}, recheck()}
}

func logsRetryRecheck(in Input) []Action {
	return join(logsAction(in), retryApp(in), []Action{recheck()})
}

// table is ordered most fundamental first; ties in confidence keep this order.
var table = []row{
	{
		code: CauseDiskFull, title: "The host is out of disk space",
		explanation: "The disk guard is refusing new pulls and builds, so nothing can start until space is freed.",
		ctxMatch:    func(c Context) (string, bool) { return "disk guard: " + c.DiskDetail, c.DiskCritical },
		patterns:    []string{"no space left on device"},
		actions: func(Input) []Action {
			return []Action{{Type: ActionFix, Label: "Free space", FixID: diskReclaimFixID}, recheck()}
		},
	},
	{
		code: CauseNodeOffline, title: "The node running this is offline",
		explanation: "The node stopped sending heartbeats, so the control plane cannot change anything running on it.",
		reasons:     []string{"HeartbeatStale", "NodeOffline", "NodeUnreachable"},
		ctxMatch:    func(c Context) (string, bool) { return "node status: " + c.NodeStatus, c.NodeStatus == "offline" },
		actions:     nodeActions,
	},
	{
		code: CauseNodePending, title: "The node has not finished joining",
		explanation: "The node was enrolled but has not sent its first heartbeat yet.",
		ctxMatch:    func(c Context) (string, bool) { return "node status: " + c.NodeStatus, c.NodeStatus == "pending" },
		actions:     nodeActions,
	},
	{
		code: CausePlacement, title: "No node can run this",
		explanation: "Placement found no schedulable node that satisfies this resource's constraints.",
		reasons:     []string{"PlacementFailed", "NoSchedulableNode", "Unschedulable"},
		patterns:    []string{"no schedulable node", "no eligible node", "no node satisfies", "cannot place"},
		actions: func(Input) []Action {
			return []Action{{Type: ActionLink, Label: "Open nodes", Href: "/nodes"}, recheck()}
		},
	},
	{
		code: CauseImagePull, title: "The image could not be pulled",
		explanation: "Docker could not fetch the image: the name or tag is wrong, it was never pushed, or registry credentials are missing.",
		reasons:     []string{"ImagePullFailed", "RegistryAuthDenied", "CredentialsNotConfigured"},
		patterns:    []string{"pull access denied", "manifest unknown", "no such image", "toomanyrequests", "unauthorized: authentication required"},
		actions: func(in Input) []Action {
			link := []Action{{Type: ActionLink, Label: "Check registry credentials", Href: href(in, "registries", "/settings")}}
			return join(link, retryApp(in), []Action{recheck()})
		},
	},
	{
		code: CausePortConflict, title: "The port is already in use",
		explanation: "Another container or process on the host already holds the port this resource needs.",
		patterns:    []string{"port is already allocated", "address already in use"},
		actions:     func(in Input) []Action { return join(retryApp(in), []Action{recheck()}) },
	},
	{
		code: CauseMissingSecret, title: "A required secret or setting is missing",
		explanation: "The workload needs a value that is not set, so it exits or never starts.",
		reasons:     []string{"MissingSecret", "SecretNotFound"},
		patterns:    []string{"required env", "missing required", "secret not found", "is required but"},
		actions: func(in Input) []Action {
			var link []Action
			if h := in.Ctx.Href["environment"]; h != "" {
				link = []Action{{Type: ActionLink, Label: "Edit environment", Href: h}}
			}
			return join(link, retryApp(in), []Action{recheck()})
		},
	},
	{
		code: CauseDependency, title: "A dependency is not ready",
		explanation: "This resource waits on another resource that is not ready yet.",
		reasons:     []string{"DependencyNotReady", "WaitingForDependency"},
		ctxMatch: func(c Context) (string, bool) {
			return "waiting on " + c.DependencyNotReady, c.DependencyNotReady != ""
		},
		actions: func(Input) []Action { return []Action{recheck()} },
	},
	{
		code: CauseCrashloop, title: "The container keeps crashing",
		explanation: "It starts, exits, and is restarted over and over, so it never stays up long enough to be ready.",
		reasons:     []string{"CrashLoop", "CrashLoopBackOff"},
		ctxMatch:    func(c Context) (string, bool) { return "crashloop alert is firing", c.Crashloop },
		actions:     logsRetryRecheck,
	},
	{
		code: CauseReadiness, title: "The health check never passes",
		explanation: "The container started but its readiness probe did not succeed in time: wrong port or path, or a slow start.",
		reasons:     []string{"ReadinessFailed"},
		patterns:    []string{"readiness probe", "timed out waiting for"},
		actions:     logsRetryRecheck,
	},
	{
		code: CauseOperationFailed, title: "The last operation failed",
		explanation: "The reconciler tried to create, start or replace this resource and the operation returned an error.",
		reasons:     []string{"CreateFailed", "StartFailed", "ReplaceFailed", "RemoveFailed", "StoreError", "WriteFailed"},
		actions:     logsRetryRecheck,
	},
}

// Explain builds the report for in. A resource whose Ready condition is True
// yields a healthy report with no causes.
func Explain(in Input) Report {
	rep := Report{
		Kind: in.Kind, Name: in.Name, ThresholdSecs: int64(in.Threshold / time.Second),
		Timeline: sortedTimeline(in.Conditions), Causes: []Cause{},
	}
	ready, notReady := readyState(in.Conditions)
	rep.Ready = ready
	if ready {
		return rep
	}
	if notReady != nil && !notReady.Since.IsZero() {
		since := notReady.Since
		rep.NotReadySince = &since
		rep.NotReadySeconds = int64(in.Now.Sub(since) / time.Second)
	}
	rep.Stuck = in.Threshold > 0 && rep.NotReadySeconds >= rep.ThresholdSecs
	rep.Causes = rank(in)
	if len(rep.Causes) == 0 && len(in.Conditions) == 0 {
		rep.Causes = []Cause{{
			Code: CauseNotReconciled, Title: "Not reconciled yet", Confidence: ConfidenceMedium,
			Explanation: "No status has been recorded for this resource yet.",
			Evidence:    []string{}, Actions: []Action{recheck()},
		}}
	}
	return rep
}

func readyState(cs []Condition) (bool, *Condition) {
	var first *Condition
	for i := range cs {
		if cs[i].Type != "Ready" {
			continue
		}
		if cs[i].Status == "True" {
			return true, nil
		}
		if first == nil {
			first = &cs[i]
		}
	}
	return false, first
}

func sortedTimeline(cs []Condition) []Condition {
	out := append([]Condition{}, cs...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Since.Before(out[j].Since) })
	return out
}

func rank(in Input) []Cause {
	texts := collectTexts(in)
	var causes []Cause
	for _, r := range table {
		var evidence []string
		reasonHit := false
		for _, c := range in.Conditions {
			for _, want := range r.reasons {
				if strings.EqualFold(c.Reason, want) && c.Status != "True" {
					reasonHit = true
					evidence = append(evidence, "condition "+c.Type+" reason "+c.Reason+msgSuffix(c.Message))
				}
			}
		}
		patternHit := false
		for _, t := range texts {
			low := strings.ToLower(t.text)
			for _, p := range r.patterns {
				if strings.Contains(low, p) {
					patternHit = true
					evidence = append(evidence, t.source+": "+clip(t.text))
					break
				}
			}
		}
		ctxHit := false
		if r.ctxMatch != nil {
			if line, ok := r.ctxMatch(in.Ctx); ok {
				ctxHit = true
				evidence = append(evidence, strings.TrimSpace(line))
			}
		}
		if !reasonHit && !patternHit && !ctxHit {
			continue
		}
		if r.code == CauseOperationFailed && len(causes) > 0 {
			continue // generic fallback, only when nothing more specific matched
		}
		conf := ConfidenceMedium
		if reasonHit || ctxHit {
			conf = ConfidenceHigh
		}
		causes = append(causes, Cause{
			Code: r.code, Title: r.title, Explanation: r.explanation, Confidence: conf,
			Evidence: dedupe(evidence), Actions: r.actions(in),
		})
	}
	sort.SliceStable(causes, func(i, j int) bool { return confRank(causes[i].Confidence) > confRank(causes[j].Confidence) })
	return causes
}

type text struct{ source, text string }

func collectTexts(in Input) []text {
	var out []text
	if in.Ctx.LastError != "" {
		out = append(out, text{"last error", in.Ctx.LastError})
	}
	for _, c := range in.Conditions {
		if c.Message != "" {
			out = append(out, text{"condition " + c.Type + " message", c.Message})
		}
	}
	for _, e := range in.Ctx.RecentEvents {
		out = append(out, text{"event", e})
	}
	return out
}

func msgSuffix(m string) string {
	if m == "" {
		return ""
	}
	return ": " + clip(m)
}

func clip(s string) string {
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		return s[:240] + "..."
	}
	return s
}

func confRank(c string) int {
	if c == ConfidenceHigh {
		return 2
	}
	return 1
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
