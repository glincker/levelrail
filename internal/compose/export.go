package compose

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/store"
	"gopkg.in/yaml.v3"
)

// secretPlaceholderKind is never a generatable kind (magicvars.go), so
// wrapping a captured key in it always leaves the var unresolved
// instead of risking a key name like "PASSWORD_SALT" being auto-filled
// with a random value.
const secretPlaceholderKind = "SECRET"

// composeExportDoc is FromDesiredServices' marshal target. No top-level
// volumes: block: File (yaml.go) never reads one back.
type composeExportDoc struct {
	Version  string                    `yaml:"version"`
	Services map[string]map[string]any `yaml:"services"`
}

// FromDesiredServices builds a compose.yaml that reconstructs appName's
// services as closely as this package's File/Service model allows.
// Secret, database, and vault env values never carry over as real
// values, only a ${SERVICE_SECRET_<key>} placeholder (no secret value
// is ever captured, the whole point of this function). Domains,
// HostPort/BindAddress, bind mounts, resources, health checks, and
// egress policy are dropped too: none of those round-trips through this
// package's own compose model (see compose.go's doc comment).
func FromDesiredServices(appName string, services []store.DesiredService) (string, error) {
	if len(services) == 0 {
		return "", fmt.Errorf("compose: export %q: no services to export", appName)
	}

	out := make(map[string]map[string]any, len(services))
	for _, d := range services {
		key := strings.TrimPrefix(d.Name, appName+"-")
		if key == "" {
			key = d.Name
		}
		out[key] = exportServiceBody(appName, key, d)
	}

	doc := composeExportDoc{Version: "3.8", Services: out}
	body, err := yaml.Marshal(doc)
	if err != nil {
		return "", fmt.Errorf("compose: export %q: marshal: %w", appName, err)
	}
	return string(body), nil
}

func exportServiceBody(appName, serviceKey string, d store.DesiredService) map[string]any {
	body := map[string]any{"image": d.Image}
	if d.Port > 0 {
		body["ports"] = []string{strconv.Itoa(d.Port)}
	}
	if env := exportEnv(d); len(env) > 0 {
		body["environment"] = env
	}
	if len(d.Labels) > 0 {
		body["labels"] = d.Labels
	}
	if len(d.Command) > 0 {
		body["command"] = []string(d.Command)
	}
	if len(d.Entrypoint) > 0 {
		body["entrypoint"] = []string(d.Entrypoint)
	}
	if d.PullPolicy != "" {
		body["pull_policy"] = d.PullPolicy
	}
	if len(d.DependsOn) > 0 {
		body["depends_on"] = []string(d.DependsOn)
	}
	if vols := exportVolumes(appName, serviceKey, d.Volumes); len(vols) > 0 {
		body["volumes"] = vols
	}
	if deploy := exportDeploy(d); len(deploy) > 0 {
		body["deploy"] = deploy
	}
	return body
}

// exportEnv merges plain env literals with a placeholder per
// secret/database/vault-backed key (see FromDesiredServices).
func exportEnv(d store.DesiredService) map[string]string {
	n := len(d.Env) + len(d.SecretEnv) + len(d.DatabaseEnv) + len(d.VaultEnv)
	if n == 0 {
		return nil
	}
	out := make(map[string]string, n)
	for k, v := range d.Env {
		out[k] = v
	}
	for _, ref := range d.SecretEnv {
		out[ref.Name] = secretPlaceholder(ref.Name)
	}
	for k := range d.DatabaseEnv {
		out[k] = secretPlaceholder(k)
	}
	for k := range d.VaultEnv {
		out[k] = secretPlaceholder(k)
	}
	return out
}

func secretPlaceholder(key string) string {
	return "${SERVICE_" + secretPlaceholderKind + "_" + key + "}"
}

// exportVolumes reverses volumeName (translate.go): strips the
// "app-<appName>-<serviceKey>-" prefix back off to recover the logical
// volume name a fresh ToDesiredServices call would re-derive.
func exportVolumes(appName, serviceKey string, vols []store.ServiceVolume) []string {
	if len(vols) == 0 {
		return nil
	}
	prefix := "app-" + appName + "-" + serviceKey + "-"
	out := make([]string, 0, len(vols))
	for _, v := range vols {
		logical := strings.TrimPrefix(v.Name, prefix)
		if logical == "" {
			logical = v.Name
		}
		out = append(out, logical+":"+v.ContainerPath)
	}
	return out
}

// exportDeploy reverses gpuFromDeploy/replicasFromDeploy, the only two
// deploy: subkeys this package reads back (Deploy's own doc comment).
func exportDeploy(d store.DesiredService) map[string]any {
	out := map[string]any{}
	if d.Replicas > 1 {
		out["replicas"] = d.Replicas
	}
	if d.Resources != nil && d.Resources.GPU != nil {
		out["resources"] = map[string]any{
			"reservations": map[string]any{
				"devices": []map[string]any{exportGPUDevice(*d.Resources.GPU)},
			},
		}
	}
	return out
}

func exportGPUDevice(g store.ServiceGPU) map[string]any {
	dev := map[string]any{"driver": "nvidia", "capabilities": []string{"gpu"}}
	if len(g.DeviceIDs) > 0 {
		dev["device_ids"] = g.DeviceIDs
		return dev
	}
	if g.Count > 0 {
		dev["count"] = strconv.Itoa(g.Count)
	} else {
		dev["count"] = "all"
	}
	return dev
}
