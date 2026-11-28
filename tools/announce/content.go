package main

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	htmlTagRe    = regexp.MustCompile(`<[A-Za-z/!?]`)
	entityRe     = regexp.MustCompile(`&#?[A-Za-z0-9]{1,10};`)
	mdLinkRe     = regexp.MustCompile(`\[([^\]]*)\]\(([^)\s]*)\)`)
	bareURLRe    = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s)\]>"']+`)
	badSchemeRe  = regexp.MustCompile(`(?i)\b(javascript|data|vbscript|file|blob):`)
	codeSpanRe   = regexp.MustCompile("`([^`]*)`")
	codeTokenRe  = regexp.MustCompile(`(?i)(\||\b(curl|wget|sh|bash|zsh|iex|sudo|powershell|eval)\b)`)
	commandRe    = regexp.MustCompile(`(?i)(\bcurl\s|\bwget\s|\bsudo\s|\|\s*(ba|z)?sh\b|\biex\b|invoke-expression|\bchmod\s+[+0-9]|\brm\s+-)`)
	paidRe       = regexp.MustCompile(`(?i)\b(paid|enterprise|pricing|price|premium|subscription|per[- ]seat|license fee)\b`)
	limitRe      = regexp.MustCompile(`(?i)\b(limitations?|not supported|unsupported|does not support|doesn't support|only supports|maximum of)\b`)
	competitorRe = regexp.MustCompile(`(?i)\b(better than|faster than|cheaper than|unlike|beats|alternative to|replaces|versus|vs\.?)\b`)
	urgencyRe    = regexp.MustCompile(`(?i)\b(now|urgent|urgently|immediately|asap|today|hurry)\b`)
	secretRes    = []*regexp.Regexp{
		regexp.MustCompile(`gh[pousr]_[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`github_pat_[A-Za-z0-9_]{20,}`),
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY`),
		regexp.MustCompile(`xox[abprs]-[A-Za-z0-9-]{10,}`),
		regexp.MustCompile(`\bsk-[A-Za-z0-9]{20,}`),
		regexp.MustCompile(`eyJ[A-Za-z0-9_-]{10,}\.[A-Za-z0-9_-]{10,}\.`),
		regexp.MustCompile(`\b[0-9a-fA-F]{40,}\b`),
		regexp.MustCompile(`[A-Za-z0-9+/_-]{40,}={0,2}`),
	}
)

// linkProblem returns "" when the URL is allowed: https only, either
// levelrail.com or a github.com/glincker/ path, no query, fragment or port.
func linkProblem(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "unparseable link " + raw
	}
	switch {
	case u.Scheme != "https":
		return "link must be https: " + raw
	case u.User != nil, u.Port() != "", u.RawQuery != "", u.ForceQuery, u.Fragment != "", u.RawFragment != "":
		return "link may not carry credentials, port, query or fragment: " + raw
	case strings.Contains(u.Path, ".."), strings.Contains(u.EscapedPath(), "%"):
		return "link path may not contain dot segments or escapes: " + raw
	}
	switch host := strings.ToLower(u.Hostname()); host {
	case "levelrail.com", "www.levelrail.com":
		return ""
	case "github.com":
		p := strings.ToLower(u.Path)
		if strings.HasPrefix(p, "/glincker/") && len(p) > len("/glincker/") {
			return ""
		}
		return "github link must be under /glincker/: " + raw
	default:
		return "link host not allowlisted: " + raw
	}
}

// textProblems checks one title or body. multiline allows newlines.
func textProblems(field, s string, runeCap int, multiline bool) []string {
	var out []string
	add := func(f string, a ...any) { out = append(out, field+": "+fmt.Sprintf(f, a...)) }
	if !utf8.ValidString(s) {
		return []string{field + ": invalid utf-8"}
	}
	if strings.TrimSpace(s) == "" {
		add("empty")
	}
	if n := utf8.RuneCountInString(s); n > runeCap {
		add("%d characters, cap is %d", n, runeCap)
	}
	for _, r := range s {
		bad := unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t'))
		bidi := (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069) || r == 0x200B || r == 0xFEFF
		if bad || bidi {
			add("control or invisible character U+%04X", r)
			break
		}
	}
	if htmlTagRe.MatchString(s) || entityRe.MatchString(s) {
		add("html is not allowed")
	}
	if strings.Contains(s, "![") {
		add("images are not allowed")
	}
	if strings.Contains(s, "```") || strings.Contains(s, "~~~") {
		add("fenced code blocks are not allowed")
	}
	for _, m := range codeSpanRe.FindAllStringSubmatch(s, -1) {
		if codeTokenRe.MatchString(m[1]) {
			add("inline code looks like a command: %q", m[1])
		}
	}
	if commandRe.MatchString(s) {
		add("text looks like a shell command")
	}
	stripped := s
	for _, m := range mdLinkRe.FindAllStringSubmatch(s, -1) {
		if p := linkProblem(m[2]); p != "" {
			add("%s", p)
		}
		stripped = strings.Replace(stripped, m[0], " ", 1)
	}
	for _, u := range bareURLRe.FindAllString(stripped, -1) {
		u = strings.TrimRight(u, ".,;:!?")
		if p := linkProblem(u); p != "" {
			add("%s", p)
		}
		stripped = strings.Replace(stripped, u, " ", 1)
	}
	if badSchemeRe.MatchString(stripped) {
		add("script or data scheme is not allowed")
	}
	for _, re := range secretRes {
		if re.MatchString(stripped) {
			add("looks like a secret or token")
			break
		}
	}
	return out
}

// wordingProblems applies the tier specific editorial rules.
func wordingProblems(it *Item) []string {
	var out []string
	text := it.Title + "\n" + it.Body
	if it.Tier == "newsletter" || it.Tier == "feature" {
		if paidRe.MatchString(text) {
			out = append(out, it.Tier+": paid, pricing or enterprise wording is not allowed")
		}
	}
	if it.Tier == "newsletter" {
		if limitRe.MatchString(text) {
			out = append(out, "newsletter: statements of product limits are not allowed")
		}
		if competitorRe.MatchString(text) {
			out = append(out, "newsletter: comparison or competitor claims are not allowed")
		}
		if urgencyRe.MatchString(it.Title) {
			out = append(out, "newsletter: urgency words are not allowed in titles")
		}
	}
	return out
}
