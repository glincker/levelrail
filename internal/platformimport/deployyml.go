package platformimport

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// deployYMLNoted are deploy.yml keys with no IaC equivalent. They are
// reported in the notes instead of dropped silently.
var deployYMLNoted = map[string]string{
	"registry":            "registry settings are not imported, add pull credentials to the target server",
	"ssh":                 "ssh settings are not imported, Levelrail deploys through its own agent",
	"builder":             "builder settings are not imported, build and push an image first",
	"boot":                "boot strategy is not imported, use the App rollout settings",
	"volumes":             "volume mounts are not imported, add storage for them",
	"asset_path":          "asset bridging is not imported",
	"healthcheck":         "top level healthcheck is not imported, set the proxy healthcheck path or an App health probe",
	"primary_role":        "primary_role is not imported",
	"retain_containers":   "retain_containers is not imported",
	"readiness_delay":     "readiness_delay is not imported",
	"deploy_timeout":      "deploy_timeout is not imported",
	"drain_timeout":       "drain_timeout is not imported",
	"logging":             "logging options are not imported",
	"aliases":             "aliases are CLI shortcuts and are not imported",
	"labels":              "labels are not imported",
	"hooks_path":          "deploy hooks are not imported, use App pre and post deploy hooks",
	"run_directory":       "run_directory is not imported",
	"minimum_version":     "minimum_version is not imported",
	"require_destination": "require_destination is not imported",
}

var deployYMLHandled = map[string]bool{
	"service": true, "image": true, "servers": true, "proxy": true, "env": true, "accessories": true,
}

// DeployYMLToEmit converts a deployment tool deploy.yml into emit input.
// Clear env values go through the emit redaction; secret entries stay names.
func DeployYMLToEmit(data []byte) (EmitInput, error) {
	in := EmitInput{Source: "deploy.yml"}
	if strings.Contains(string(data), "<%") {
		return in, fmt.Errorf("deploy.yml uses ERB tags: render the file first, the importer does not evaluate templates")
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return in, fmt.Errorf("deploy.yml does not parse: %w", err)
	}
	var unsupported []string
	for _, k := range sortedAnyKeys(doc) {
		if note, ok := deployYMLNoted[k]; ok {
			in.Notes = append(in.Notes, note)
		} else if !deployYMLHandled[k] {
			unsupported = append(unsupported, k)
		}
	}
	if len(unsupported) > 0 {
		return in, fmt.Errorf("deploy.yml has fields the importer does not support: %s", strings.Join(unsupported, ", "))
	}
	service, _ := doc["service"].(string)
	image, _ := doc["image"].(string)
	if service == "" || image == "" {
		return in, fmt.Errorf("deploy.yml needs both service and image")
	}
	base := EmitApp{Name: SanitizeName(service), Image: image}
	if !strings.Contains(image[strings.LastIndex(image, "/")+1:], ":") {
		in.Notes = append(in.Notes, fmt.Sprintf("image %s has no tag: the source tool appended the release version, pin a tag before apply", image))
	}
	if err := deployYMLEnv(&base, doc["env"], "env"); err != nil {
		return in, err
	}
	if err := deployYMLProxy(&base, doc["proxy"], &in); err != nil {
		return in, err
	}
	roles, err := deployYMLRoles(doc["servers"])
	if err != nil {
		return in, err
	}
	in.Apps = append(in.Apps, base)
	for _, r := range roles {
		if r.name == "web" && len(r.cmd) == 0 {
			continue
		}
		app := base
		app.Name = SanitizeName(service + "-" + r.name)
		app.Domains, app.Port, app.HealthPath = nil, 0, ""
		app.Command = r.cmd
		in.Apps = append(in.Apps, app)
		in.Notes = append(in.Notes, fmt.Sprintf("role %s became App %s: hosts are not imported, place it on a server", r.name, app.Name))
	}
	in.Notes = append(in.Notes, "server hosts in servers: are not imported, place the Apps on servers in Levelrail")
	return in, deployYMLAccessories(doc["accessories"], &in)
}

type deployRole struct {
	name string
	cmd  []string
}

