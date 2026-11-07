package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/apiclient"
	"github.com/GLINCKER/levelrail/internal/configtranslate"
)

// cronSecretEnv is the env var Vercel sends as the cron bearer token.
const cronSecretEnv = "CRON_SECRET"

type vercelRule struct {
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Headers     []struct {
		Key string `json:"key"`
	} `json:"headers"`
}

type vercelExtras struct {
	Crons []struct {
		Path     string `json:"path"`
		Schedule string `json:"schedule"`
	} `json:"crons"`
	Headers   []vercelRule               `json:"headers"`
	Redirects []vercelRule               `json:"redirects"`
	Rewrites  []vercelRule               `json:"rewrites"`
	Regions   []string                   `json:"regions"`
	Functions map[string]json.RawMessage `json:"functions"`
}

// parseVercelExtras reads the keys this flow handles; a missing file is fine, bad JSON is not.
func parseVercelExtras(raw []byte) (vercelExtras, error) {
	var v vercelExtras
	if len(raw) == 0 {
		return v, nil
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return v, fmt.Errorf("vercel.json is not valid JSON: %w", err)
	}
	return v, nil
}

var cronNonSlug = regexp.MustCompile(`[^a-z0-9]+`)

// cronJobName makes a unique job name from the cron path.
func cronJobName(path string, used map[string]bool) string {
	name := strings.Trim(cronNonSlug.ReplaceAllString(strings.ToLower(strings.SplitN(path, "?", 2)[0]), "-"), "-")
	if name == "" {
		name = "cron"
	}
	base := name
	for i := 2; used[name]; i++ {
		name = fmt.Sprintf("%s-%d", base, i)
	}
	used[name] = true
	return name
}

func (g *guidedRun) stepExtras() {
	v, _ := parseVercelExtras(g.files["vercel.json"])
	s := configtranslate.SummarizeVercel(g.files)
	g.extrasCrons(v)
	g.extrasRules(v, s.Framework == "next" || s.Framework == "nextjs")
	if len(v.Regions) > 0 {
		g.say("  regions (%s): not applicable on a self-hosted server, Levelrail runs where your server runs.", strings.Join(v.Regions, ", "))
	}
	if len(v.Functions) > 0 {
		g.say("  functions (%d entries): memory, duration and runtime settings are not applicable on a self-hosted server; set app resources with the app spec instead.", len(v.Functions))
	}
}

func (g *guidedRun) extrasCrons(v vercelExtras) {
	if len(v.Crons) == 0 {
		return
	}
	hasSecret := false
	for _, p := range g.envPairs {
		hasSecret = hasSecret || strings.HasPrefix(p, cronSecretEnv+"=")
	}
	client := apiClientFromFlags(g.prog, g.apiURL, g.token, g.profile, g.lookupEnv)
	existing := map[string]bool{}
	if !g.f.dryRun {
		if jobs, err := client.ListJobs(context.Background(), g.f.name); err == nil {
			for _, j := range jobs {
				existing[j.Name] = true
			}
		}
	}
	used := map[string]bool{}
	for _, c := range v.Crons {
		name := cronJobName(c.Path, used)
		line := fmt.Sprintf("cron job %s: %s GET %s (Authorization: Bearer from %s)", name, c.Schedule, c.Path, cronSecretEnv)
		if g.f.dryRun {
			g.say("  would create %s", line)
			continue
		}
		if existing[name] {
			g.say("  kept existing %s", line)
			continue
		}
		req := scheduledTaskRequest{Name: name, Schedule: c.Schedule, Timezone: "UTC", Mode: "http", Enabled: true,
			HTTP: &apiclient.JobHTTP{Method: "GET", Path: c.Path, Headers: []apiclient.JobHeader{{Name: "Authorization", SecretRef: cronSecretEnv, Prefix: "Bearer "}}}}
		if _, err := client.CreateJob(context.Background(), g.f.name, req); err != nil {
			g.say("  could not create %s: %v", line, err)
			continue
		}
		g.say("  created %s", line)
	}
	if !hasSecret {
		g.say("  Set %s on the app (%s apps env set %s %s=...) so the cron calls are authorized. The value is never printed here.", cronSecretEnv, g.prog, g.f.name, cronSecretEnv)
	}
}

func (g *guidedRun) extrasRules(v vercelExtras, next bool) {
	total := len(v.Headers) + len(v.Redirects) + len(v.Rewrites)
	if total == 0 {
		return
	}
	if next {
		g.say("  Next.js project: redirects, rewrites and headers are served by the app through next.config, so there is nothing to do here. If %d rule(s) exist only in vercel.json, mirror them in next.config.", total)
		return
	}
	g.say("  not carried over: %d header, redirect and rewrite rule(s). Port each one to your server or app config (static sites: see import config):", total)
	listRules := func(kind string, rules []vercelRule) {
		for i, r := range rules {
			if i == guidedListLimit {
				g.say("    %s: ... and %d more", kind, len(rules)-i)
				return
			}
			switch {
			case len(r.Headers) > 0:
				keys := make([]string, 0, len(r.Headers))
				for _, h := range r.Headers {
					keys = append(keys, h.Key)
				}
				g.say("    %s: %s sets %s", kind, r.Source, strings.Join(keys, ", "))
			default:
				g.say("    %s: %s -> %s", kind, r.Source, r.Destination)
			}
		}
	}
	listRules("header", v.Headers)
	listRules("redirect", v.Redirects)
	listRules("rewrite", v.Rewrites)
}
