// Package pkgregistry renders private package registry credentials into
// the tool config files a Dockerfile build reads through BuildKit secret
// mounts, so a token is never baked into an image layer or a build log.
package pkgregistry

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// Ecosystems a package registry credential can target.
const (
	EcosystemNPM      = "npm"
	EcosystemPyPI     = "pypi"
	EcosystemMaven    = "maven"
	EcosystemNuGet    = "nuget"
	EcosystemCargo    = "cargo"
	EcosystemGo       = "go"
	EcosystemRubyGems = "rubygems"
)

// Scope kinds: how widely a package registry applies.
const (
	ScopeInstance = "instance"
	ScopeProject  = "project"
	ScopeApp      = "app"
)

// Ecosystems lists every supported ecosystem in display order.
var Ecosystems = []string{
	EcosystemNPM, EcosystemPyPI, EcosystemMaven, EcosystemNuGet,
	EcosystemCargo, EcosystemGo, EcosystemRubyGems,
}

// SecretIDs maps an ecosystem to the BuildKit secret id its generated
// config is mounted under, the id a Dockerfile references.
var SecretIDs = map[string]string{
	EcosystemNPM:      "npmrc",
	EcosystemPyPI:     "pip_conf",
	EcosystemMaven:    "maven_settings",
	EcosystemNuGet:    "nuget_config",
	EcosystemCargo:    "cargo_config",
	EcosystemGo:       "netrc",
	EcosystemRubyGems: "bundle_config",
}

// IsEcosystem reports whether e is a supported ecosystem.
func IsEcosystem(e string) bool {
	_, ok := SecretIDs[e]
	return ok
}

// Entry is one resolved package registry credential, secret included.
type Entry struct {
	Ecosystem string
	URL       string
	Username  string
	Secret    string
	// ModulePatterns is a comma separated GOPRIVATE list (go only).
	ModulePatterns string
}

// Output is what a build receives: secret file contents by mount id,
// non-secret build args, and every string that must be masked in logs.
type Output struct {
	Files  map[string][]byte
	Args   map[string]string
	Redact []string
}

// ValidateURL requires an http or https URL with no embedded credentials,
// which would otherwise leak through build args and logs.
func ValidateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return fmt.Errorf("url must be an absolute http or https URL")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return fmt.Errorf("url must use http or https")
	}
	if u.User != nil {
		return fmt.Errorf("url must not embed credentials, use the username and secret fields")
	}
	return nil
}

// Render produces the build inputs for entries. When several entries
// share an ecosystem the first wins, so callers order narrowest scope first.
func Render(entries []Entry) (Output, error) {
	out := Output{Files: map[string][]byte{}, Args: map[string]string{}}
	seen := map[string]bool{}
	for _, e := range entries {
		if seen[e.Ecosystem] {
			continue
		}
		seen[e.Ecosystem] = true
		if err := ValidateURL(e.URL); err != nil {
			return Output{}, fmt.Errorf("pkgregistry: %s registry: %w", e.Ecosystem, err)
		}
		u, _ := url.Parse(e.URL)
		body, args := renderOne(e, u)
		out.Files[SecretIDs[e.Ecosystem]] = []byte(body)
		for k, v := range args {
			out.Args[k] = v
		}
		out.Redact = append(out.Redact, redactions(e)...)
	}
	return out, nil
}

func redactions(e Entry) []string {
	if e.Secret == "" {
		return nil
	}
	return []string{
		e.Secret,
		url.QueryEscape(e.Secret),
		url.PathEscape(e.Secret),
		base64.StdEncoding.EncodeToString([]byte(e.Secret)),
		base64.StdEncoding.EncodeToString([]byte(e.Username + ":" + e.Secret)),
	}
}

func renderOne(e Entry, u *url.URL) (string, map[string]string) {
	switch e.Ecosystem {
	case EcosystemNPM:
		return renderNPM(e, u), nil
	case EcosystemPyPI:
		return renderPyPI(e, u), nil
	case EcosystemMaven:
		return renderMaven(e), nil
	case EcosystemNuGet:
		return renderNuGet(e), nil
	case EcosystemCargo:
		return renderCargo(e), nil
	case EcosystemGo:
		return renderGo(e, u)
	default:
		return renderRubyGems(e, u), nil
	}
}

