package configtranslate

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	fileProcfile      = "Procfile"
	fileAppJSON       = "app.json"
	defaultHerokuPort = 8080
)

var procLineRe = regexp.MustCompile(`^([A-Za-z0-9_-]+):\s+(.+)$`)

// railpackBuildpacks are the Heroku official buildpacks Railpack covers.
var railpackBuildpacks = map[string]bool{
	"heroku/nodejs": true, "heroku/python": true, "heroku/ruby": true,
	"heroku/go": true, "heroku/java": true, "heroku/php": true,
}

type appJSON struct {
	Env        map[string]json.RawMessage `json:"env"`
	Addons     []json.RawMessage          `json:"addons"`
	Buildpacks []struct {
		URL string `json:"url"`
	} `json:"buildpacks"`
	Formation map[string]struct {
		Quantity *int   `json:"quantity"`
		Size     string `json:"size"`
	} `json:"formation"`
	Scripts      map[string]string `json:"scripts"`
	Stack        string            `json:"stack"`
	Healthchecks map[string][]struct {
		Path string `json:"path"`
	} `json:"healthchecks"`
}

type envObj struct {
	Description string          `json:"description"`
	Value       json.RawMessage `json:"value"`
	Required    bool            `json:"required"`
	Generator   string          `json:"generator"`
}

func translateHeroku(files map[string][]byte, o options) (Fragment, []Finding) {
	frag := Fragment{Version: 1, Services: map[string]Service{}}
	var fs []Finding
	add := func(f Finding) { fs = append(fs, f) }

	var app appJSON
	if b, ok := lookup(files, fileAppJSON); ok {
		if err := json.Unmarshal(b, &app); err != nil {
			add(Finding{Code: "heroku.appjson.invalid", Severity: SeverityBlocker, Source: fileAppJSON,
				Message: "app.json is not valid JSON: " + err.Error(), Fix: "Fix the JSON syntax and run the translation again."})
			app = appJSON{}
		}
	}

	procs, release, perr := parseProcfile(files)
	if perr != "" {
		add(Finding{Code: "heroku.procfile.invalid", Severity: SeverityWarn, Source: fileProcfile, Subject: perr,
			Message: "Procfile line is not in the form 'type: command' and was skipped", Fix: "Rewrite the line as 'web: your command'."})
	}
	if len(procs) == 0 {
		if _, ok := lookup(files, fileProcfile); !ok {
			procs = map[string]string{"web": ""}
			add(Finding{Code: "heroku.procfile.missing", Severity: SeverityInfo, Source: fileAppJSON,
				Message: "no Procfile, so a web service is proposed with the start command left to the builder",
				Fix:     "Add a Procfile or set the command on the service."})
		}
	}

	build, bfs := herokuBuild(app)
	fs = append(fs, bfs...)

	for _, name := range sortedKeys(procs) {
		svc := Service{Build: build}
		if cmd := procs[name]; cmd != "" {
			svc.Command = []string{"sh", "-c", cmd}
		}
		if name == "web" {
			svc.Port = o.webPort
			svc.Env = map[string]Env{"PORT": {Value: fmt.Sprint(o.webPort)}}
			add(Finding{Code: "heroku.web.port", Severity: SeverityInfo, Source: fileProcfile, Subject: "web",
				Message: "Heroku injects $PORT, here the service listens on a fixed port and PORT is set to match",
				Fix:     "Change port and the PORT env value together if your app expects another port."})
			if hc := app.Healthchecks["web"]; len(hc) > 0 && hc[0].Path != "" {
				svc.Health = &Health{Readiness: &Probe{Path: hc[0].Path}}
			} else {
				add(Finding{Code: "heroku.health.none", Severity: SeverityInfo, Source: fileAppJSON, Subject: "web",
					Message: "no healthcheck is defined, so deploys are not readiness gated",
					Fix:     "Add health.readiness with a cheap path such as /health."})
			}
		} else {
			svc.Port = o.webPort
			add(Finding{Code: "heroku.process.port", Severity: SeverityInfo, Source: fileProcfile, Subject: name,
				Message: "the spec requires a port on every non-static service, nothing needs to listen on it",
				Fix:     "Leave it as is, or add an exec health probe."})
			if name != "worker" {
				add(Finding{Code: "heroku.process.custom", Severity: SeverityInfo, Source: fileProcfile, Subject: name,
					Message: "process type " + name + " becomes its own always-on service",
					Fix:     "If it is a scheduler loop, move the work to jobs with a cron schedule."})
			}
		}
		fs = append(fs, herokuFormation(&svc, name, app)...)
		frag.Services[name] = svc
	}

	fs = append(fs, herokuRelease(&frag, release)...)
	fs = append(fs, herokuScripts(&frag, app)...)
	dbFs := herokuAddons(&frag, app)
	fs = append(fs, dbFs...)
	fs = append(fs, herokuEnv(&frag, app)...)
	return frag, fs
}

