package api

import (
	"context"
	"fmt"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/runtimesettings"
	"github.com/GLINCKER/levelrail/internal/store"
)

const dockerGuardDocsPath = "/docker-access"

// The high-risk finding is its own doctor row identity, see silenceRootOnlyDoctorKeys.
const (
	dockerGuardHighRiskCode = "docker_guard_high_risk"
	dockerGuardHighRiskName = "Docker API guard alert"
)

// daemonSecurityReporter is the Docker client behind DockerPinger, narrowed
// the same consumer-defined way runtimeReporter is.
type daemonSecurityReporter interface {
	DaemonSecurity(ctx context.Context) (docker.DaemonSecurity, error)
}

// doctorCheckDockerGuard reports the guard's mode and what audit mode saw.
func (rt *Router) doctorCheckDockerGuard(ctx context.Context) doctorCheckResource {
	const code, name = "docker_guard", "Docker API guard"
	res := rt.dockerGuardResource(ctx, time.Now())
	fix := "Review the would-be denials, then switch with `" + rt.cliName() + " docker-guard set --mode enforce` or the Security settings page."
	okResult := func(msg string) doctorCheckResource {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg, DocsPath: dockerGuardDocsPath}
	}
	warn := func(msg string) doctorCheckResource {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: msg, Fix: fix, DocsPath: dockerGuardDocsPath}
	}
	switch {
	case !res.Configured:
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "no docker guard configured on this process", DocsPath: dockerGuardDocsPath}
	case res.ConfigError != "" && !res.Running:
		msg := "the settings file is invalid and the guard is not running, so Docker clients reach the daemon directly: " + res.ConfigError
		if res.StartError != "" {
			msg += "; start error: " + res.StartError
		}
		out := warn(msg)
		out.Class = attention.ClassIncident
		return out
	case res.ConfigError != "":
		// A broken guard is an incident, unlike "off on a rootful daemon", which stays advice by choice (less noise).
		out := warn("configuration error, failing closed: " + res.ConfigError)
		out.Class = attention.ClassIncident
		return out
	case !res.Running && res.StartError != "":
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, DocsPath: dockerGuardDocsPath, Class: attention.ClassIncident,
			Message: "the guard could not start, so Docker clients use the daemon socket directly: " + res.StartError,
			Fix:     "Point DOCKER_HOST at a unix socket so the guard can front it, or set APP_DOCKER_GUARD=off to acknowledge running unguarded."}
	case res.RestartRequired:
		return warn(fmt.Sprintf("configured %s but running %s: restart the control plane to apply", res.Mode, res.Effective))
	case res.WindowError != "" && res.Effective != dockerguard.ModeOff:
		return warn("could not read the guard audit log, so what the guard saw is unknown: " + res.WindowError)
	case res.Effective == dockerguard.ModeEnforce:
		out := doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, DocsPath: dockerGuardDocsPath,
			Message: fmt.Sprintf("enforce: %d decision(s) denied in the last %s", res.Denied, windowLabel(res.WindowSeconds))}
		if res.Denied > 0 {
			out.Fix = "If a workload you trust was blocked, relax with APP_DOCKER_GUARD=audit or `" + rt.cliName() + " docker-guard set --mode audit`."
		}
		return out
	case res.Effective == dockerguard.ModeAudit && res.WouldDeny > 0:
		top, high := res.topWouldDeny()
		if high {
			// Its own code and name, so a silence set for another guard state cannot hide it.
			out := warn(fmt.Sprintf("audit: rule %s would have denied a request in the last %s (last seen %s), so a caller requested host-level power", top.Rule, windowLabel(res.WindowSeconds), top.LastSeen))
			out.Code, out.Name, out.Class = dockerGuardHighRiskCode, dockerGuardHighRiskName, attention.ClassIncident
			return out
		}
		// Visible here but not a warning: the attention feed carries the single decision row.
		return okResult(fmt.Sprintf("audit: %d decision(s) would have been denied in the last %s (top rule %s)", res.WouldDeny, windowLabel(res.WindowSeconds), top.Rule))
	case res.Effective == dockerguard.ModeAudit && res.ReadyToEnforce:
		return okResult(fmt.Sprintf("audit: nothing would have been denied in the last %s; jobs that run less often were not seen", windowLabel(res.WindowSeconds)))
	case res.Effective == dockerguard.ModeAudit:
		return okResult(auditWatching(res.AuditSince))
	default:
		if rt.daemonRootless(ctx) {
			return warn("off: a caller of the Docker API can still bind the daemon user's home, data dir and Levelrail secrets into a container; the guard also defends bind_sensitive_path, bind_undeclared, exec_privileged and image_import")
		}
		return warn("off: every Levelrail process with the Docker socket can ask Docker for anything, including privileged containers")
	}
}

