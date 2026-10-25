// Package deploylink validates and builds "Deploy to Levelrail" links for git
// repositories and templates. The dashboard mirrors these rules in
// web/src/lib/deployLink.ts, and both run the cases in testdata/cases.json.
package deploylink

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// Path is the dashboard route a link opens.
const Path = "/deploy"

const (
	maxRepoLen   = 2048
	maxBranchLen = 200
)

var (
	nameRE     = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	templateRE = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,62})$`)
	branchRE   = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
	hostRE     = regexp.MustCompile(`^[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?(?:\.[A-Za-z0-9](?:[A-Za-z0-9-]*[A-Za-z0-9])?)*$`)
	sshUserRE  = regexp.MustCompile(`^[a-z_][a-z0-9_-]{0,31}$`)
	repoPathRE = regexp.MustCompile(`^[A-Za-z0-9._~/-]+$`)
	scpRE      = regexp.MustCompile(`^([a-z_][a-z0-9_-]{0,31})@([A-Za-z0-9.-]+):([A-Za-z0-9._~/-]+)$`)
)

// Params are the values a link may carry. Env values and secrets are never
// part of a link, so there is deliberately no field for them.
type Params struct {
	Repo     string
	Branch   string
	Name     string
	Template string
	Port     int
}

// ValidateRepo accepts https and ssh git URLs without credentials.
func ValidateRepo(s string) error {
	if s == "" || len(s) > maxRepoLen {
		return errors.New("repo must be 1 to 2048 characters")
	}
	for _, r := range s {
		if r <= 0x20 || r == 0x7f || r == '\\' {
			return errors.New("repo must not contain spaces, control characters or backslashes")
		}
	}
	if strings.Contains(s, "://") {
		return validateRepoURL(s)
	}
	m := scpRE.FindStringSubmatch(s)
	if m == nil || !hostRE.MatchString(m[2]) {
		return errors.New("repo must be an https URL, an ssh:// URL or user@host:path")
	}
	return validateRepoPath(m[3])
}

func validateRepoURL(s string) error {
	u, err := url.Parse(s)
	if err != nil {
		return errors.New("repo is not a valid URL")
	}
	if u.RawQuery != "" || u.Fragment != "" || strings.ContainsAny(s, "?#") {
		return errors.New("repo must not have a query or fragment")
	}
	if !hostRE.MatchString(u.Hostname()) {
		return errors.New("repo host is not valid")
	}
	if p := u.Port(); p != "" {
		if _, err := ParsePort(p); err != nil {
			return errors.New("repo port is not valid")
		}
	}
	switch u.Scheme {
	case "https":
		if u.User != nil {
			return errors.New("repo must not contain credentials")
		}
	case "ssh":
		if u.User != nil {
			if _, hasPass := u.User.Password(); hasPass || !sshUserRE.MatchString(u.User.Username()) {
				return errors.New("repo must not contain credentials")
			}
		}
	default:
		return errors.New("repo scheme must be https or ssh")
	}
	return validateRepoPath(strings.TrimPrefix(u.EscapedPath(), "/"))
}

func validateRepoPath(p string) error {
	if p == "" || !repoPathRE.MatchString(p) {
		return errors.New("repo path is not valid")
	}
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return errors.New("repo path must not contain dot-dot segments")
		}
	}
	return nil
}

// ValidateBranch accepts a plain branch or tag name.
func ValidateBranch(s string) error {
	if s == "" || len(s) > maxBranchLen || !branchRE.MatchString(s) ||
		strings.Contains(s, "..") || strings.Contains(s, "//") ||
		strings.HasSuffix(s, "/") || strings.HasSuffix(s, ".lock") || strings.HasSuffix(s, ".") {
		return errors.New("branch is not a safe git ref name")
	}
	return nil
}

// ValidateName accepts a DNS label.
func ValidateName(s string) error {
	if !nameRE.MatchString(s) {
		return errors.New("name must be lowercase letters, digits and hyphens, at most 63 characters")
	}
	return nil
}

// ValidateTemplate checks the shape of a catalog template id. Whether the id
// exists is checked by whoever holds the catalog.
func ValidateTemplate(s string) error {
	if !templateRE.MatchString(s) {
		return errors.New("template must be a catalog id: lowercase letters, digits and hyphens")
	}
	return nil
}

// ParsePort parses a decimal port in 1 to 65535.
func ParsePort(s string) (int, error) {
	if len(s) == 0 || len(s) > 5 {
		return 0, errors.New("port must be between 1 and 65535")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, errors.New("port must be between 1 and 65535")
		}
	}
	n, _ := strconv.Atoi(s)
	if n < 1 || n > 65535 {
		return 0, errors.New("port must be between 1 and 65535")
	}
	return n, nil
}

// Validate checks every set field. A link needs a repo or a template, not both.
func (p Params) Validate() error {
	if (p.Repo == "") == (p.Template == "") {
		return errors.New("set exactly one of repo and template")
	}
	check := func(label string, err error) error {
		if err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		return nil
	}
	if p.Repo != "" {
		if err := check("repo", ValidateRepo(p.Repo)); err != nil {
			return err
		}
	}
	if p.Template != "" {
		if err := check("template", ValidateTemplate(p.Template)); err != nil {
			return err
		}
	}
	if p.Branch != "" {
		if p.Repo == "" {
			return errors.New("branch needs a repo")
		}
		if err := check("branch", ValidateBranch(p.Branch)); err != nil {
			return err
		}
	}
	if p.Name != "" {
		if err := check("name", ValidateName(p.Name)); err != nil {
			return err
		}
	}
	if p.Port != 0 {
		if p.Port < 1 || p.Port > 65535 {
			return errors.New("port: must be between 1 and 65535")
		}
	}
	return nil
}

// Query returns the encoded query in a fixed parameter order.
func (p Params) Query() string {
	var parts []string
	add := func(k, v string) { parts = append(parts, k+"="+url.QueryEscape(v)) }
	if p.Repo != "" {
		add("repo", p.Repo)
	}
	if p.Branch != "" {
		add("branch", p.Branch)
	}
	if p.Template != "" {
		add("template", p.Template)
	}
	if p.Name != "" {
		add("name", p.Name)
	}
	if p.Port != 0 {
		add("port", strconv.Itoa(p.Port))
	}
	return strings.Join(parts, "&")
}

// URL builds the link for an instance base URL such as https://lr.example.com.
func (p Params) URL(instance string) (string, error) {
	if err := p.Validate(); err != nil {
		return "", err
	}
	u, err := url.Parse(strings.TrimSpace(instance))
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Hostname() == "" || u.User != nil {
		return "", fmt.Errorf("instance %q must be an http or https URL without credentials", instance)
	}
	return u.Scheme + "://" + u.Host + strings.TrimRight(u.Path, "/") + Path + "?" + p.Query(), nil
}

// Markdown is the README snippet for a link and a badge image URL.
func Markdown(alt, link, badge string) string {
	return fmt.Sprintf("[![%s](%s)](%s)", alt, badge, link)
}

// HTML is the README snippet for a link and a badge image URL.
func HTML(alt, link, badge string) string {
	esc := func(s string) string {
		return strings.NewReplacer("&", "&amp;", `"`, "&quot;", "<", "&lt;", ">", "&gt;").Replace(s)
	}
	return fmt.Sprintf(`<a href="%s"><img src="%s" alt="%s" height="40"></a>`, esc(link), esc(badge), esc(alt))
}
