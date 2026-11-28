package announcements

import (
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Content rules, shared by the client and the publishing validator.
//
// Body text is the whole supported markdown subset: plain text paragraphs
// (blank line separated), **bold**, `inline code` and [text](https://...)
// links. No HTML, images, fenced code or headings are interpreted; the
// validator rejects HTML and images outright, and the renderer shows
// anything else as plain text. Every URL, linked or bare, must pass
// ValidateLink.

// Link host allowlist: the site host, and GitHub paths under the org only.
const (
	allowedSiteHost   = "levelrail.com"
	allowedGitHubHost = "github.com"
	allowedGitHubPath = "/glincker/"
	maxLinkTextRunes  = 80
	maxLinksPerItem   = 5
	maxIDLen          = 80
)

var (
	idPattern     = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
	htmlPattern   = regexp.MustCompile(`(?i)<[a-z/!?]|&#|&[a-z]+;`)
	mdLinkPattern = regexp.MustCompile(`\[([^\]]*)\]\(([^)]*)\)`)
	urlPattern    = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s)]+`)
	codePattern   = regexp.MustCompile("`([^`]*)`")
	schemePattern = regexp.MustCompile(`(?i)\b(javascript|data|vbscript|file|blob):`)
	cmdWord       = regexp.MustCompile(`(?i)\b(curl|wget|sh|bash|zsh|iex|sudo|powershell|chmod|eval)\b`)
	urgencyWord   = regexp.MustCompile(`(?i)\b(now|urgent|urgently|immediately|asap)\b`)
	salesWord     = regexp.MustCompile(`(?i)\b(pricing|price|paid|enterprise|subscription|premium|discount)\b`)
	validChannels = map[string]bool{"stable": true, "beta": true, "edge": true}
)

// ErrContent is wrapped by every validation failure.
var ErrContent = errors.New("announcements: invalid content")

func bad(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrContent, fmt.Sprintf(format, a...))
}

// ValidateLink accepts only https links to the allowlisted hosts, without
// credentials, port, query or fragment.
func ValidateLink(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return bad("link does not parse")
	}
	if u.Scheme != "https" || u.User != nil || u.Opaque != "" {
		return bad("link must be a plain https URL")
	}
	if u.RawQuery != "" || u.Fragment != "" || strings.Contains(raw, "?") || strings.Contains(raw, "#") {
		return bad("link must not carry a query or fragment")
	}
	if strings.Contains(raw, "%") || strings.Contains(u.Path, "..") || strings.Contains(u.Path, "//") {
		return bad("link path is not plain")
	}
	switch strings.ToLower(u.Host) {
	case allowedSiteHost:
		return nil
	case allowedGitHubHost:
		if strings.HasPrefix(strings.ToLower(u.Path), allowedGitHubPath) && len(u.Path) > len(allowedGitHubPath) {
			return nil
		}
		return bad("github link must be under %s", allowedGitHubPath)
	}
	return bad("link host %q is not allowed", u.Host)
}

func validateText(field, s string, limit int, required bool) error {
	if required && strings.TrimSpace(s) == "" {
		return bad("%s is empty", field)
	}
	if utf8.RuneCountInString(s) > limit {
		return bad("%s is longer than %d characters", field, limit)
	}
	for _, r := range s {
		if r == '\n' || r == '\t' {
			continue
		}
		if unicode.IsControl(r) {
			return bad("%s contains a control character", field)
		}
	}
	if htmlPattern.MatchString(s) {
		return bad("%s contains HTML", field)
	}
	return nil
}

func validateBody(body string) error {
	if strings.Contains(body, "![") {
		return bad("images are not allowed")
	}
	if strings.Contains(body, "```") || strings.Contains(body, "~~~") {
		return bad("code blocks are not allowed")
	}
	if schemePattern.MatchString(body) {
		return bad("script or data links are not allowed")
	}
	for _, m := range mdLinkPattern.FindAllStringSubmatch(body, -1) {
		if err := ValidateLink(strings.TrimSpace(m[2])); err != nil {
			return err
		}
	}
	for _, u := range urlPattern.FindAllString(body, -1) {
		if err := ValidateLink(strings.TrimRight(u, ".,;:!")); err != nil {
			return err
		}
	}
	for _, m := range codePattern.FindAllStringSubmatch(body, -1) {
		if strings.Contains(m[1], "|") || cmdWord.MatchString(m[1]) {
			return bad("inline code looks like a command")
		}
	}
	return nil
}

// ValidateItem checks one item's content and targeting fields.
func ValidateItem(it Item, lim Limits) error {
	if it.ID == "" || len(it.ID) > maxIDLen || !idPattern.MatchString(it.ID) {
		return bad("id %q is not a plain slug", it.ID)
	}
	if !knownTiers[it.Tier] {
		return bad("unknown tier %q", it.Tier)
	}
	if it.IssuedAt.IsZero() || it.ExpiresAt.IsZero() || !it.ExpiresAt.After(it.IssuedAt) {
		return bad("item needs issued_at before expires_at")
	}
	if it.Kind == KindRevokeKey {
		if it.Tier != TierCritical || it.KeyID == "" {
			return bad("revoke_key must be critical and name a key_id")
		}
		return nil
	}
	if err := validateText("title", it.Title, lim.MaxTitle, true); err != nil {
		return err
	}
	if err := validateText("body", it.Body, lim.MaxBody, true); err != nil {
		return err
	}
	if err := validateBody(it.Body); err != nil {
		return err
	}
	if it.CTA != "" && it.CTA != CTAUpdates && it.CTA != CTASettings {
		return bad("cta %q is not an internal route", it.CTA)
	}
	if err := validateTargeting(it); err != nil {
		return err
	}
	if len(it.Links) > maxLinksPerItem {
		return bad("too many links")
	}
	for _, l := range it.Links {
		if err := validateText("link text", l.Text, maxLinkTextRunes, true); err != nil {
			return err
		}
		if err := ValidateLink(l.URL); err != nil {
			return err
		}
	}
	if it.Tier == TierNewsletter {
		if urgencyWord.MatchString(it.Title) || salesWord.MatchString(it.Title+" "+it.Body) {
			return bad("newsletter wording rule")
		}
	}
	return nil
}

func validateTargeting(it Item) error {
	for _, c := range it.Channels {
		if !validChannels[c] {
			return bad("unknown channel %q", c)
		}
	}
	urgent := it.Tier == TierCritical || it.Tier == TierAdvisory
	if it.AppliesTo == "" {
		if urgent {
			return bad("%s items need applies_to", it.Tier)
		}
		return nil
	}
	if _, err := ParseRange(it.AppliesTo); err != nil {
		return bad("applies_to does not parse")
	}
	return nil
}