func auditWatching(since time.Time) string {
	if since.IsZero() {
		return "audit: still watching, no observation window yet"
	}
	return "audit since " + since.UTC().Format("2006-01-02") + ", still watching"
}

// daemonRootless is true only when the daemon is known to be rootless.
// userns-remap is not isolation: an API caller can opt a container out of it.
func (rt *Router) daemonRootless(ctx context.Context) bool {
	rep, ok := rt.dockerPinger.(daemonSecurityReporter)
	if !ok {
		return false
	}
	s, err := rep.DaemonSecurity(ctx)
	return err == nil && s.Rootless
}

// doctorCheckDockerPrivilege reports what a compromise of this process
// would hold: the daemon's isolation and this user's socket access.
func (rt *Router) doctorCheckDockerPrivilege(ctx context.Context) doctorCheckResource {
	const code, name = "docker_privilege", "Docker privilege"
	su := docker.CurrentServiceUser()
	var sec docker.DaemonSecurity
	secKnown := false
	if rep, ok := rt.dockerPinger.(daemonSecurityReporter); ok {
		if s, err := rep.DaemonSecurity(ctx); err == nil {
			sec, secKnown = s, true
		}
	}
	facts := []string{
		"daemon rootless=" + knownBool(secKnown, sec.Rootless),
		"userns-remap=" + knownBool(secKnown, sec.UsernsRemap),
		"service user " + orUnknown(su.Name) + " root=" + strconv.FormatBool(su.Root) + " docker-group=" + strconv.FormatBool(su.DockerGroup),
	}
	msg := strings.Join(facts, ", ")
	mode := dockerguard.ModeOff
	if rt.dockerGuard != nil {
		mode = rt.dockerGuard.Status().Effective
	}
	if (secKnown && sec.Rootless) || mode == dockerguard.ModeEnforce || !secKnown {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg, DocsPath: dockerGuardDocsPath}
	}
	if mode == dockerguard.ModeAudit {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, DocsPath: dockerGuardDocsPath,
			Message: msg + "; rootful daemon: audit logs but does not block, this process stays root equivalent until enforce",
			Fix:     "Switch the Docker guard to enforce, or run Docker rootless."}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, DocsPath: dockerGuardDocsPath,
		Message: msg + "; a rootful daemon with the guard off makes this process root equivalent",
		Fix:     "Turn the Docker guard on (audit, then enforce), or run Docker rootless."}
}

