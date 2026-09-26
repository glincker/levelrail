package diagnose

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// HealthPathCandidates are the readiness paths tried, in order, when a probe
// gets a 404 from the configured one.
var HealthPathCandidates = []string{"/healthz", "/health", "/", "/api/health", "/up", "/status", "/ping"}

const maxHealthFixes = 3

var listenLogRes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(?:listening|listen|serving|running|started|ready)\b[^\n]{0,40}\b(?:on|at)\b[^\n]{0,30}?(?:port\s*|:)(\d{2,5})\b`),
	regexp.MustCompile(`(?i)Local:\s+https?://[^:\s]+:(\d{2,5})`),
	regexp.MustCompile(`(?i)Uvicorn running on https?://[^:\s]+:(\d{2,5})`),
	regexp.MustCompile(`(?i)Listening at: https?://[^:\s]+:(\d{2,5})`),
	regexp.MustCompile(`(?i)port\s*[:=]?\s*(\d{2,5})\b[^\n]{0,20}(?:listening|started|ready)`),
}

var healthNeedles = []string{"readiness probe", "timed out waiting for", "connection refused", "unhealthy", "health check failed", "healthcheck"}

func healthFailing(lines []line) (line, bool) { return firstContains(lines, healthNeedles...) }

func ruleWrongPort(in Input, lines []line) *Cause {
	f := in.Facts
	if f.Port <= 0 {
		return nil
	}
	var actual int
	var ev []Signal
	conf := ConfidenceMedium
	switch {
	case len(f.ListeningPorts) > 0:
		if containsInt(f.ListeningPorts, f.Port) {
			return nil
		}
		actual = f.ListeningPorts[0]
		conf = ConfidenceHigh
		ev = append(ev, Signal{Source: "container_sockets", Excerpt: fmt.Sprintf("listening on %v, configured port is %d", f.ListeningPorts, f.Port)})
	default:
		hl, failing := healthFailing(lines)
		if !failing {
			return nil
		}
		if l, m, ok := firstMatchAny(lines, listenLogRes); ok {
			if p, err := strconv.Atoi(m[1]); err == nil && p != f.Port {
				actual = p
				ev = append(ev, evidence(l), evidence(hl))
			}
		}
		if actual == 0 && len(f.ExposedPorts) > 0 && !containsInt(f.ExposedPorts, f.Port) {
			actual = f.ExposedPorts[0]
			ev = append(ev, Signal{Source: "image", Excerpt: fmt.Sprintf("image exposes %v, configured port is %d", f.ExposedPorts, f.Port)}, evidence(hl))
		}
	}
	if actual == 0 || actual == f.Port {
		return nil
	}
	return &Cause{
		Code:        CauseWrongPort,
		Title:       "App listens on a different port than configured",
		Explanation: fmt.Sprintf("The app is configured for port %d but appears to listen on %d, so traffic and health checks never reach it.", f.Port, actual),
		Confidence:  conf,
		Evidence:    ev,
		Fixes: []Fix{{
			Label:    fmt.Sprintf("Change the app port from %d to %d", f.Port, actual),
			Kind:     FixPatch,
			Changes:  []Change{{Field: "port", From: strconv.Itoa(f.Port), To: strconv.Itoa(actual)}},
			Redeploy: true,
		}},
	}
}

var (
	status404Re  = regexp.MustCompile(`(?i)(?:status|returned|got|http|code)\D{0,12}404|404 not found`)
	refusedNeedl = "connection refused"
)

func ruleHealthcheck(in Input, lines []line) *Cause {
	hl, ok := healthFailing(lines)
	if !ok {
		return nil
	}
	if ruleWrongPort(in, lines) != nil {
		return nil
	}
	c := &Cause{
		Code:       CauseHealthcheckFail,
		Title:      "Readiness check never passed",
		Confidence: ConfidenceMedium,
		Evidence:   []Signal{evidence(hl)},
	}
	if l, found := firstMatchLine(lines, status404Re); found {
		c.Evidence = append(c.Evidence, evidence(l))
		c.Confidence = ConfidenceHigh
		c.Explanation = "The readiness probe reached the app but the configured path returned 404."
		for _, p := range candidatePaths(in.Facts.HealthPath) {
			c.Fixes = append(c.Fixes, Fix{
				Label:    "Use " + p + " as the readiness path",
				Kind:     FixPatch,
				Changes:  []Change{{Field: "health.readiness.path", From: in.Facts.HealthPath, To: p}},
				Redeploy: true,
			})
		}
		return c
	}
	if strings.Contains(strings.ToLower(hl.text), refusedNeedl) {
		c.Explanation = "The readiness probe could not connect: nothing accepted the connection on the app's port."
		c.Fixes = []Fix{{Label: "Make the app listen on all interfaces on its configured port", Kind: FixManual, Hint: "Bind to 0.0.0.0 (not 127.0.0.1) on the configured port, and check the app is not crashing at startup."}}
		return c
	}
	c.Explanation = "The new container started but did not pass its readiness check in time."
	c.Fixes = []Fix{{Label: "Check the health path and startup time", Kind: FixManual, Hint: "Confirm health.readiness.path answers 200 quickly, and raise the readiness timeout if the app needs longer to start."}}
	return c
}

func firstMatchLine(lines []line, re *regexp.Regexp) (line, bool) {
	l, _, ok := firstMatch(lines, re)
	return l, ok
}

func candidatePaths(current string) []string {
	var out []string
	for _, p := range HealthPathCandidates {
		if p == current {
			continue
		}
		out = append(out, p)
		if len(out) == maxHealthFixes {
			break
		}
	}
	return out
}

func crashloopCause(in Input) *Cause {
	cl := in.Crashloop
	firing := cl != nil && cl.Firing
	exitFail := in.Facts.HasExit && in.Facts.ExitCode != 0
	if !firing && !exitFail {
		return nil
	}
	var ev []Signal
	if firing {
		ev = append(ev, Signal{Source: "crashloop_rule", Excerpt: crashloopExcerpt(*cl)})
	}
	if exitFail {
		ev = append(ev, Signal{Source: "container_state", Excerpt: "exit code " + strconv.Itoa(in.Facts.ExitCode)})
	}
	return &Cause{
		Code:        CauseCrashloopGeneric,
		Title:       "App keeps crashing on start",
		Explanation: "The container exits shortly after starting and no more specific cause matched.",
		Confidence:  ConfidenceMedium,
		Evidence:    ev,
		Fixes:       []Fix{{Label: "Read the last log lines before each exit", Kind: FixManual, Hint: "Look at the logs right after startup: bad config, a missing dependency service or a wrong entrypoint are the usual causes."}},
	}
}
