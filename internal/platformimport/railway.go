package platformimport

import (
	"encoding/json"
	"fmt"
	"strings"
)

// railwayBuildKeys and railwayDeployKeys are the config-as-code fields handled
// by RailwayToEmit; every other key is reported, never dropped silently.
var railwayBuildKeys = map[string]bool{
	"builder": true, "buildCommand": true, "watchPatterns": true, "dockerfilePath": true, "nixpacksConfigPath": true,
}

var railwayDeployKeys = map[string]bool{
	"startCommand": true, "healthcheckPath": true, "healthcheckTimeout": true, "restartPolicyType": true,
	"restartPolicyMaxRetries": true, "cronSchedule": true, "numReplicas": true, "preDeployCommand": true,
}

// RailwayToEmit converts a railway.json or railway.toml into emit input. The
// file holds build and deploy settings only: variable values are never read,
// and a top-level variables table contributes names only.
func RailwayToEmit(data []byte, name, filename string) (EmitInput, error) {
	in := EmitInput{Source: "railway config " + filename}
	var doc map[string]any
	if strings.HasSuffix(strings.ToLower(filename), ".toml") {
		d, err := parseRailwayTOML(string(data))
		if err != nil {
			return in, fmt.Errorf("%s does not parse: %w", filename, err)
		}
		doc = d
	} else if err := json.Unmarshal(data, &doc); err != nil {
		return in, fmt.Errorf("%s does not parse: %w", filename, err)
	}
	build, err := railwaySection(doc, "build")
	if err != nil {
		return in, err
	}
	deploy, err := railwaySection(doc, "deploy")
	if err != nil {
		return in, err
	}
	app := EmitApp{Name: SanitizeName(name), PortlessOK: true}
	for _, k := range sortedAnyKeys(doc) {
		switch k {
		case "build", "deploy", "$schema":
		case "variables":
			railwayVariables(&app, doc[k])
		default:
			in.Notes = append(in.Notes, fmt.Sprintf("top level field %s has no equivalent and was not imported", k))
		}
	}
	railwayBuildNotes(&in, build)
	if err := railwayDeploy(&app, &in, deploy); err != nil {
		return in, err
	}
	in.Notes = append(in.Notes,
		"railway keeps service variables outside the config file: list their names with the railway CLI and add them as secrets, values are never read",
		"the app has no image: build and push one from your repository, then set it with --image-repo or in the App document")
	in.Apps = append(in.Apps, app)
	return in, nil
}

func railwaySection(doc map[string]any, key string) (map[string]any, error) {
	v, ok := doc[key]
	if !ok || v == nil {
		return nil, nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("railway config %s must be an object", key)
	}
	return m, nil
}

func railwayVariables(app *EmitApp, v any) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	for _, k := range sortedAnyKeys(m) {
		app.Secrets = append(app.Secrets, EmitSecret{Name: k, Hint: "listed under variables, value not read"})
	}
}

func railwayBuildNotes(in *EmitInput, build map[string]any) {
	for _, k := range sortedAnyKeys(build) {
		val := fmt.Sprint(build[k])
		switch {
		case !railwayBuildKeys[k]:
			in.Notes = append(in.Notes, fmt.Sprintf("build.%s has no equivalent and was not imported", k))
		case k == "buildCommand" && CommandHasSecret(val):
			in.Notes = append(in.Notes, "build.buildCommand carries an inline credential and was not kept: move it to a build secret")
		case k == "builder":
			in.Notes = append(in.Notes, fmt.Sprintf("build.builder %s: apply runs prebuilt images, so choose the build type (dockerfile, railpack or static) when you create the build from the repository", val))
		case k == "buildCommand":
			in.Notes = append(in.Notes, fmt.Sprintf("build.buildCommand %q: set it as the build command of the App build, apply does not run builds", val))
		case k == "dockerfilePath":
			in.Notes = append(in.Notes, fmt.Sprintf("build.dockerfilePath %s: use it as the Dockerfile path of the App build", val))
		default:
			in.Notes = append(in.Notes, fmt.Sprintf("build.%s has no equivalent and was not imported", k))
		}
	}
}

func railwayDeploy(app *EmitApp, in *EmitInput, deploy map[string]any) error {
	for _, k := range sortedAnyKeys(deploy) {
		if !railwayDeployKeys[k] {
			in.Notes = append(in.Notes, fmt.Sprintf("deploy.%s has no equivalent and was not imported", k))
		}
	}
	str := func(k string) string { s, _ := deploy[k].(string); return strings.TrimSpace(s) }
	cmd := str("startCommand")
	switch {
	case cmd == "":
	case CommandHasSecret(cmd):
		in.Notes = append(in.Notes, "deploy.startCommand carries an inline credential and was not kept: read it from a secret instead")
	default:
		app.Command = []string{"/bin/sh", "-c", cmd}
	}
	app.HealthPath = str("healthcheckPath")
	if n := railwayInt(deploy["healthcheckTimeout"]); n > 0 {
		in.Notes = append(in.Notes, fmt.Sprintf("deploy.healthcheckTimeout %ds: set the health probe timeout and retries on the App to match", n))
	}
	if n := railwayInt(deploy["numReplicas"]); n > 0 {
		app.Replicas = n
	}
	if pre := railwayCommand(deploy["preDeployCommand"]); pre != "" {
		if CommandHasSecret(pre) {
			in.Notes = append(in.Notes, "deploy.preDeployCommand carries an inline credential and was not kept")
		} else {
			app.PreDeploy = pre
		}
	}
	if t := str("restartPolicyType"); t != "" {
		in.Notes = append(in.Notes, fmt.Sprintf("deploy.restartPolicyType %s (max retries %d): restart handling follows the App rollout and health settings, no per-policy field exists", t, railwayInt(deploy["restartPolicyMaxRetries"])))
	}
	if sched := str("cronSchedule"); sched != "" {
		if app.Command == nil {
			return fmt.Errorf("deploy.cronSchedule needs a usable deploy.startCommand: a Railway cron service runs its start command on the schedule")
		}
		app.Jobs = append(app.Jobs, EmitJob{Name: SanitizeJobName(app.Name), Schedule: NormalizeSchedule(sched), Command: app.Command})
		app.Command = nil
		in.Notes = append(in.Notes, "deploy.cronSchedule became a scheduled job: Railway evaluates cron in UTC, set the job timezone to UTC")
	}
	return nil
}

func railwayCommand(v any) string {
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	case []any:
		var parts []string
		for _, p := range t {
			parts = append(parts, strings.TrimSpace(fmt.Sprint(p)))
		}
		return strings.Join(parts, " && ")
	}
	return ""
}

func railwayInt(v any) int {
	if f, ok := v.(float64); ok {
		return int(f)
	}
	return anyInt(v)
}
