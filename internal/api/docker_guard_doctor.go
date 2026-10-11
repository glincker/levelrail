package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/docker"
	"github.com/GLINCKER/levelrail/internal/dockerguard"
)

const dockerGuardDocsPath = "/docker-access"

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
	warn := func(msg string) doctorCheckResource {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: msg, Fix: fix, DocsPath: dockerGuardDocsPath}
	}
	switch {
	case !res.Configured:
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "no docker guard configured on this process", DocsPath: dockerGuardDocsPath}
	case res.ConfigError != "":
		return warn("configuration error, failing closed: " + res.ConfigError)
	case res.RestartRequired:
		return warn(fmt.Sprintf("configured %s but running %s: restart the control plane to apply", res.Mode, res.Effective))
	case res.Effective == dockerguard.ModeEnforce:
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, DocsPath: dockerGuardDocsPath,
			Message: fmt.Sprintf("enforce: %d request(s) denied in the last %s", res.Denied, windowLabel(res.WindowSeconds))}
	case res.Effective == dockerguard.ModeAudit && res.WouldDeny > 0:
		return warn(fmt.Sprintf("audit: %d request(s) would have been denied in the last %s (top rule %s)", res.WouldDeny, windowLabel(res.WindowSeconds), res.Window[0].Rule))
	case res.Effective == dockerguard.ModeAudit:
		return warn(fmt.Sprintf("audit: nothing would have been denied in the last %s; enforce is likely safe", windowLabel(res.WindowSeconds)))
	default:
		return warn("off: every Levelrail process with the Docker socket can ask Docker for anything, including privileged containers")
	}
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
	isolated := secKnown && (sec.Rootless || sec.UsernsRemap)
	enforced := rt.dockerGuard != nil && rt.dockerGuard.Status().Effective == dockerguard.ModeEnforce
	if isolated || enforced || !secKnown {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg, DocsPath: dockerGuardDocsPath}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, DocsPath: dockerGuardDocsPath,
		Message: msg + "; a rootful daemon without the guard in enforce mode makes this process root equivalent",
		Fix:     "Switch the Docker guard to enforce, or run Docker rootless or with userns-remap."}
}

// dockerGuardAttentionItems tells an admin what audit mode saw and offers
// enforce once a full window was clean.
func (rt *Router) dockerGuardAttentionItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	if rt.dockerGuard == nil || !hasAbility(abilities, AbilityRoot) {
		return nil
	}
	res := rt.dockerGuardResource(r.Context(), now)
	window := windowLabel(res.WindowSeconds)
	params := map[string]string{"mode": string(res.Effective), "would_deny": strconv.Itoa(res.WouldDeny), "denied": strconv.Itoa(res.Denied)}
	switch {
	case res.Effective == dockerguard.ModeAudit && res.WouldDeny > 0:
		params["top_rule"] = res.Window[0].Rule
		return []attention.Item{feedItem(attention.Warning, attention.KindDockerGuard, "would-deny",
			fmt.Sprintf("audit mode: %d Docker API request(s) in the last %s would have been denied, most by rule %s", res.WouldDeny, window, res.Window[0].Rule), params)}
	case res.ReadyToEnforce:
		return []attention.Item{feedItem(attention.Info, attention.KindDockerGuard, "ready",
			fmt.Sprintf("audit mode saw no would-be denials in the last %s; switch to enforce", window), params)}
	case res.Effective == dockerguard.ModeEnforce && res.Denied > 0:
		params["top_rule"] = res.Window[0].Rule
		return []attention.Item{feedItem(attention.Warning, attention.KindDockerGuard, "denied",
			fmt.Sprintf("enforce mode denied %d Docker API request(s) in the last %s, most by rule %s", res.Denied, window, res.Window[0].Rule), params)}
	}
	return nil
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
