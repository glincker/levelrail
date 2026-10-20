package proxyroutes

import (
	"fmt"
	"path"
	"strings"

	"gopkg.in/yaml.v3"
)

// traefikStaticDefaults are where Traefik looks for its static configuration
// when --configFile is not given (besides $HOME and the working directory).
var traefikStaticDefaults = []string{"/etc/traefik/traefik.yml", "/etc/traefik/traefik.yaml", "/etc/traefik/traefik.toml"}

// StaticConfigCandidates lists host paths that may hold the container's
// static configuration file, in Traefik's own search order. Only mounted
// paths are returned, since an unmounted file cannot be read from the host.
func StaticConfigCandidates(c Container) []string {
	flags := parseArgs(c.Args)
	inside := traefikStaticDefaults
	if f := flags["configfile"]; f != "" {
		inside = []string{f}
	}
	var out []string
	for _, p := range inside {
		if host, ok := hostPath(c.Mounts, p); ok {
			out = append(out, host)
		}
	}
	return out
}

// staticFlags flattens a YAML static configuration into the same lowercased
// dotted keys parseArgs produces, so both sources share one detector.
func staticFlags(name string, body []byte) (map[string]string, error) {
	switch strings.ToLower(path.Ext(name)) {
	case ".yml", ".yaml":
	default:
		return nil, fmt.Errorf("the static configuration %s is not YAML; set the values in settings", name)
	}
	var root map[string]any
	if err := yaml.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	out := map[string]string{}
	flatten("", root, out)
	return out, nil
}

func flatten(prefix string, v any, out map[string]string) {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			key := strings.ToLower(k)
			if prefix != "" {
				key = prefix + "." + key
			}
			flatten(key, child, out)
		}
	case nil:
		// A bare section such as "insecure:" enables it, like a bare flag.
		if prefix != "" {
			out[prefix] = "true"
		}
	case []any:
	default:
		out[prefix] = fmt.Sprint(t)
	}
}
