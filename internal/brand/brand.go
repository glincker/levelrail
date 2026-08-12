// Package brand provides the branding indirection layer described in
// CLAUDE.md section 3: no product name is ever hardcoded in source.
package brand

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Brand holds every user-visible identity string for the running binary.
// Nothing outside this package should reference a product name directly.
type Brand struct {
	Name         string `yaml:"name"`
	ShortName    string `yaml:"short_name"`
	BinaryName   string `yaml:"binary_name"`
	Domain       string `yaml:"domain"`
	SupportURL   string `yaml:"support_url"`
	PrimaryColor string `yaml:"primary_color"`
	LogoSVG      string `yaml:"logo_svg"`
	DocsURL      string `yaml:"docs_url"`
}

const envPrefix = "APP_BRAND_"

// Load reads brand.yaml from path, then applies APP_BRAND_* environment
// overrides on top. Env vars win over the file so a deployment can be
// rebranded without a rebuild. The prefix is deliberately generic rather
// than tied to the current name, see CLAUDE.md section 3.
func Load(path string) (*Brand, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("brand: read %s: %w", path, err)
	}

	var b Brand
	if err := yaml.Unmarshal(data, &b); err != nil {
		return nil, fmt.Errorf("brand: parse %s: %w", path, err)
	}

	b.applyEnvOverrides()

	if err := b.validate(); err != nil {
		return nil, fmt.Errorf("brand: %w", err)
	}

	return &b, nil
}

func (b *Brand) applyEnvOverrides() {
	override(&b.Name, envPrefix+"NAME")
	override(&b.ShortName, envPrefix+"SHORT_NAME")
	override(&b.BinaryName, envPrefix+"BINARY_NAME")
	override(&b.Domain, envPrefix+"DOMAIN")
	override(&b.SupportURL, envPrefix+"SUPPORT_URL")
	override(&b.PrimaryColor, envPrefix+"PRIMARY_COLOR")
	override(&b.LogoSVG, envPrefix+"LOGO_SVG")
	override(&b.DocsURL, envPrefix+"DOCS_URL")
}

func override(field *string, envVar string) {
	if v, ok := os.LookupEnv(envVar); ok && v != "" {
		*field = v
	}
}

func (b *Brand) validate() error {
	if b.Name == "" {
		return fmt.Errorf("name is required")
	}
	if b.BinaryName == "" {
		return fmt.Errorf("binary_name is required")
	}
	return nil
}
