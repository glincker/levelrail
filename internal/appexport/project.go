package appexport

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ProjectInput is a whole project: every app service filed under it, keyed
// uniquely across apps, plus the databases that this export leaves out.
type ProjectInput struct {
	Name     string
	Services []Service
	// Databases are managed database names that are not exported as services.
	Databases []string
	// IncludeSecrets also writes a .env holding Service.SecretValues. The
	// default is placeholders only.
	IncludeSecrets bool
}

// volumeManifest is volumes.json: what data the compose file expects and
// where it came from, so a user can move it by hand.
type volumeManifest struct {
	Project string         `json:"project"`
	Volumes []manifestVol  `json:"volumes"`
	Binds   []manifestBind `json:"bind_mounts"`
	Note    string         `json:"note"`
}

type manifestVol struct {
	Service       string `json:"service"`
	ContainerPath string `json:"container_path"`
	LevelrailName string `json:"levelrail_volume"`
	ComposeName   string `json:"compose_volume"`
}

type manifestBind struct {
	Service       string `json:"service"`
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
	ReadOnly      bool   `json:"read_only"`
}

// RenderProject composes a project into one docker-compose.yml, .env.example
// and volumes.json (and a .env only when secrets were explicitly included).
// Whatever cannot be expressed is listed in the compose header.
func RenderProject(in ProjectInput) ([]File, error) {
	if in.Name == "" || len(in.Services) == 0 {
		return nil, fmt.Errorf("appexport: a project name and at least one service are required")
	}
	all := append([]Service(nil), in.Services...)
	sort.Slice(all, func(i, j int) bool { return all[i].Key < all[j].Key })
	present := map[string]bool{}
	var svcs []Service
	var gaps []string
	for _, s := range all {
		if s.Key == "" || present[s.Key] {
			return nil, fmt.Errorf("appexport: service %q needs a unique key", s.Key)
		}
		present[s.Key] = true
		// One app without an image must not sink the whole project export.
		if s.Image == "" {
			gaps = append(gaps, s.Key+": skipped, the app has no image to run")
			continue
		}
		svcs = append(svcs, s)
	}
	if len(svcs) == 0 {
		return nil, fmt.Errorf("appexport: no service with an image to export")
	}
	for k := range present {
		present[k] = false
	}
	for _, s := range svcs {
		present[s.Key] = true
	}
	if err := validateNames(svcs); err != nil {
		return nil, err
	}
	gaps = append(gaps, collectGaps(svcs, in.Databases, present, in.IncludeSecrets)...)
	sort.Strings(gaps)
	for i := range svcs {
		svcs[i].DependsOn = keepPresent(svcs[i].DependsOn, present)
		svcs[i].DependsOnHealthy = keepPresent(svcs[i].DependsOnHealthy, present)
	}
	vars := planVars(svcs)
	compose := renderCompose(in.Name, svcs, vars)
	compose = strings.Replace(compose, "# Exported from Levelrail app", "# Exported from Levelrail project", 1)
	compose = projectHeader(gaps) + compose
	manifest, err := renderVolumeManifest(in.Name, svcs)
	if err != nil {
		return nil, err
	}
	readme := renderReadme(in.Name, svcs)
	files := []File{
		{Name: ".env.example", Content: []byte(renderEnv(svcs, vars))},
		{Name: "README.md", Content: []byte(readme)},
		{Name: "docker-compose.yml", Content: []byte(compose)},
		{Name: "volumes.json", Content: manifest},
	}
	if in.IncludeSecrets {
		if env, ok := revealedEnv(svcs, vars); ok {
			files = append(files, File{Name: ".env", Content: []byte(env)})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Name < files[j].Name })
	return files, nil
}

func keepPresent(deps []string, present map[string]bool) []string {
	var out []string
	for _, d := range deps {
		if present[d] {
			out = append(out, d)
		}
	}
	return out
}

// revealedEnv swaps in opt-in plaintext for secret keys that have one.
// Keys without a stored value (vault or database references) stay placeholders.
func revealedEnv(svcs []Service, vars map[string][]envVar) (string, bool) {
	out := map[string][]envVar{}
	found := false
	for _, s := range svcs {
		for _, v := range vars[s.Key] {
			if val, ok := s.SecretValues[v.Key]; ok && v.Secret {
				v.Value, v.Secret, v.Note = val, false, ""
				found = true
			}
			out[s.Key] = append(out[s.Key], v)
		}
	}
	if !found {
		return "", false
	}
	env := renderEnv(svcs, out)
	return strings.Replace(env, "# Copy to .env and fill in", "# Contains secret values, never commit it. Fill in any CHANGE_ME left in", 1), true
}

