package platformimport

import (
	"fmt"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
	"github.com/GLINCKER/levelrail/internal/spec"
)

// NetlifyTOMLToEmit converts a netlify.toml into a static site App: the build
// command, publish directory and base, redirects and headers. Everything else
// is reported, never dropped silently. Build environment values that look like
// secrets are not kept, because build args end up in public output.
func NetlifyTOMLToEmit(data []byte, name, filename string) (EmitInput, error) {
	in := EmitInput{Source: "netlify config " + filename}
	doc, err := parseRailwayTOML(string(data))
	if err != nil {
		return in, fmt.Errorf("%s does not parse: %w", filename, err)
	}
	st := &EmitStatic{}
	app := EmitApp{Name: SanitizeName(name), Static: st}
	build, _ := doc["build"].(map[string]any)
	for _, k := range sortedAnyKeys(build) {
		switch k {
		case "command":
			cmd := strings.TrimSpace(fmt.Sprint(build[k]))
			if CommandHasSecret(cmd) {
				in.Notes = append(in.Notes, "build.command carries an inline credential and was not kept: move it to a build secret")
			} else {
				st.Command = cmd
			}
		case "publish":
			st.Publish = strings.TrimSpace(fmt.Sprint(build[k]))
		case "base":
			st.BaseDirectory = strings.TrimSpace(fmt.Sprint(build[k]))
		case "environment":
			netlifyBuildEnv(st, &in, build[k])
		default:
			in.Notes = append(in.Notes, fmt.Sprintf("build.%s has no equivalent and was not imported", k))
		}
	}
	if st.Publish != "" && st.Command == "" {
		in.Notes = append(in.Notes, "build.publish without build.command: serve the committed directory by setting build.path on the App build, publish needs a command")
		st.Publish = ""
	}
	if st.Command != "" && st.Publish == "" {
		in.Notes = append(in.Notes, "build.command has no build.publish: set the output directory, a static build needs one")
	}
	limits := spec.DefaultStaticLimits()
	netlifyRedirects(st, &in, doc["redirects"], limits)
	netlifyHeaders(st, &in, doc["headers"], limits)
	for _, k := range sortedAnyKeys(doc) {
		switch k {
		case "build", "redirects", "headers":
		case "context":
			in.Notes = append(in.Notes, "context overrides were not imported: add contexts to the App for deploy-preview and branch-deploy, see the migration guide")
		default:
			in.Notes = append(in.Notes, fmt.Sprintf("top level table %s has no equivalent and was not imported", k))
		}
	}
	in.Notes = append(in.Notes, "the App is a static site with no source: connect the repository and set the build type to static when you create the build")
	in.Apps = append(in.Apps, app)
	return in, nil
}

func netlifyBuildEnv(st *EmitStatic, in *EmitInput, v any) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	for _, k := range sortedAnyKeys(m) {
		val := fmt.Sprint(m[k])
		if envredact.SensitiveName(k) || envredact.SensitiveValue(k, val) {
			in.Notes = append(in.Notes, fmt.Sprintf("build.environment.%s looks like a secret and was not kept: build args are public", k))
			continue
		}
		if st.Args == nil {
			st.Args = map[string]string{}
		}
		st.Args[k] = val
	}
}

func netlifyRedirects(st *EmitStatic, in *EmitInput, v any, l spec.StaticLimits) {
	arr, _ := v.([]any)
	for i, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		str := func(k string) string { s, _ := m[k].(string); return strings.TrimSpace(s) }
		label := fmt.Sprintf("redirects entry %d (%s)", i+1, str("from"))
		if _, has := m["conditions"]; has {
			in.Notes = append(in.Notes, label+" has conditions (country, language, role, cookie), they need an edge runtime: the rule was skipped")
			continue
		}
		r := spec.Route{From: str("from"), To: str("to"), Status: railwayInt(m["status"]), Force: m["force"] == true}
		if q, ok := m["query"].(map[string]any); ok && len(q) > 0 {
			r.Query = map[string]string{}
			for k, val := range q {
				r.Query[k] = fmt.Sprint(val)
			}
		}
		rule := spec.StaticRule{From: r.From, To: r.To, Status: r.Status, Force: r.Force}
		for _, k := range sortedKeysOf(r.Query) {
			rule.Query = append(rule.Query, spec.StaticQueryMatch{Key: k, Value: r.Query[k]})
		}
		if err := spec.ValidateStaticRule(&rule, l); err != nil {
			in.Notes = append(in.Notes, fmt.Sprintf("%s was skipped: %v", label, err))
			continue
		}
		if rule.Status != r.Status {
			r.Status = rule.Status
		}
		st.Routes = append(st.Routes, r)
	}
}

func netlifyHeaders(st *EmitStatic, in *EmitInput, v any, l spec.StaticLimits) {
	arr, _ := v.([]any)
	for i, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		path, _ := m["for"].(string)
		h := spec.HeaderRule{Path: strings.TrimSpace(path), Values: map[string]string{}}
		vals, _ := m["values"].(map[string]any)
		for _, k := range sortedAnyKeys(vals) {
			h.Values[k] = strings.TrimSpace(fmt.Sprint(vals[k]))
		}
		block := spec.StaticHeaderBlock{Path: h.Path}
		for _, k := range sortedKeysOf(h.Values) {
			block.Set = append(block.Set, spec.StaticHeaderKV{Name: k, Value: h.Values[k]})
		}
		if err := spec.ValidateStaticHeaderBlock(&block, l); err != nil {
			in.Notes = append(in.Notes, fmt.Sprintf("headers entry %d (%s) was skipped: %v", i+1, path, err))
			continue
		}
		st.Headers = append(st.Headers, h)
	}
}