func parseProcfile(files map[string][]byte) (map[string]string, string, string) {
	b, ok := lookup(files, fileProcfile)
	if !ok {
		return nil, "", ""
	}
	procs := map[string]string{}
	release, bad := "", ""
	sc := bufio.NewScanner(bytes.NewReader(b))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		m := procLineRe.FindStringSubmatch(line)
		if m == nil {
			if bad == "" {
				bad = line
			}
			continue
		}
		if m[1] == "release" {
			release = m[2]
			continue
		}
		procs[m[1]] = m[2]
	}
	return procs, release, bad
}

func herokuBuild(app appJSON) (Build, []Finding) {
	var fs []Finding
	if app.Stack == "container" {
		fs = append(fs, Finding{Code: "heroku.stack.container", Severity: SeverityInfo, Source: fileAppJSON,
			Message: "container stack builds from a Dockerfile, but Railpack is proposed because heroku.yml is not read",
			Fix:     "Set build.type to dockerfile with build.path if you ship a Dockerfile."})
	}
	var unsupported []string
	for _, bp := range app.Buildpacks {
		if !railpackBuildpacks[bp.URL] {
			unsupported = append(unsupported, bp.URL)
		}
	}
	for _, u := range unsupported {
		fs = append(fs, Finding{Code: "heroku.buildpack.unsupported", Severity: SeverityWarn, Source: fileAppJSON, Subject: u,
			Message: "no equivalent: buildpack " + u + " is not a Railpack provider",
			Fix:     "Write a Dockerfile that does what the buildpack did and use build.type dockerfile."})
	}
	if len(app.Buildpacks) > 1 && len(unsupported) == 0 {
		fs = append(fs, Finding{Code: "heroku.buildpack.multiple", Severity: SeverityWarn, Source: fileAppJSON,
			Message: "Railpack detects one provider per build, a multi-buildpack chain is not reproduced",
			Fix:     "Use a Dockerfile if the build needs more than one language."})
	}
	return Build{Type: "railpack"}, fs
}

func herokuFormation(svc *Service, name string, app appJSON) []Finding {
	f, ok := app.Formation[name]
	if !ok {
		return nil
	}
	var fs []Finding
	if f.Quantity != nil {
		if *f.Quantity > 0 {
			svc.Replicas = *f.Quantity
		} else {
			fs = append(fs, Finding{Code: "heroku.formation.zero", Severity: SeverityInfo, Source: fileAppJSON, Subject: name,
				Message: "formation sets zero dynos, the service is still proposed with the default replica count",
				Fix:     "Remove the service if it should not run."})
		}
	}
	if f.Size != "" {
		fs = append(fs, Finding{Code: "heroku.formation.size", Severity: SeverityInfo, Source: fileAppJSON, Subject: name,
			Message: "dyno size " + f.Size + " has no direct mapping",
			Fix:     "Set resources.memory from the dyno's RAM, or leave it unlimited."})
	}
	return fs
}

