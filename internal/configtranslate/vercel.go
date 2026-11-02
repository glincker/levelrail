package configtranslate

import (
	"encoding/json"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strings"
)

const (
	fileVercel = "vercel.json"
)

// staticFrameworks are Vercel framework presets that build to a folder of files.
var staticFrameworks = map[string]bool{
	"vite": true, "create-react-app": true, "gatsby": true, "astro": true,
	"docusaurus-2": true, "vuepress": true, "hugo": true, "jekyll": true,
	"eleventy": true, "hexo": true, "parcel": true, "ember": true, "angular": true, "vue": true,
}

type vercelJSON struct {
	Framework       *string  `json:"framework"`
	BuildCommand    *string  `json:"buildCommand"`
	InstallCommand  *string  `json:"installCommand"`
	OutputDirectory *string  `json:"outputDirectory"`
	Regions         []string `json:"regions"`
	CleanURLs       *bool    `json:"cleanUrls"`
	TrailingSlash   *bool    `json:"trailingSlash"`
	Crons           []struct {
		Path     string `json:"path"`
		Schedule string `json:"schedule"`
	} `json:"crons"`
	Rewrites []struct {
		Source      string            `json:"source"`
		Destination string            `json:"destination"`
		Has         []json.RawMessage `json:"has"`
		Missing     []json.RawMessage `json:"missing"`
	} `json:"rewrites"`
	Redirects []struct {
		Source      string            `json:"source"`
		Destination string            `json:"destination"`
		Permanent   *bool             `json:"permanent"`
		StatusCode  int               `json:"statusCode"`
		Has         []json.RawMessage `json:"has"`
		Missing     []json.RawMessage `json:"missing"`
	} `json:"redirects"`
	Headers []struct {
		Source  string `json:"source"`
		Headers []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"headers"`
		Has []json.RawMessage `json:"has"`
	} `json:"headers"`
	Functions map[string]struct {
		Runtime     string `json:"runtime"`
		Memory      int    `json:"memory"`
		MaxDuration int    `json:"maxDuration"`
	} `json:"functions"`
	Images json.RawMessage `json:"images"`
	Build  struct {
		Env map[string]string `json:"env"`
	} `json:"build"`
	Builds json.RawMessage `json:"builds"`
	Routes json.RawMessage `json:"routes"`
	Git    struct {
		DeploymentEnabled json.RawMessage `json:"deploymentEnabled"`
	} `json:"git"`
}

func str(p *string) string {
	if p == nil {
		return ""
	}
	return strings.TrimSpace(*p)
}

func translateVercel(files map[string][]byte, o options) (Fragment, []Finding) {
	frag := Fragment{Version: 1, Services: map[string]Service{}}
	var fs []Finding
	add := func(code string, sev Severity, subject, msg, fix string) {
		fs = append(fs, Finding{Code: code, Severity: sev, Source: fileVercel, Subject: subject, Message: msg, Fix: fix})
	}

	var v vercelJSON
	b, _ := lookup(files, fileVercel)
	if err := json.Unmarshal(b, &v); err != nil {
		add("vercel.invalid", SeverityBlocker, "", "vercel.json is not valid JSON: "+err.Error(), "Fix the JSON syntax and run the translation again.")
		return frag, fs
	}

	fw := str(v.Framework)
	isStatic := staticFrameworks[fw] || (fw == "" && str(v.OutputDirectory) != "")
	svc := Service{}
	if isStatic {
		svc.Build, fs = staticBuild(v, fs)
	} else {
		svc.Build = Build{Type: "railpack"}
		svc.Port = o.nodePort
		if fw == "" {
			add("vercel.framework.unknown", SeverityInfo, "", "no framework is set, Railpack detects the app from the repo", "Check the detected provider in the Deploy from git wizard.")
		}
		if str(v.BuildCommand) != "" || str(v.InstallCommand) != "" || str(v.OutputDirectory) != "" {
			add("vercel.build.custom", SeverityWarn, "", "Railpack runs the package.json build and start scripts, custom build, install and output settings are not applied",
				"Move them into package.json scripts, or use a Dockerfile and build.type dockerfile.")
		}
	}

	if len(v.Build.Env) > 0 {
		add("vercel.build.env", SeverityWarn, "build.env", "vercel.json build.env is not read, only app env vars with a public prefix (NEXT_PUBLIC_, VITE_ and similar) reach a build",
			"Set the NEXT_PUBLIC_* values as plain app env before the first deploy; secrets are never passed to a build.")
	}
	if edge := edgeSourceFiles(files); len(edge) > 0 {
		fs = append(fs, edgeSourceFinding(edge))
	}
	fs = append(fs, vercelDepFindings(files)...)
	if len(v.Builds) > 0 || len(v.Routes) > 0 {
		add("vercel.legacy", SeverityWarn, "builds/routes", "legacy builds and routes keys are not translated", "Port them to the current rewrites, redirects and headers keys first.")
	}
	if len(v.Images) > 0 {
		add("vercel.images", SeverityInfo, "images", "no global image CDN, next/image runs in your container", "Add sharp to the image or set images.unoptimized.")
	}
	if len(v.Git.DeploymentEnabled) > 0 {
		add("vercel.git.deployment", SeverityInfo, "git.deploymentEnabled", "per-branch deploy switches are not translated", "Use apps previews enable and the git source's branch settings.")
	}
	switch {
	case len(v.Regions) > 1:
		add("vercel.regions.multi", SeverityWarn, strings.Join(v.Regions, ","), "no equivalent: multi-region deployment, a service runs on one node", "Pick the node closest to your users.")
	case len(v.Regions) == 1:
		add("vercel.regions.single", SeverityInfo, v.Regions[0], "region ids do not apply, a service runs on the node you choose", "Place the service on a node near your users.")
	}

	fs = append(fs, vercelFunctions(&svc, v, isStatic)...)
	fs = append(fs, vercelRouting(&svc, v, isStatic)...)
	fs = append(fs, vercelCrons(&svc, v, isStatic, o)...)
	frag.Services["web"] = svc
	return frag, fs
}

func staticBuild(v vercelJSON, fs []Finding) (Build, []Finding) {
	out := str(v.OutputDirectory)
	cmd := str(v.BuildCommand)
	if cmd == "" {
		return Build{Type: "static"}, append(fs, Finding{Code: "vercel.static.nobuild", Severity: SeverityWarn, Source: fileVercel,
			Message: "no buildCommand, so committed files are served as they are",
			Fix:     "Set build.command and build.publish, or commit the built output and set build.path."})
	}
	if out == "" {
		out = "dist"
		fs = append(fs, Finding{Code: "vercel.static.output", Severity: SeverityInfo, Source: fileVercel, Subject: "outputDirectory",
			Message: "no outputDirectory, dist is assumed", Fix: "Set build.publish to the folder your build writes."})
	}
	if in := str(v.InstallCommand); in != "" {
		cmd = in + " && " + cmd
	}
	return Build{Type: "static", Command: cmd, Publish: out}, fs
}

func vercelFunctions(svc *Service, v vercelJSON, isStatic bool) []Finding {
	var fs []Finding
	maxMem := 0
	for _, pat := range sortedKeys(v.Functions) {
		f := v.Functions[pat]
		if f.Runtime == "edge" {
			fs = append(fs, Finding{Code: "vercel.runtime.edge", Severity: SeverityBlocker, Source: fileVercel, Subject: pat,
				Message: "no equivalent: Vercel Edge runtime",
				Fix:     "Move this code to the Node runtime, then drop the runtime setting."})
		}
		if f.Memory > maxMem {
			maxMem = f.Memory
		}
		if f.MaxDuration > 0 {
			fs = append(fs, Finding{Code: "vercel.function.maxduration", Severity: SeverityInfo, Source: fileVercel, Subject: pat,
				Message: "no per-function time limit applies in a container",
				Fix:     "Remove maxDuration, or enforce a timeout in the app."})
		}
	}
	if maxMem > 0 && !isStatic {
		svc.Resources = &Resources{Memory: fmt.Sprintf("%dMi", maxMem)}
		fs = append(fs, Finding{Code: "vercel.function.memory", Severity: SeverityInfo, Source: fileVercel,
			Message: "the largest function memory is applied to the whole service", Fix: "Adjust resources.memory to what the whole app needs."})
	}
	return fs
}

var (
	splatTailRe = regexp.MustCompile(`/(:[A-Za-z_][A-Za-z0-9_]*[*+]|\(\.\*\)|\*)$`)
	namedRe     = regexp.MustCompile(`^[A-Za-z0-9_./:-]*$`)
)

// convertSource turns a Vercel path pattern into a static-site pattern, and
// returns the splat parameter name used by the source ("" when none).
func convertSource(src string) (string, string, bool) {
	if m := splatTailRe.FindStringSubmatch(src); m != nil {
		head := strings.TrimSuffix(src, m[0])
		name := strings.TrimRight(strings.TrimPrefix(m[1], ":"), "*+")
		if m[1] == "(.*)" || m[1] == "*" {
			name = ""
		}
		if namedRe.MatchString(head) {
			return head + "/*", name, true
		}
		return "", "", false
	}
	if namedRe.MatchString(src) {
		return src, "", true
	}
	return "", "", false
}

func convertDest(dst, splat string) (string, bool) {
	if splat != "" {
		for _, s := range []string{":" + splat + "*", ":" + splat + "+", ":" + splat} {
			dst = strings.ReplaceAll(dst, s, ":splat")
		}
	}
	dst = strings.ReplaceAll(dst, "$1", ":splat")
	if strings.ContainsAny(dst, "()") {
		return "", false
	}
	return dst, true
}

func vercelRouting(svc *Service, v vercelJSON, isStatic bool) []Finding {
	var fs []Finding
	skip := func(code, subject, msg, fix string) {
		fs = append(fs, Finding{Code: code, Severity: SeverityWarn, Source: fileVercel, Subject: subject, Message: msg, Fix: fix})
	}
	if !isStatic {
		if n := len(v.Rewrites) + len(v.Redirects) + len(v.Headers); n > 0 {
			skip("vercel.routing.server", "rewrites/redirects/headers",
				"routing and header rules only apply to static services, this one runs a server",
				"Move them into the app (for Next.js, next.config.js rewrites, redirects and headers move with the app).")
		}
		return fs
	}
	if v.CleanURLs != nil || v.TrailingSlash != nil {
		opts := &StaticOptions{}
		if v.CleanURLs != nil {
			opts.CleanURLs = *v.CleanURLs
		}
		if v.TrailingSlash != nil {
			opts.TrailingSlash = map[bool]string{true: "add", false: "remove"}[*v.TrailingSlash]
		}
		svc.Static = opts
	}
	// Redirects come first: routes match in order and Vercel applies redirects before rewrites.
	for _, r := range v.Redirects {
		if len(r.Has)+len(r.Missing) > 0 {
			skip("vercel.route.condition", r.Source, "has and missing conditions have no equivalent, redirect skipped", "Handle the condition in the app or drop the rule.")
			continue
		}
		from, splat, ok := convertSource(r.Source)
		to, ok2 := convertDest(r.Destination, splat)
		if !ok || !ok2 {
			skip("vercel.route.pattern", r.Source, "pattern uses regex features the static router lacks, redirect skipped", "Rewrite it as literal paths, :name segments or a trailing /*.")
			continue
		}
		status := r.StatusCode
		if status == 0 {
			status = 307
			if r.Permanent != nil && *r.Permanent {
				status = 308
			}
		}
		svc.Routes = append(svc.Routes, Route{From: from, To: to, Status: status})
	}
	for _, r := range v.Rewrites {
		if len(r.Has)+len(r.Missing) > 0 {
			skip("vercel.route.condition", r.Source, "has and missing conditions have no equivalent, rewrite skipped", "Handle the condition in the app or drop the rule.")
			continue
		}
		if strings.Contains(r.Destination, "://") {
			skip("vercel.rewrite.proxy", r.Source, "no equivalent: rewrite to an external URL (proxy), rule skipped", "Proxy in the app, or point clients at the target directly.")
			continue
		}
		from, splat, ok := convertSource(r.Source)
		to, ok2 := convertDest(r.Destination, splat)
		if !ok || !ok2 {
			skip("vercel.route.pattern", r.Source, "pattern uses regex features the static router lacks, rewrite skipped", "Rewrite it as literal paths, :name segments or a trailing /*.")
			continue
		}
		svc.Routes = append(svc.Routes, Route{From: from, To: to, Status: 200})
	}
	for _, h := range v.Headers {
		from, _, ok := convertSource(h.Source)
		if !ok || len(h.Has) > 0 {
			skip("vercel.headers.pattern", h.Source, "header source uses regex or conditions, rule skipped", "Rewrite the source as literal paths, :name segments or a trailing /*.")
			continue
		}
		vals := map[string]string{}
		for _, kv := range h.Headers {
			vals[kv.Key] = kv.Value
		}
		svc.Headers = append(svc.Headers, HeaderRule{Path: from, Values: vals})
	}
	return fs
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func vercelCrons(svc *Service, v vercelJSON, isStatic bool, o options) []Finding {
	if len(v.Crons) == 0 {
		return nil
	}
	if isStatic {
		return []Finding{{Code: "vercel.cron.static", Severity: SeverityBlocker, Source: fileVercel, Subject: "crons",
			Message: "no equivalent: a static site has no container to run a scheduled task in",
			Fix:     "Run the endpoints from a server service, or schedule them outside the site."}}
	}
	var fs []Finding
	used := map[string]bool{}
	for _, c := range v.Crons {
		pathOnly := strings.SplitN(c.Path, "?", 2)[0]
		name := strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(pathOnly), "-"), "-")
		if name == "" {
			name = "cron"
		}
		base := name
		for i := 2; used[name]; i++ {
			name = fmt.Sprintf("%s-%d", base, i)
		}
		used[name] = true
		svc.Jobs = append(svc.Jobs, Job{
			Name: name, Schedule: c.Schedule, Timezone: "UTC", Mode: "exec",
			Command: cronCommand(o.nodePort, c.Path),
		})
		fs = append(fs, Finding{Code: "vercel.cron.http", Severity: SeverityWarn, Source: fileVercel, Subject: c.Path,
			Message: "Vercel cron makes an HTTP call, a Levelrail task runs a command in the container, so a curl, wget or node call is proposed that sends Authorization: Bearer $CRON_SECRET when CRON_SECRET is set",
			Fix:     "Set CRON_SECRET as an app secret if the route checks it, and confirm the port is right."})
	}
	return fs
}

type vercelDep struct {
	sev     Severity
	message string
	fix     string
}

// vercelDeps maps hosted @vercel packages to what replaces them when self-hosting.
var vercelDeps = map[string]vercelDep{
	"og":             {SeverityWarn, "@vercel/og needs the Vercel runtime image tooling", "Import ImageResponse from next/og (bundled with Next.js, runs on Node), or use satori with resvg-js."},
	"kv":             {SeverityWarn, "@vercel/kv is a hosted Redis, there is no such service behind it here", "Add a Redis database to the stack and switch to a Redis client (ioredis or redis) reading REDIS_URL."},
	"blob":           {SeverityWarn, "@vercel/blob is a hosted object store, there is no such service behind it here", "Use an S3-compatible store with the AWS S3 SDK, or a volume if one node is enough."},
	"postgres":       {SeverityWarn, "@vercel/postgres talks to a hosted Postgres over its own proxy and fails against a plain server", "Add a Postgres database to the stack and use pg or postgres.js with DATABASE_URL."},
	"analytics":      {SeverityInfo, "@vercel/analytics reports to a hosted dashboard, nothing receives it here", "Remove the component, per-app traffic stats are built in, or send events to a self-hosted analytics tool."},
	"speed-insights": {SeverityInfo, "@vercel/speed-insights reports to a hosted dashboard, nothing receives it here", "Remove the component, or report web-vitals to your own endpoint."},
}

var vercelDepImport = regexp.MustCompile(`(?:from|import|require)\s*\(?\s*['"]@vercel/(og|kv|blob|postgres|analytics|speed-insights)(?:/[^'"]*)?['"]`)

// vercelDepFindings emits one finding per hosted @vercel package imported in the source files.
func vercelDepFindings(files map[string][]byte) []Finding {
	seen := map[string]bool{}
	for name, b := range files {
		clean := path.Clean(name)
		if !edgeSourceExt[path.Ext(clean)] || strings.HasPrefix(clean, "node_modules/") || strings.Contains(clean, "/node_modules/") {
			continue
		}
		for _, m := range vercelDepImport.FindAllSubmatch(b, -1) {
			seen[string(m[1])] = true
		}
	}
	names := make([]string, 0, len(seen))
	for n := range seen {
		names = append(names, n)
	}
	sort.Strings(names)
	var out []Finding
	for _, n := range names {
		d := vercelDeps[n]
		out = append(out, Finding{Code: "vercel.dep." + n, Severity: d.sev, Source: fileVercel, Subject: "@vercel/" + n, Message: d.message, Fix: d.fix})
	}
	return out
}
