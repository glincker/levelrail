package catalog

import (
	"regexp"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/compose"
)

// Generatable SERVICE_ tokens are rejected inside command entries, so a
// template must route generated secrets through environment values.
func TestTemplates_MagicVarsAllResolve(t *testing.T) {
	for _, tpl := range Templates {
		f, err := compose.Parse([]byte(tpl.Compose))
		if err != nil {
			continue
		}
		gen := func(kind, _ string, length int) (string, error) { return compose.GenerateValue(kind, length) }
		persist := func(_, _, _ string) error { return nil }
		_, unresolved, err := compose.ResolveMagicVars(f, gen, persist)
		if err != nil {
			t.Errorf("template %q: %v", tpl.ID, err)
		}
		for _, u := range unresolved {
			if strings.Contains(u.Token, "SERVICE_PROMPT_") {
				continue // asked of the installer by design
			}
			t.Errorf("template %q: unresolved %s in %s of service %s", tpl.ID, u.Token, u.EnvKey, u.Service)
		}
	}
}

var (
	// $$ is a compose escape for a container-shell variable, so it is skipped.
	shellVarRef    = regexp.MustCompile(`(^|[^$])\$(?:\{([A-Za-z_][A-Za-z0-9_]*)(:?-[^}]*)?\}|([A-Za-z_][A-Za-z0-9_]*))`)
	secretLikeName = regexp.MustCompile(`(?i)(PASSWORD|PASSWD|SECRET|TOKEN|APIKEY|API_KEY|_KEY$|^KEY$|PASS$)`)
)

// A secret-like ${VAR} that nothing generates would resolve from the host
// shell to an empty string, deploying the service with a blank credential.
func TestTemplates_SecretRefsAreGeneratedOrDeclared(t *testing.T) {
	for _, tpl := range Templates {
		declared := map[string]bool{}
		// A service's own environment key is expanded by the container
		// shell, not the host, so $KEY in its command is safe.
		if f, err := compose.Parse([]byte(tpl.Compose)); err == nil {
			for _, svc := range f.Services {
				for k := range svc.Environment {
					declared[k] = true
				}
			}
		}
		if tpl.Manifest != nil {
			for _, e := range tpl.Manifest.Env {
				declared[e.Key] = true
			}
		}
		for _, m := range shellVarRef.FindAllStringSubmatch(tpl.Compose, -1) {
			name, suffix := m[2], m[3]
			if name == "" {
				name = m[4]
			}
			if strings.HasPrefix(name, "SERVICE_") || !secretLikeName.MatchString(name) || declared[name] {
				continue
			}
			if (strings.HasPrefix(suffix, ":-") && len(suffix) > 2) || (strings.HasPrefix(suffix, "-") && len(suffix) > 1) {
				continue
			}
			t.Errorf("template %q references secret-like %q that no $SERVICE_ generator or env input provides", tpl.ID, name)
		}
	}
}

// Only environment values and command entries are resolved at deploy
// time; a SERVICE_ token in an entrypoint or healthcheck stays literal.
func TestTemplates_NoMagicVarsInEntrypointOrHealthcheck(t *testing.T) {
	for _, tpl := range Templates {
		f, err := compose.Parse([]byte(tpl.Compose))
		if err != nil {
			continue
		}
		for name, svc := range f.Services {
			parts := append([]string{}, svc.Entrypoint...)
			if svc.Healthcheck != nil {
				parts = append(parts, svc.Healthcheck.Test...)
			}
			for _, p := range parts {
				if strings.Contains(p, "SERVICE_") {
					t.Errorf("template %q service %q: %q uses a SERVICE_ token that is never resolved here", tpl.ID, name, p)
				}
			}
		}
	}
}

// Laravel's AES-256 cipher needs a 32 byte key; "base64:" plus a 32 char
// generated value decodes to 24 bytes and the app fails to boot.
func TestTemplates_NoShortBase64AppKey(t *testing.T) {
	for _, tpl := range Templates {
		if strings.Contains(tpl.Compose, "base64:$SERVICE_BASE64_32") || strings.Contains(tpl.Compose, "base64:${SERVICE_BASE64_32") {
			t.Errorf("template %q prefixes base64: to a 32 char generated value (24 decoded bytes); use $SERVICE_PASSWORD_32_APPKEY", tpl.ID)
		}
	}
}
