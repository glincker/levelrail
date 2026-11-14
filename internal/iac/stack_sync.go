package iac

import (
	"errors"
	"net/url"
	"path"
	"regexp"
	"strings"
)

// StackSync is the opt-in git source of a stack. The sync loop only plans from
// it; apply, prune and deploy on sync stay separate, explicit flags.
type StackSync struct {
	RepoURL string `json:"repo_url"`
	Branch  string `json:"branch"`
	Path    string `json:"path,omitempty"`
	// CredentialRef names a stored credential ("app:<name>"), never a secret value.
	CredentialRef   string `json:"credential_ref,omitempty"`
	IntervalSeconds int    `json:"interval_seconds,omitempty"`
	WebhookOnly     bool   `json:"webhook_only,omitempty"`
	Enabled         bool   `json:"enabled"`
}

// MaxSyncIntervalSeconds bounds a per stack interval to one day.
const MaxSyncIntervalSeconds = 86400

var (
	syncBranchRe = regexp.MustCompile(`^[A-Za-z0-9._/-]{1,200}$`)
	syncCredRe   = regexp.MustCompile(`^app:[a-z0-9][a-z0-9-]{0,62}$`)
	syncSCPRe    = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}@[A-Za-z0-9.-]{1,253}:[A-Za-z0-9._/~-]{1,512}$`)
)

// Validate returns the stable code and reason for the first invalid field.
// Paths that leave the checkout carry CodeSyncPathInvalid.
func (s StackSync) Validate() (string, error) {
	if err := validSyncURL(s.RepoURL); err != nil {
		return CodeInvalidRequest, err
	}
	b := s.Branch
	if !syncBranchRe.MatchString(b) || strings.HasPrefix(b, "-") || strings.HasPrefix(b, "/") || strings.HasSuffix(b, "/") ||
		strings.Contains(b, "..") || strings.Contains(b, "//") || strings.HasSuffix(b, ".lock") {
		return CodeInvalidRequest, errors.New("branch must be a plain branch name")
	}
	if _, err := CleanSyncPath(s.Path); err != nil {
		return CodeSyncPathInvalid, err
	}
	if s.CredentialRef != "" && !syncCredRe.MatchString(s.CredentialRef) {
		return CodeInvalidRequest, errors.New(`credential_ref must name a stored credential as "app:<name>"`)
	}
	if s.IntervalSeconds < 0 || s.IntervalSeconds > MaxSyncIntervalSeconds {
		return CodeInvalidRequest, errors.New("interval_seconds must be from 0 to 86400")
	}
	return "", nil
}

// CleanSyncPath returns the slash path under the repository root; "" is the root.
func CleanSyncPath(p string) (string, error) {
	if strings.ContainsAny(p, "\\\x00") || strings.HasPrefix(p, "/") {
		return "", errors.New("path must be relative to the repository root")
	}
	c := path.Clean("/" + p)[1:]
	for _, seg := range strings.Split(p, "/") {
		if seg == ".." {
			return "", errors.New("path must not hold a parent segment")
		}
	}
	if c == "." {
		c = ""
	}
	if strings.HasPrefix(c, ".git/") || c == ".git" {
		return "", errors.New("path must not point into .git")
	}
	return c, nil
}

func validSyncURL(raw string) error {
	if raw == "" || len(raw) > 2048 || strings.HasPrefix(raw, "-") || strings.ContainsAny(raw, " \t\r\n\x00") {
		return errors.New("repo_url must be an https or ssh git URL")
	}
	if !strings.Contains(raw, "://") {
		if syncSCPRe.MatchString(raw) && !strings.Contains(raw, "..") {
			return nil
		}
		return errors.New("repo_url must be an https or ssh git URL, not a local path")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || strings.HasPrefix(u.Host, "-") {
		return errors.New("repo_url must be an https or ssh git URL")
	}
	switch u.Scheme {
	case "https":
		if u.User != nil {
			return errors.New("repo_url must not carry credentials; use credential_ref")
		}
	case "ssh":
		if _, has := u.User.Password(); has {
			return errors.New("repo_url must not carry credentials; use credential_ref")
		}
	default:
		return errors.New("repo_url must use https or ssh")
	}
	return nil
}