// dockerGuardAttentionItems tells an admin what audit mode saw and offers
// enforce once a full window was clean.
func (rt *Router) dockerGuardAttentionItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	if rt.dockerGuard == nil || !hasAbility(abilities, AbilityRoot) {
		return nil
	}
	res := rt.dockerGuardResource(r.Context(), now)
	window := windowLabel(res.WindowSeconds)
	params := map[string]string{"mode": string(res.Effective), "would_deny": strconv.Itoa(res.WouldDeny), "denied": strconv.Itoa(res.Denied), "window": window}
	switch {
	case res.Effective == dockerguard.ModeAudit && res.WouldDeny > 0:
		return rt.dockerGuardWouldDenyItems(r, res, params, now)
	case res.ReadyToEnforce:
		it := feedItem(attention.Info, attention.KindDockerGuard, "ready",
			fmt.Sprintf("audit mode saw no would-be denials in the last %s; jobs that run less often were not seen", window), params)
		it.Title = "Docker API guard can switch to enforce"
		return []attention.Item{it}
	case res.Effective == dockerguard.ModeEnforce && res.Denied > 0:
		params["top_rule"] = res.Window[0].Rule
		return []attention.Item{feedItem(attention.Warning, attention.KindDockerGuard, "denied",
			fmt.Sprintf("enforce mode denied %d Docker API decision(s) in the last %s, most by rule %s", res.Denied, window, res.Window[0].Rule), params)}
	}
	return nil
}

// dockerGuardWouldDenyItems emits one item per high-risk rule, each with its
// own id so a silence on a noisy rule cannot hide a new one. Low-risk
// findings show as a single item only when nothing high-risk was seen.
func (rt *Router) dockerGuardWouldDenyItems(r *http.Request, res dockerGuardResource, params map[string]string, now time.Time) []attention.Item {
	window := windowLabel(res.WindowSeconds)
	var high, low []dockerGuardRuleSummary
	for _, s := range res.Window {
		switch {
		case s.WouldDeny == 0:
		case dockerguard.RuleIsHighRisk(s.Rule):
			high = append(high, s)
		default:
			low = append(low, s)
		}
	}
	if len(high) == 0 {
		top := low[0]
		params["top_rule"], params["last_seen"] = top.Rule, top.LastSeen
		return []attention.Item{feedItem(attention.Warning, attention.KindDockerGuard, "would-deny:low:"+top.Rule,
			fmt.Sprintf("audit mode: %d Docker API decision(s) in the last %s would have been denied, most by rule %s", res.WouldDeny, window, top.Rule), params)}
	}
	// Silenced rules leave the list first, so the cap never cuts an unsilenced rule behind a muted noisy one.
	silences := rt.activeSilences(r.Context(), now)
	open := make([]dockerGuardRuleSummary, 0, len(high))
	for _, s := range high {
		if _, muted := store.FindSilence(silences, attention.KindDockerGuard+":"+attention.DockerGuardWouldDenyHigh+":"+s.Rule, "", now); !muted {
			open = append(open, s)
		}
	}
	if len(open) == 0 {
		return nil
	}
	high = open
	limit := rt.runtimeSettings.Int(r.Context(), runtimesettings.KeySecurityDockerGuardMaxItems)
	if limit < 1 || limit > len(high) {
		limit = len(high)
	}
	items := make([]attention.Item, 0, limit)
	for _, s := range high[:limit] {
		p := maps.Clone(params)
		p["top_rule"], p["last_seen"], p["risk"], p["would_deny"] = s.Rule, s.LastSeen, "high", strconv.Itoa(s.WouldDeny)
		items = append(items, feedItem(attention.Warning, attention.KindDockerGuard, attention.DockerGuardWouldDenyHigh+":"+s.Rule,
			fmt.Sprintf("audit mode: %d Docker API decision(s) in the last %s would have been denied by rule %s", s.WouldDeny, window, s.Rule), p))
	}
	if more := len(high) - limit; more > 0 {
		items[limit-1].Params["more"] = strconv.Itoa(more)
	}
	return items
}

func windowLabel(seconds int64) string {
	d := time.Duration(seconds) * time.Second
	if d >= 24*time.Hour && d%(24*time.Hour) == 0 {
		return strconv.FormatInt(int64(d/(24*time.Hour)), 10) + "d"
	}
	return d.String()
}

func knownBool(known, v bool) string {
	if !known {
		return "unknown"
	}
	return strconv.FormatBool(v)
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
