// Package reporef parses a git repository reference in any common form
// (https, http, ssh, git, scp style) into host, owner, repo, provider and a
// canonical clone URL. It is the one parser every repo URL input goes through.
package reporef

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
)

// Provider is a best guess at the git host software, from the host name only.
type Provider string

// Provider values. Self-hosted instances on custom domains report ProviderOther.
const (
	ProviderGitHub    Provider = "github"
	ProviderGitLab    Provider = "gitlab"
	ProviderBitbucket Provider = "bitbucket"
	ProviderOther     Provider = "other"
)

// Scheme values for Ref.Scheme. SchemeSCP is the git@host:owner/repo form.
const (
	SchemeHTTPS = "https"
	SchemeHTTP  = "http"
	SchemeSSH   = "ssh"
	SchemeGit   = "git"
	SchemeSCP   = "scp"
)

// ErrUnsupported is returned for input that is not a remote repository URL.
var ErrUnsupported = errors.New("reporef: not a remote git repository URL")

// Ref is a parsed repository reference. It never holds credentials.
type Ref struct {
	// Scheme is the transport the input used, one of the Scheme constants.
	Scheme string
	// Host is the lowercase host name without a port.
	Host string
	// Port is kept only for http and https; an ssh port means nothing over https.
	Port string
	// Owner is every path segment before the repo, so GitLab subgroups stay intact.
	Owner string
	// Repo is the last path segment without a ".git" suffix.
	Repo     string
	Provider Provider
	// HadCredentials is true when a password or token was stripped from the input.
	HadCredentials bool
}

