package platformimport

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var flyMemoryRe = regexp.MustCompile(`^(\d+)\s*(mb|gb|mib|gib|m|g)?$`)

// flyKnownTop lists the top level fly.toml keys FlyTOMLToEmit maps or notes by
// name; any other key is reported as not imported.
var flyKnownTop = map[string]bool{
	"app": true, "primary_region": true, "build": true, "env": true, "http_service": true, "services": true,
	"vm": true, "deploy": true, "processes": true, "mounts": true, "statics": true, "checks": true,
}

// FlyTOMLToEmit converts a fly.toml into emit input. Only the file is read:
// secrets live on the platform, so none are listed and no value is fetched.
func FlyTOMLToEmit(data []byte, name, filename string) (EmitInput, error) {
	in := EmitInput{Source: "fly config " + filename}
	doc, err := parseRailwayTOML(string(data))
	if err != nil {
		return in, fmt.Errorf("%s does not parse: %w", filename, err)
	}
	if s, ok := doc["app"].(string); ok && strings.TrimSpace(s) != "" {
		name = s
	}
	app := EmitApp{Name: SanitizeName(name), PortlessOK: true}
	for _, k := range sortedAnyKeys(doc) {
		if !flyKnownTop[k] {
			in.Notes = append(in.Notes, fmt.Sprintf("top level field %s has no equivalent and was not imported", k))
		}
	}
	if r, ok := doc["primary_region"].(string); ok && r != "" {
		in.Notes = append(in.Notes, fmt.Sprintf("primary_region %s: pick the node or placement that matches it, regions do not map one to one", r))
	}
	flyBuild(&app, &in, doc["build"])
	if env, ok := doc["env"].(map[string]any); ok {
		app.Env = map[string]string{}
		for _, k := range sortedAnyKeys(env) {
			app.Env[k] = fmt.Sprint(env[k])
		}
	}
	flyServices(&app, &in, doc)
	flyVM(&app, &in, doc["vm"])
	if d, ok := doc["deploy"].(map[string]any); ok {
		for _, k := range sortedAnyKeys(d) {
			if k != "release_command" {
				in.Notes = append(in.Notes, fmt.Sprintf("deploy.%s has no equivalent and was not imported", k))
			}
		}
		if rc, _ := d["release_command"].(string); strings.TrimSpace(rc) != "" {
			if CommandHasSecret(rc) {
				in.Notes = append(in.Notes, "deploy.release_command carries an inline credential and was not kept")
			} else {
				app.PreDeploy = strings.TrimSpace(rc)
			}
		}
	}
	for _, k := range []string{"processes", "mounts", "statics", "checks"} {
		if _, ok := doc[k]; ok {
			in.Notes = append(in.Notes, fmt.Sprintf("%s has no direct equivalent and was not imported: model it with a second App, a Volume or a Job", k))
		}
	}
	in.Notes = append(in.Notes,
		"fly secrets are not in the file: list their names with fly secrets list and add them as secrets, values are never read")
	if app.Image == "" {
		in.Notes = append(in.Notes, "the app has no image: build and push one from your repository, then set it with --image-repo or in the App document")
	}
	in.Apps = append(in.Apps, app)
	return in, nil
}

func flyBuild(app *EmitApp, in *EmitInput, v any) {
	b, ok := v.(map[string]any)
	if !ok {
		return
	}
	for _, k := range sortedAnyKeys(b) {
		val := fmt.Sprint(b[k])
		switch k {
		case "image":
			app.Image = val
		case "dockerfile":
			in.Notes = append(in.Notes, fmt.Sprintf("build.dockerfile %s: use it as the Dockerfile path of the App build", val))
		case "builder", "buildpacks":
			in.Notes = append(in.Notes, fmt.Sprintf("build.%s %s: choose the build type (dockerfile or railpack) when you create the build from the repository", k, val))
		case "args":
			in.Notes = append(in.Notes, "build.args were not imported: pass them as build args of the App build, never as secrets")
		default:
			in.Notes = append(in.Notes, fmt.Sprintf("build.%s has no equivalent and was not imported", k))
		}
	}
}

// flyServices takes the port and health path from http_service first, then
// from the first [[services]] entry, and reports what it left out.
func flyServices(app *EmitApp, in *EmitInput, doc map[string]any) {
	var entries []map[string]any
	if h, ok := doc["http_service"].(map[string]any); ok {
		entries = append(entries, h)
		if v, ok := h["force_https"].(bool); ok && v {
			in.Notes = append(in.Notes, "http_service.force_https: HTTPS redirect is a domain setting, enable it on the domain")
		}
		if n := railwayInt(h["min_machines_running"]); n > 0 {
			app.Replicas = n
		}
		for _, k := range []string{"auto_stop_machines", "auto_start_machines"} {
			if _, ok := h[k]; ok {
				in.Notes = append(in.Notes, fmt.Sprintf("http_service.%s has no equivalent: apps keep running, use a Job or scale to zero by hand", k))
			}
		}
	}
	if arr, ok := doc["services"].([]any); ok {
		for _, e := range arr {
			if m, ok := e.(map[string]any); ok {
				entries = append(entries, m)
			}
		}
	}
	for _, e := range entries {
		p := railwayInt(e["internal_port"])
		if p > 0 && app.Port == 0 {
			app.Port = p
		} else if p > 0 && p != app.Port {
			in.Notes = append(in.Notes, fmt.Sprintf("service on internal port %d was not imported: an App has one HTTP port, add a second App for it", p))
		}
		if app.HealthPath == "" {
			app.HealthPath = flyCheckPath(e)
		}
	}
}

func flyCheckPath(e map[string]any) string {
	for _, key := range []string{"checks", "http_checks"} {
		arr, _ := e[key].([]any)
		for _, c := range arr {
			if m, ok := c.(map[string]any); ok {
				if p, _ := m["path"].(string); strings.HasPrefix(p, "/") {
					return p
				}
			}
		}
	}
	return ""
}

func flyVM(app *EmitApp, in *EmitInput, v any) {
	arr, ok := v.([]any)
	if !ok || len(arr) == 0 {
		return
	}
	vm, ok := arr[0].(map[string]any)
	if !ok {
		return
	}
	if len(arr) > 1 {
		in.Notes = append(in.Notes, "several [[vm]] tables: only the first one was imported")
	}
	if n := railwayInt(vm["cpus"]); n > 0 {
		app.NanoCPUs = int64(n) * 1_000_000_000
	}
	switch mem := vm["memory"].(type) {
	case string:
		if m := flyMemoryRe.FindStringSubmatch(strings.ToLower(strings.TrimSpace(mem))); m != nil {
			n, _ := strconv.Atoi(m[1])
			if strings.HasPrefix(m[2], "g") {
				n *= 1024
			}
			app.Memory = fmt.Sprintf("%dMi", n)
		} else {
			in.Notes = append(in.Notes, fmt.Sprintf("vm.memory %q is not a size this importer reads, set resources.memory by hand", mem))
		}
	case float64:
		app.Memory = fmt.Sprintf("%dMi", int(mem))
	}
	for _, k := range []string{"size", "cpu_kind", "memory_mb"} {
		if val, ok := vm[k]; ok {
			if k == "memory_mb" && app.Memory == "" {
				app.Memory = fmt.Sprintf("%dMi", railwayInt(val))
				continue
			}
			in.Notes = append(in.Notes, fmt.Sprintf("vm.%s %v: set cpu and memory on the App to match", k, val))
		}
	}
}
