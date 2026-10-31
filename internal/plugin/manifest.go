// Package plugin runs out-of-process extensions behind a signed, digest-pinned
// package and an operator-approved ability list (ADR 034).
package plugin

import (
	"bytes"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Extension kinds. KindPanelProvider is declared in panel.go.
const (
	KindNotificationChannel = "notification_channel"
	KindDNSProvider         = "dns_provider"
)

// Abilities a plugin may be granted. Both come from the scoped-token
// vocabulary (ADR 012); root and every write ability are refused.
const (
	AbilityRead          = "read"
	AbilityReadSensitive = "read:sensitive"
)

// ManifestAPIVersion is the manifest schema version this build accepts.
const ManifestAPIVersion = 1

const (
	maxManifestBytes = 32 << 10
	maxOutboundHosts = 16
)

var (
	namePattern  = regexp.MustCompile(`^[a-z][a-z0-9-]{1,39}$`)
	imagePattern = regexp.MustCompile(`^[a-z0-9][A-Za-z0-9._/:-]{0,200}@(sha256:[0-9a-f]{64})$`)
	// Manifests declare no config schema, so names are checked by shape here and by value at install.
	configFieldPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,39}$`)
	hostPattern        = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)
	versionRegexp      = regexp.MustCompile(`^[0-9A-Za-z][0-9A-Za-z._+-]{0,39}$`)
)

// Manifest is the plugin.yaml a publisher signs together with the image digest.
type Manifest struct {
	APIVersion    int      `yaml:"api_version" json:"api_version"`
	Name          string   `yaml:"name" json:"name"`
	Version       string   `yaml:"version" json:"version"`
	Kind          string   `yaml:"kind" json:"kind"`
	Publisher     string   `yaml:"publisher" json:"publisher"`
	Description   string   `yaml:"description" json:"description"`
	Image         string   `yaml:"image" json:"image"`
	Abilities     []string `yaml:"abilities" json:"abilities"`
	OutboundHosts []string `yaml:"outbound_hosts" json:"outbound_hosts"`
	// OutboundHostsFromConfig names config fields whose values are hosts or URLs the plugin may reach.
	OutboundHostsFromConfig []string `yaml:"outbound_hosts_from_config" json:"outbound_hosts_from_config"`
	Contributes             Contrib  `yaml:"contributes" json:"contributes"`
}

// Contrib lists the UI a plugin adds. It is data the core renders with its own
// components, never code.
type Contrib struct {
	Panels   []Panel   `yaml:"panels" json:"panels"`
	Commands []Command `yaml:"commands" json:"commands"`
}

// Panel is a card the dashboard renders. Without a Source it is static rows on
// the Plugins page; with one it is widgets fed by a plugin call, in Slot.
type Panel struct {
	ID          string       `yaml:"id" json:"id"`
	Title       string       `yaml:"title" json:"title"`
	Description string       `yaml:"description" json:"description"`
	Rows        []PanelRow   `yaml:"rows" json:"rows"`
	Slot        string       `yaml:"slot" json:"slot"`
	Source      *PanelSource `yaml:"source" json:"source,omitempty"`
	Widgets     []Widget     `yaml:"widgets" json:"widgets"`
}

// PanelRow is one label and plain-text value in a Panel.
type PanelRow struct {
	Label string `yaml:"label" json:"label"`
	Value string `yaml:"value" json:"value"`
}

// Digest returns the sha256 digest the image reference is pinned to.
func (m Manifest) Digest() string {
	if i := strings.LastIndex(m.Image, "@"); i >= 0 {
		return m.Image[i+1:]
	}
	return ""
}

// ParseManifest decodes and validates raw manifest bytes. Unknown fields are
// an error so a typo in an ability list can never silently widen or narrow it.
func ParseManifest(raw []byte) (Manifest, error) {
	if len(raw) == 0 || len(raw) > maxManifestBytes {
		return Manifest{}, invalid("manifest must be 1 to %d bytes", maxManifestBytes)
	}
	var m Manifest
	dec := yaml.NewDecoder(bytes.NewReader(raw))
	dec.KnownFields(true)
	if err := dec.Decode(&m); err != nil {
		return Manifest{}, invalid("parse manifest: %v", err)
	}
	if err := m.validate(); err != nil {
		return Manifest{}, err
	}
	if err := validateSchema(raw); err != nil {
		return Manifest{}, err
	}
	if err := m.validatePanels(); err != nil {
		return Manifest{}, err
	}
	return m, nil
}

func (m *Manifest) validate() error {
	if m.APIVersion != ManifestAPIVersion {
		return invalid("manifest api_version must be %d", ManifestAPIVersion)
	}
	if !namePattern.MatchString(m.Name) {
		return invalid("name must be 2 to 40 lowercase letters, digits or dashes, starting with a letter")
	}
	if !versionRegexp.MatchString(m.Version) {
		return invalid("version is missing or malformed")
	}
	if m.Kind != KindNotificationChannel && m.Kind != KindPanelProvider && m.Kind != KindDNSProvider {
		return invalid("kind %q is not supported, want %s, %s or %s", m.Kind, KindNotificationChannel, KindPanelProvider, KindDNSProvider)
	}
	if strings.TrimSpace(m.Publisher) == "" || len(m.Publisher) > 80 {
		return invalid("publisher is required (80 characters at most)")
	}
	if len(m.Description) > 400 {
		return invalid("description is 400 characters at most")
	}
	if !imagePattern.MatchString(m.Image) {
		return invalid("image must be a registry reference pinned by digest (name@sha256:<64 hex>), tags are refused")
	}
	abilities, err := normalizeAbilities(m.Abilities)
	if err != nil {
		return err
	}
	if (m.Kind == KindNotificationChannel || m.Kind == KindDNSProvider) && !contains(abilities, AbilityRead) {
		return invalid("a %s plugin must request the read ability to be called", m.Kind)
	}
	m.Abilities = abilities
	if len(m.OutboundHosts) > maxOutboundHosts {
		return invalid("at most %d outbound_hosts", maxOutboundHosts)
	}
	hosts := make([]string, 0, len(m.OutboundHosts))
	for _, h := range m.OutboundHosts {
		h = strings.ToLower(strings.TrimSpace(h))
		if !hostPattern.MatchString(h) {
			return invalid("outbound host %q is not a bare hostname", h)
		}
		hosts = append(hosts, h)
	}
	sort.Strings(hosts)
	m.OutboundHosts = dedupe(hosts)
	if len(m.OutboundHostsFromConfig) > maxConfigHostFields {
		return invalid("at most %d outbound_hosts_from_config names", maxConfigHostFields)
	}
	fields := make([]string, 0, len(m.OutboundHostsFromConfig))
	for _, f := range m.OutboundHostsFromConfig {
		f = strings.TrimSpace(f)
		if !configFieldPattern.MatchString(f) {
			return invalid("outbound_hosts_from_config name %q is not a valid config field name", f)
		}
		fields = append(fields, f)
	}
	sort.Strings(fields)
	m.OutboundHostsFromConfig = dedupe(fields)
	if m.Contributes.Panels == nil {
		m.Contributes.Panels = []Panel{}
	}
	if m.Contributes.Commands == nil {
		m.Contributes.Commands = []Command{}
	}
	return nil
}

func normalizeAbilities(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	for _, a := range in {
		a = strings.TrimSpace(a)
		switch a {
		case AbilityRead, AbilityReadSensitive:
			out = append(out, a)
		case "root":
			return nil, invalid("the root ability can never be granted to a plugin")
		default:
			return nil, invalid("ability %q is not available to plugins in this release (allowed: %s, %s)", a, AbilityRead, AbilityReadSensitive)
		}
	}
	sort.Strings(out)
	return dedupe(out), nil
}

func dedupe(sorted []string) []string {
	out := sorted[:0:0]
	for i, s := range sorted {
		if i == 0 || s != sorted[i-1] {
			out = append(out, s)
		}
	}
	return out
}

func contains(list []string, v string) bool {
	for _, s := range list {
		if s == v {
			return true
		}
	}
	return false
}