// collectGaps lists everything the compose output cannot carry, so the header
// never hides a loss.
func collectGaps(svcs []Service, databases []string, present map[string]bool, includeSecrets bool) []string {
	var gaps []string
	for _, s := range svcs {
		if s.Health == nil {
			gaps = append(gaps, s.Key+": no readiness probe is configured, so no healthcheck is written")
		} else if !healthExpressible(s) {
			gaps = append(gaps, s.Key+": the readiness probe cannot be written as a compose healthcheck")
		}
		for _, d := range s.DependsOn {
			if !present[d] {
				gaps = append(gaps, fmt.Sprintf("%s: depends_on %s is outside this export and was removed", s.Key, d))
			}
		}
		if len(s.FilePaths) > 0 {
			gaps = append(gaps, fmt.Sprintf("%s: managed config files are not exported (%s); recreate them as bind mounts", s.Key, strings.Join(sortedCopy(s.FilePaths), ", ")))
		}
		if len(s.Refs) > 0 {
			gaps = append(gaps, s.Key+": database and vault references became CHANGE_ME placeholders")
		}
		if len(s.Domains) > 0 {
			gaps = append(gaps, s.Key+": domains and TLS are kept as x-domains only; set up your own proxy")
		}
		if includeSecrets {
			for _, k := range s.SecretEnv {
				if _, ok := s.SecretValues[k]; !ok {
					gaps = append(gaps, fmt.Sprintf("%s: secret %s has no exportable value and stays CHANGE_ME in .env", s.Key, k))
				}
			}
		}
		for _, u := range s.Unsupported {
			gaps = append(gaps, s.Key+": "+oneLine(u))
		}
	}
	if len(databases) > 0 {
		gaps = append(gaps, "managed databases are not exported, dump them separately: "+strings.Join(sortedCopy(databases), ", "))
	}
	sort.Strings(gaps)
	return gaps
}

func healthExpressible(s Service) bool {
	h := s.Health
	return len(h.Exec) > 0 || (h.TCP && s.Port > 0) || (h.Path != "" && s.Port > 0)
}

func projectHeader(gaps []string) string {
	if len(gaps) == 0 {
		return "# Nothing was left out of this export.\n"
	}
	var b strings.Builder
	b.WriteString("# Not carried over, review before you rely on this file:\n")
	for _, g := range gaps {
		fmt.Fprintf(&b, "#   - %s\n", g)
	}
	return b.String()
}

func renderVolumeManifest(project string, svcs []Service) ([]byte, error) {
	m := volumeManifest{
		Project: projectName(project), Volumes: []manifestVol{}, Binds: []manifestBind{},
		Note: "Named volumes start empty. Copy each Levelrail volume's contents into the compose volume before first start.",
	}
	for _, s := range svcs {
		for _, v := range s.Volumes {
			m.Volumes = append(m.Volumes, manifestVol{Service: s.Key, ContainerPath: v.ContainerPath, LevelrailName: v.Name, ComposeName: m.Project + "_" + volumeKey(v.Name)})
		}
		for _, b := range s.BindMounts {
			m.Binds = append(m.Binds, manifestBind{Service: s.Key, HostPath: b.HostPath, ContainerPath: b.ContainerPath, ReadOnly: b.ReadOnly})
		}
	}
	sort.Slice(m.Volumes, func(i, j int) bool {
		if m.Volumes[i].Service != m.Volumes[j].Service {
			return m.Volumes[i].Service < m.Volumes[j].Service
		}
		return m.Volumes[i].ContainerPath < m.Volumes[j].ContainerPath
	})
	sort.Slice(m.Binds, func(i, j int) bool {
		if m.Binds[i].Service != m.Binds[j].Service {
			return m.Binds[i].Service < m.Binds[j].Service
		}
		return m.Binds[i].ContainerPath < m.Binds[j].ContainerPath
	})
	b, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// oneLine keeps a caller note from breaking out of a YAML comment line.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
