package plugin

import (
	"encoding/json"
	"net"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

const maxConfigHostFields = 8

// ConfigHosts parses the operator-configured values named by the manifest's
// outbound_hosts_from_config into host or host:port entries. A missing or empty
// value is skipped; a malformed one is an error so a bad host is never allowed.
func ConfigHosts(man Manifest, configJSON string) ([]string, error) {
	if len(man.OutboundHostsFromConfig) == 0 || strings.TrimSpace(configJSON) == "" {
		return []string{}, nil
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(configJSON), &obj); err != nil {
		return nil, invalid("config must be a JSON object")
	}
	out := []string{}
	for _, field := range man.OutboundHostsFromConfig {
		raw, ok := obj[field]
		if !ok || string(raw) == "null" {
			continue
		}
		var v string
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, invalid("config field %q must be a string host or URL", field)
		}
		if strings.TrimSpace(v) == "" {
			continue
		}
		h, err := parseConfigHost(v)
		if err != nil {
			return nil, invalid("config field %q: %v", field, err)
		}
		out = append(out, h)
	}
	sort.Strings(out)
	return dedupe(out), nil
}

// EffectiveOutboundHosts is the full allowed host set: declared hosts plus the
// hosts parsed from operator config. The egress layer should call only this.
func EffectiveOutboundHosts(man Manifest, configJSON string) ([]string, error) {
	extra, err := ConfigHosts(man, configJSON)
	if err != nil {
		return nil, err
	}
	all := append(append([]string{}, man.OutboundHosts...), extra...)
	sort.Strings(all)
	return dedupe(all), nil
}

func parseConfigHost(v string) (string, error) {
	v = strings.TrimSpace(v)
	if !strings.Contains(v, "://") {
		v = "//" + v
	}
	u, err := url.Parse(v)
	if err != nil {
		return "", invalid("not a valid host or URL")
	}
	if u.Scheme != "" && u.Scheme != "http" && u.Scheme != "https" {
		return "", invalid("only http and https URLs are accepted")
	}
	host := strings.ToLower(u.Hostname())
	if net.ParseIP(host) == nil && !hostPattern.MatchString(host) {
		return "", invalid("not a valid hostname")
	}
	port := u.Port()
	if port == "" {
		return host, nil
	}
	if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
		return "", invalid("port is out of range")
	}
	return net.JoinHostPort(host, port), nil
}