func renderNPM(e Entry, u *url.URL) string {
	prefix := "//" + u.Host + strings.TrimRight(u.Path, "/") + "/"
	var b strings.Builder
	fmt.Fprintf(&b, "registry=%s/\n", strings.TrimRight(e.URL, "/"))
	if e.Username == "" {
		fmt.Fprintf(&b, "%s:_authToken=%s\n", prefix, e.Secret)
		return b.String()
	}
	fmt.Fprintf(&b, "%s:username=%s\n", prefix, e.Username)
	fmt.Fprintf(&b, "%s:_password=%s\n", prefix, base64.StdEncoding.EncodeToString([]byte(e.Secret)))
	return b.String()
}

func renderPyPI(e Entry, u *url.URL) string {
	user := e.Username
	if user == "" {
		user = "__token__"
	}
	withAuth := *u
	withAuth.User = url.UserPassword(user, e.Secret)
	return "[global]\nindex-url = " + withAuth.String() + "\n"
}

func xmlEscape(s string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "&apos;").Replace(s)
}

func renderMaven(e Entry) string {
	return fmt.Sprintf(`<settings>
  <servers>
    <server><id>private</id><username>%s</username><password>%s</password></server>
  </servers>
  <mirrors>
    <mirror><id>private</id><mirrorOf>*</mirrorOf><url>%s</url></mirror>
  </mirrors>
</settings>
`, xmlEscape(e.Username), xmlEscape(e.Secret), xmlEscape(e.URL))
}

func renderNuGet(e Entry) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="utf-8"?>
<configuration>
  <packageSources><add key="private" value="%s" /></packageSources>
  <packageSourceCredentials>
    <private>
      <add key="Username" value="%s" />
      <add key="ClearTextPassword" value="%s" />
    </private>
  </packageSourceCredentials>
</configuration>
`, xmlEscape(e.URL), xmlEscape(e.Username), xmlEscape(e.Secret))
}

func tomlString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

func renderCargo(e Entry) string {
	index := e.URL
	if !strings.HasPrefix(index, "sparse+") {
		index = "sparse+" + strings.TrimRight(index, "/") + "/"
	}
	return "[registries.private]\nindex = " + tomlString(index) + "\ntoken = " + tomlString(e.Secret) + "\n"
}

func renderGo(e Entry, u *url.URL) (string, map[string]string) {
	user := e.Username
	if user == "" {
		user = "token"
	}
	netrc := fmt.Sprintf("machine %s login %s password %s\n", u.Hostname(), user, e.Secret)
	args := map[string]string{"GOPROXY": strings.TrimRight(e.URL, "/") + ",direct"}
	if p := strings.TrimSpace(e.ModulePatterns); p != "" {
		args["GOPRIVATE"] = p
	}
	return netrc, args
}

func renderRubyGems(e Entry, u *url.URL) string {
	key := strings.ToUpper(u.Hostname())
	key = strings.ReplaceAll(key, "-", "___")
	key = strings.ReplaceAll(key, ".", "__")
	cred := e.Secret
	if e.Username != "" {
		cred = e.Username + ":" + e.Secret
	}
	return fmt.Sprintf("---\nBUNDLE_%s: %s\n", key, tomlString(cred))
}

// Redactor masks every value in redact (longest first) with ***.
func Redactor(redact []string) func(string) string {
	vals := make([]string, 0, len(redact))
	for _, r := range redact {
		if len(r) >= 4 {
			vals = append(vals, r)
		}
	}
	sort.Slice(vals, func(i, j int) bool { return len(vals[i]) > len(vals[j]) })
	return func(s string) string {
		for _, v := range vals {
			s = strings.ReplaceAll(s, v, "***")
		}
		return s
	}
}