func herokuRelease(frag *Fragment, release string) []Finding {
	if release == "" {
		return nil
	}
	web, ok := frag.Services["web"]
	if !ok {
		return []Finding{{Code: "heroku.release.no-web", Severity: SeverityWarn, Source: fileProcfile, Subject: "release",
			Message: "release phase found but there is no web service to attach a pre-deploy command to",
			Fix:     "Run '" + release + "' by hand before the first deploy."}}
	}
	web.Hooks = &Hooks{PreDeploy: release}
	frag.Services["web"] = web
	return []Finding{{Code: "heroku.release.predeploy", Severity: SeverityWarn, Source: fileProcfile, Subject: "release",
		Message: "release phase becomes hooks.preDeploy, run in the new web container before readiness, not in a one-off dyno",
		Fix:     "Make sure the command is idempotent and needs nothing but the built image."}}
}

func herokuScripts(frag *Fragment, app appJSON) []Finding {
	var fs []Finding
	for _, k := range sortedKeys(app.Scripts) {
		cmd := app.Scripts[k]
		switch k {
		case "postdeploy":
			if web, ok := frag.Services["web"]; ok {
				if web.Hooks == nil {
					web.Hooks = &Hooks{}
				}
				web.Hooks.PostDeploy = cmd
				frag.Services["web"] = web
			}
			fs = append(fs, Finding{Code: "heroku.script.postdeploy", Severity: SeverityWarn, Source: fileAppJSON, Subject: k,
				Message: "Heroku runs postdeploy only when a review app is created, hooks.postDeploy runs on every deploy",
				Fix:     "Drop it, or make the command safe to repeat."})
		default:
			fs = append(fs, Finding{Code: "heroku.script.unsupported", Severity: SeverityInfo, Source: fileAppJSON, Subject: k,
				Message: "no equivalent: app.json script " + k + " is not translated",
				Fix:     "Move test scripts to CI and teardown work to a manual step."})
		}
	}
	return fs
}

type addonRule struct {
	engine, env string
}

var addonRules = map[string]addonRule{
	"heroku-postgresql": {"postgres", "DATABASE_URL"},
	"heroku-redis":      {"redis", "REDIS_URL"},
	"rediscloud":        {"redis", "REDIS_URL"},
	"redistogo":         {"redis", "REDIS_URL"},
	"jawsdb":            {"mysql", "DATABASE_URL"},
	"cleardb":           {"mysql", "DATABASE_URL"},
	"jawsdb-maria":      {"mariadb", "DATABASE_URL"},
	"mongolab":          {"mongodb", "MONGODB_URI"},
}

var builtinAddons = map[string]bool{"papertrail": true, "logentries": true, "loggly": true, "coralogix": true}

func herokuAddons(frag *Fragment, app appJSON) []Finding {
	var fs []Finding
	for _, raw := range app.Addons {
		plan, as := addonPlan(raw)
		if plan == "" {
			continue
		}
		name := strings.SplitN(plan, ":", 2)[0]
		switch {
		case addonRules[name].engine != "":
			r := addonRules[name]
			fs = append(fs, attachDatabase(frag, r, plan, as)...)
		case name == "scheduler":
			fs = append(fs, Finding{Code: "heroku.addon.scheduler", Severity: SeverityWarn, Source: fileAppJSON, Subject: plan,
				Message: "Heroku Scheduler jobs live in the add-on, not in app.json",
				Fix:     "Recreate each job under the service's jobs: with a cron schedule."})
		case builtinAddons[name]:
			fs = append(fs, Finding{Code: "heroku.addon.logging", Severity: SeverityInfo, Source: fileAppJSON, Subject: plan,
				Message: "logs are stored and searchable on the node, no add-on needed",
				Fix:     "Drop this add-on, or ship logs elsewhere with a drain if you must."})
		default:
			fs = append(fs, Finding{Code: "heroku.addon.manual", Severity: SeverityWarn, Source: fileAppJSON, Subject: plan,
				Message: "no equivalent: add-on " + name + " is not in the Levelrail catalog",
				Fix:     "Sign up with the vendor directly or self-host it, then set its env vars by hand."})
		}
	}
	return fs
}

func addonPlan(raw json.RawMessage) (plan, as string) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, ""
	}
	var o struct {
		Plan string `json:"plan"`
		As   string `json:"as"`
	}
	if json.Unmarshal(raw, &o) == nil {
		return o.Plan, o.As
	}
	return "", ""
}