var (
	scpForm      = regexp.MustCompile(`^(?:([A-Za-z0-9._~-]+)@)?([A-Za-z0-9.-]+):(.+)$`)
	segmentChars = regexp.MustCompile(`^[A-Za-z0-9._~+-]+$`)
	hostChars    = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]*[a-z0-9])?$`)
	schemeAlias  = map[string]string{"git+ssh": SchemeSSH, "ssh+git": SchemeSSH}
	allowed      = map[string]bool{SchemeHTTPS: true, SchemeHTTP: true, SchemeSSH: true, SchemeGit: true}
	sshOnlyHosts = map[string]string{"ssh.github.com": "github.com", "altssh.gitlab.com": "gitlab.com", "altssh.bitbucket.org": "bitbucket.org"}
)

// Parse reads a repository URL. Shorthand such as "owner/repo" needs ParseWithHost.
func Parse(raw string) (Ref, error) {
	s := strings.TrimSpace(raw)
	if s == "" || strings.ContainsAny(s, " \t\r\n\x00") {
		return Ref{}, ErrUnsupported
	}
	if strings.Contains(s, "://") {
		return parseURL(s)
	}
	return parseSCP(s)
}

// ParseWithHost is Parse that also accepts "owner/repo" (on defaultHost) and
// "host.tld/owner/repo" shorthand, the forms import sources store.
func ParseWithHost(raw, defaultHost string) (Ref, error) {
	if r, err := Parse(raw); err == nil {
		return r, nil
	}
	s := strings.Trim(strings.TrimSpace(raw), "/")
	if s == "" || strings.Contains(s, ":") || strings.Contains(s, "@") {
		return Ref{}, ErrUnsupported
	}
	if first, _, _ := strings.Cut(s, "/"); !strings.Contains(first, ".") {
		if defaultHost == "" {
			return Ref{}, ErrUnsupported
		}
		s = defaultHost + "/" + s
	}
	return parseURL("https://" + s)
}

func parseURL(s string) (Ref, error) {
	u, err := url.Parse(s)
	if err != nil {
		return Ref{}, fmt.Errorf("%w: %w", ErrUnsupported, err)
	}
	scheme := strings.ToLower(u.Scheme)
	if a, ok := schemeAlias[scheme]; ok {
		scheme = a
	}
	if !allowed[scheme] {
		return Ref{}, ErrUnsupported
	}
	r := Ref{Scheme: scheme, Host: strings.ToLower(u.Hostname())}
	if u.User != nil {
		_, hasPass := u.User.Password()
		r.HadCredentials = hasPass || scheme == SchemeHTTPS || scheme == SchemeHTTP
	}
	if scheme == SchemeHTTPS || scheme == SchemeHTTP {
		r.Port = u.Port()
	}
	return r.finish(u.Path)
}

func parseSCP(s string) (Ref, error) {
	m := scpForm.FindStringSubmatch(s)
	// A dot in the host keeps Windows drive paths and bare ssh aliases out.
	if m == nil || !strings.Contains(m[2], ".") {
		return Ref{}, ErrUnsupported
	}
	p := m[3]
	if i := strings.IndexAny(p, "?#"); i >= 0 {
		p = p[:i]
	}
	r := Ref{Scheme: SchemeSCP, Host: strings.ToLower(m[2])}
	return r.finish(p)
}

func (r Ref) finish(rawPath string) (Ref, error) {
	if !hostChars.MatchString(r.Host) {
		return Ref{}, ErrUnsupported
	}
	p := strings.Trim(rawPath, "/")
	for strings.HasSuffix(p, ".git") {
		p = strings.Trim(strings.TrimSuffix(p, ".git"), "/")
	}
	if p == "" {
		return Ref{}, ErrUnsupported
	}
	segs := strings.Split(p, "/")
	for _, seg := range segs {
		if seg == "." || seg == ".." || !segmentChars.MatchString(seg) {
			return Ref{}, ErrUnsupported
		}
	}
	if !r.IsHTTP() {
		if h, ok := sshOnlyHosts[r.Host]; ok {
			r.Host = h
		}
		// Azure DevOps ssh paths carry a v3 prefix and no _git segment.
		if r.Host == "ssh.dev.azure.com" && len(segs) == 4 && segs[0] == "v3" {
			r.Host, segs = "dev.azure.com", []string{segs[1], segs[2], "_git", segs[3]}
		}
	}
	r.Repo = segs[len(segs)-1]
	r.Owner = strings.Join(segs[:len(segs)-1], "/")
	r.Provider = providerFor(r.Host)
	return r, nil
}

func providerFor(host string) Provider {
	switch {
	case host == "github.com" || strings.HasPrefix(host, "github.") || strings.HasSuffix(host, ".ghe.com"):
		return ProviderGitHub
	case host == "gitlab.com" || strings.HasPrefix(host, "gitlab."):
		return ProviderGitLab
	case host == "bitbucket.org" || strings.HasPrefix(host, "bitbucket."):
		return ProviderBitbucket
	}
	return ProviderOther
}

// IsHTTP reports whether the input already used http or https.
func (r Ref) IsHTTP() bool { return r.Scheme == SchemeHTTPS || r.Scheme == SchemeHTTP }

// Path is "owner/repo", or just "repo" when the host has no owner level.
func (r Ref) Path() string {
	if r.Owner == "" {
		return r.Repo
	}
	return r.Owner + "/" + r.Repo
}

// CloneURL is the canonical credential-free clone URL: https, or http when the input was plain http.
func (r Ref) CloneURL() string {
	scheme := SchemeHTTPS
	if r.Scheme == SchemeHTTP {
		scheme = SchemeHTTP
	}
	host := r.Host
	if r.Port != "" {
		host += ":" + r.Port
	}
	return scheme + "://" + host + "/" + r.Path()
}

// Identity is the lowercase path, so every form of one repository compares equal.
func (r Ref) Identity() string { return strings.ToLower(r.Path()) }

// Identity reduces any repository reference, including "owner/repo"
// shorthand, to Ref.Identity. Unreadable input yields "".
func Identity(raw string) string {
	if r, err := Parse(raw); err == nil {
		return r.Identity()
	}
	s := strings.TrimSpace(raw)
	if strings.Contains(s, "://") || strings.Contains(s, ":") {
		return ""
	}
	s = strings.TrimSuffix(strings.Trim(s, "/"), ".git")
	return strings.ToLower(strings.Trim(s, "/"))
}