func deployYMLRoles(v any) ([]deployRole, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return nil, nil
	}
	var out []deployRole
	for _, name := range sortedAnyKeys(m) {
		r := deployRole{name: name}
		if rm, ok := m[name].(map[string]any); ok {
			for k := range rm {
				if k != "hosts" && k != "cmd" {
					return nil, fmt.Errorf("deploy.yml servers.%s.%s is not supported", name, k)
				}
			}
			if c, ok := rm["cmd"].(string); ok && c != "" {
				r.cmd = []string{"/bin/sh", "-c", c}
			}
		}
		out = append(out, r)
	}
	return out, nil
}

func deployYMLProxy(app *EmitApp, v any, in *EmitInput) error {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("deploy.yml proxy must be a mapping")
	}
	for _, k := range sortedAnyKeys(m) {
		switch k {
		case "host":
			if s, ok := m[k].(string); ok && s != "" {
				app.Domains = append(app.Domains, s)
			}
		case "hosts":
			if l, ok := m[k].([]any); ok {
				for _, h := range l {
					if s, ok := h.(string); ok {
						app.Domains = append(app.Domains, s)
					}
				}
			}
		case "app_port":
			app.Port = anyInt(m[k])
		case "healthcheck":
			if hm, ok := m[k].(map[string]any); ok {
				app.HealthPath, _ = hm["path"].(string)
			}
		case "ssl":
			in.Notes = append(in.Notes, "proxy ssl is not imported: Levelrail issues certificates for each domain itself")
		default:
			return fmt.Errorf("deploy.yml proxy.%s is not supported", k)
		}
	}
	return nil
}

func deployYMLEnv(app *EmitApp, v any, path string) error {
	if v == nil {
		return nil
	}
	m, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("deploy.yml %s must be a mapping", path)
	}
	vals := map[string]any{}
	for _, k := range sortedAnyKeys(m) {
		switch k {
		case "clear":
			if cm, ok := m[k].(map[string]any); ok {
				for ck, cv := range cm {
					vals[ck] = cv
				}
			}
		case "secret":
			l, _ := m[k].([]any)
			for _, n := range l {
				if s, ok := n.(string); ok {
					app.Secrets = append(app.Secrets, EmitSecret{Name: s, Hint: "listed under " + path + ".secret, value not read"})
				}
			}
		case "tags":
			return fmt.Errorf("deploy.yml %s.tags is not supported", path)
		default:
			// A plain KEY: value map is the clear form.
			vals[k] = m[k]
		}
	}
	app.Env = map[string]string{}
	for k, val := range vals {
		app.Env[k] = fmt.Sprint(val)
	}
	return nil
}

func deployYMLAccessories(v any, in *EmitInput) error {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	for _, name := range sortedAnyKeys(m) {
		am, ok := m[name].(map[string]any)
		if !ok {
			return fmt.Errorf("deploy.yml accessories.%s must be a mapping", name)
		}
		app := EmitApp{Name: SanitizeName(name)}
		for _, k := range sortedAnyKeys(am) {
			switch k {
			case "image":
				app.Image, _ = am[k].(string)
			case "env":
				if err := deployYMLEnv(&app, am[k], "accessories."+name+".env"); err != nil {
					return err
				}
			case "port":
				p := strings.Split(fmt.Sprint(am[k]), ":")
				app.Port = anyInt(p[len(p)-1])
			case "cmd":
				if c, ok := am[k].(string); ok && c != "" {
					app.Command = []string{"/bin/sh", "-c", c}
				}
			case "host", "hosts", "roles":
				in.Notes = append(in.Notes, fmt.Sprintf("accessory %s: server placement is not imported", name))
			case "directories", "files", "volumes":
				app.Notes = append(app.Notes, fmt.Sprintf("accessory %s %s are not imported: add storage for them", name, k))
			default:
				return fmt.Errorf("deploy.yml accessories.%s.%s is not supported", name, k)
			}
		}
		if app.Image == "" {
			return fmt.Errorf("deploy.yml accessories.%s needs an image", name)
		}
		in.Apps = append(in.Apps, app)
	}
	return nil
}

func anyInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case string:
		n, _ := strconv.Atoi(strings.TrimSpace(t))
		return n
	}
	return 0
}

func sortedAnyKeys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