func attachDatabase(frag *Fragment, r addonRule, plan, as string) []Finding {
	if frag.Databases == nil {
		frag.Databases = map[string]Database{}
	}
	var fs []Finding
	if as != "" && (!ValidDatabaseName(strings.ToLower(as)) || strings.Contains(as, "-")) {
		fs = append(fs, Finding{Code: "heroku.addon.name.invalid", Severity: SeverityWarn, Source: fileAppJSON, Subject: plan,
			Message: "add-on alias " + strconv.Quote(as) + " is not a valid name, the default name is used instead",
			Fix:     "Use only letters and digits, starting with a letter, in the add-on as field."})
		as = ""
	}
	name := strings.ToLower(as)
	env := r.env
	if as != "" {
		env = strings.ToUpper(as) + "_URL"
	}
	if name == "" {
		name = r.engine
		if r.engine == "postgres" {
			name = "main"
		}
	}
	base := name
	for i := 2; ; i++ {
		if _, taken := frag.Databases[name]; !taken {
			break
		}
		name = fmt.Sprintf("%s-%d", base, i)
	}
	frag.Databases[name] = Database{Engine: r.engine}
	for _, k := range sortedKeys(frag.Services) {
		svc := frag.Services[k]
		if svc.Env == nil {
			svc.Env = map[string]Env{}
		}
		svc.Env[env] = Env{From: r.engine + "." + name + ".url"}
		frag.Services[k] = svc
	}
	return append(fs, Finding{Code: "heroku.addon.database", Severity: SeverityInfo, Source: fileAppJSON, Subject: plan,
		Message: "add-on maps to a managed " + r.engine + " database named " + name + ", exposed as " + env,
		Fix:     "Dump the data from Heroku and restore it into the new database before cutover."})
}

func herokuEnv(frag *Fragment, app appJSON) []Finding {
	var fs []Finding
	for _, key := range sortedKeys(app.Env) {
		raw := app.Env[key]
		val, ok := envValue(raw)
		var ev Env
		switch {
		case ok && val.generator == "" && val.value != "":
			ev = Env{Value: val.value}
		case val.generator != "":
			ev = Env{Secret: true, Required: true}
			fs = append(fs, Finding{Code: "heroku.env.generator", Severity: SeverityInfo, Source: fileAppJSON, Subject: key,
				Message: "Heroku generated this value, Levelrail does not",
				Fix:     "Generate one (for example openssl rand -hex 32) and set it with apps secrets set."})
		case val.required:
			ev = Env{Secret: true, Required: true}
			fs = append(fs, Finding{Code: "heroku.env.required", Severity: SeverityInfo, Source: fileAppJSON, Subject: key,
				Message: "required with no default, proposed as a required secret",
				Fix:     "Set it with apps secrets set before the first deploy."})
		default:
			fs = append(fs, Finding{Code: "heroku.env.unset", Severity: SeverityInfo, Source: fileAppJSON, Subject: key,
				Message: "optional with no default, not proposed", Fix: "Add it to the service env if the app reads it."})
			continue
		}
		for _, name := range sortedKeys(frag.Services) {
			svc := frag.Services[name]
			if svc.Env == nil {
				svc.Env = map[string]Env{}
			}
			if _, taken := svc.Env[key]; !taken {
				svc.Env[key] = ev
			}
			frag.Services[name] = svc
		}
	}
	return fs
}

type envInfo struct {
	value, generator string
	required         bool
}

// envValue reads either the string or object form of an app.json env entry.
func envValue(raw json.RawMessage) (envInfo, bool) {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return envInfo{value: s}, true
	}
	var o envObj
	if json.Unmarshal(raw, &o) != nil {
		return envInfo{}, false
	}
	info := envInfo{generator: o.Generator, required: o.Required}
	if len(o.Value) > 0 {
		var vs string
		if json.Unmarshal(o.Value, &vs) == nil {
			info.value = vs
		} else {
			info.value = strings.TrimSpace(string(o.Value))
		}
	}
	return info, true
}
