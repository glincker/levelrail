package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"
)

const skewDeploymentIDKey = "NEXT_DEPLOYMENT_ID"

// appSkewView is the read-only skew protection summary for one app.
type appSkewView struct {
	App                      string `json:"app"`
	NextJS                   bool   `json:"nextjs"`
	DeploymentID             string `json:"deployment_id,omitempty"`
	PreviousReleaseHeldUntil string `json:"previous_release_held_until,omitempty"`
	HeldSecondsLeft          int64  `json:"held_seconds_left,omitempty"`
	NextCache                string `json:"next_cache,omitempty"`
}

func appsSkewUsage(prog string) string {
	return fmt.Sprintf(`Usage:
  %[1]s apps skew <name>

Shows skew protection for a Next.js app: the current NEXT_DEPLOYMENT_ID, how long
the previous release is still held for open browser tabs, and whether the shared
cache (next.cache) is set. Read only.
%[2]s`, prog, commonFlagsHelp)
}

func shortDeploymentID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

func runAppsSkew(prog string, args []string, stdout, stderr io.Writer, lookupEnv func(string) (string, bool)) int {
	c, code, ok := parseCLICall(prog, "apps skew", appsSkewUsage(prog), args, stdout, stderr, lookupEnv, nil)
	if !ok {
		return code
	}
	rest := c.fs.Args()
	if len(rest) != 1 {
		return c.invalid("expected an app name")
	}
	app, err := c.client.GetApp(context.Background(), rest[0])
	if err != nil {
		return c.fail(fmt.Errorf("get app %q: %w", rest[0], err))
	}
	view := buildSkewView(app.Name, app.Env, app.NextCache, app.PreviousReleaseHeldUntil, time.Now())
	return c.render(view, func() { printSkew(stdout, prog, view) })
}

func buildSkewView(name string, env map[string]string, nextCache, heldUntil string, now time.Time) appSkewView {
	v := appSkewView{App: name, DeploymentID: env[skewDeploymentIDKey], NextCache: nextCache, PreviousReleaseHeldUntil: heldUntil}
	for k := range env {
		if strings.HasPrefix(k, "NEXT_") {
			v.NextJS = true
			break
		}
	}
	v.NextJS = v.NextJS || nextCache != "" || v.DeploymentID != ""
	if t, err := time.Parse(time.RFC3339, heldUntil); err == nil && t.After(now) {
		v.HeldSecondsLeft = int64(t.Sub(now).Seconds())
	}
	return v
}

func printSkew(out io.Writer, prog string, v appSkewView) {
	if !v.NextJS {
		_, _ = fmt.Fprintf(out, "%s does not look like a Next.js app, so skew protection does not apply.\n", v.App)
		return
	}
	if v.DeploymentID == "" {
		_, _ = fmt.Fprintf(out, "deployment id:  not set yet (the next deploy of a Next.js build sets NEXT_DEPLOYMENT_ID)\n")
	} else {
		_, _ = fmt.Fprintf(out, "deployment id:  %s\n", shortDeploymentID(v.DeploymentID))
	}
	if v.HeldSecondsLeft > 0 {
		left := (time.Duration(v.HeldSecondsLeft) * time.Second).Round(time.Second)
		_, _ = fmt.Fprintf(out, "previous release: held for %s more (until %s)\n", left, v.PreviousReleaseHeldUntil)
	} else {
		_, _ = fmt.Fprintln(out, "previous release: nothing held, old browser tabs get the new build after the next deploy")
	}
	if v.NextCache == "" {
		_, _ = fmt.Fprintf(out, "next.cache:     not set; with several replicas run: %s apps next-cache set %s redis\n", prog, v.App)
	} else {
		_, _ = fmt.Fprintf(out, "next.cache:     %s\n", v.NextCache)
	}
}
