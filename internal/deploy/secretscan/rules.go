package secretscan

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"strings"

	"github.com/GLINCKER/levelrail/internal/envredact"
)

// Rule is one token format. Group is the capture group holding the secret
// (0 is the whole match).
type Rule struct {
	ID    string
	Desc  string
	Re    *regexp.Regexp
	Group int
}

// RuleURLCredentials and RuleHighEntropy are rules with extra handling.
const (
	RuleURLCredentials = "url-credentials"
	RuleHighEntropy    = "high-entropy-secret"
)

// Rules is the built-in ruleset.
var Rules = []Rule{
	{ID: "aws-access-key-id", Desc: "AWS access key id", Re: regexp.MustCompile(`\b(?:AKIA|ASIA|ABIA|ACCA)[0-9A-Z]{16}\b`)},
	{ID: "aws-secret-access-key", Desc: "AWS secret access key", Re: regexp.MustCompile(`(?i)aws[\w .-]{0,24}secret[\w .-]{0,24}['"=:\s]{1,4}([A-Za-z0-9/+=]{40})`), Group: 1},
	{ID: "gcp-api-key", Desc: "Google API key", Re: regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{35}\b`)},
	{ID: "azure-storage-key", Desc: "Azure storage account key", Re: regexp.MustCompile(`AccountKey=([A-Za-z0-9+/]{60,}={0,2})`), Group: 1},
	{ID: "private-key", Desc: "private key block", Re: regexp.MustCompile(`-----BEGIN (?:RSA |EC |DSA |OPENSSH |PGP |ENCRYPTED )?PRIVATE KEY(?: BLOCK)?-----`)},
	{ID: "github-token", Desc: "GitHub token", Re: regexp.MustCompile(`\bgh[pousr]_[A-Za-z0-9]{36,255}\b`)},
	{ID: "github-fine-grained-token", Desc: "GitHub fine-grained token", Re: regexp.MustCompile(`\bgithub_pat_[A-Za-z0-9_]{22,255}\b`)},
	{ID: "gitlab-token", Desc: "GitLab personal access token", Re: regexp.MustCompile(`\bglpat-[A-Za-z0-9_-]{20,}\b`)},
	{ID: "slack-token", Desc: "Slack token", Re: regexp.MustCompile(`\bxox[abprs]-[A-Za-z0-9-]{10,}\b`)},
	{ID: "slack-webhook", Desc: "Slack webhook URL", Re: regexp.MustCompile(`https://hooks\.slack\.com/services/T[A-Z0-9]+/B[A-Z0-9]+/[A-Za-z0-9]+`)},
	{ID: "stripe-secret-key", Desc: "Stripe live secret key", Re: regexp.MustCompile(`\b[sr]k_live_[0-9A-Za-z]{16,}\b`)},
	{ID: "sendgrid-key", Desc: "SendGrid API key", Re: regexp.MustCompile(`\bSG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}\b`)},
	{ID: "npm-token", Desc: "npm access token", Re: regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`)},
	{ID: "jwt", Desc: "JSON web token", Re: regexp.MustCompile(`\beyJ[A-Za-z0-9_-]{8,}\.eyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)},
	{ID: RuleURLCredentials, Desc: "password embedded in a URL", Re: regexp.MustCompile(`\b[a-z][a-z0-9+.-]{1,20}://[^\s:/@'"]{1,64}:([^\s:/@'"]{8,128})@[^\s/'"]+`), Group: 1},
}

const (
	entropyMinLen     = 20
	entropyMinBits    = 3.5
	fingerprintLength = 16
)

var placeholderWords = []string{"changeme", "change_me", "example", "placeholder", "your_", "your-", "yourkey", "dummy", "sample", "redacted", "xxxxxxxx", "********", "todo", "insert", "replace"}

// Fingerprint is a stable short hash of the rule and the matched value, so
// an allow-list entry survives a file move without exposing the value.
func Fingerprint(ruleID, value string) string {
	sum := sha256.Sum256([]byte(ruleID + "\x00" + value))
	return hex.EncodeToString(sum[:])[:fingerprintLength]
}

func isPlaceholder(v string) bool {
	for _, p := range []string{"${", "$(", "{{", "<", "%"} {
		if strings.HasPrefix(v, p) {
			return true
		}
	}
	low := strings.ToLower(v)
	for _, w := range placeholderWords {
		if strings.Contains(low, w) {
			return true
		}
	}
	return v != "" && strings.Trim(v, v[:1]) == ""
}

// shannon returns the entropy of s in bits per character.
func shannon(s string) float64 {
	counts := map[rune]int{}
	n := 0
	for _, r := range s {
		counts[r]++
		n++
	}
	var h float64
	for _, c := range counts {
		p := float64(c) / float64(n)
		h -= p * math.Log2(p)
	}
	return h
}

type match struct {
	rule  string
	value string
}

// matchRules applies every pattern to text.
func matchRules(text string) []match {
	var out []match
	for _, r := range Rules {
		for _, loc := range r.Re.FindAllStringSubmatchIndex(text, -1) {
			i := r.Group * 2
			if i+1 >= len(loc) || loc[i] < 0 {
				continue
			}
			val := text[loc[i]:loc[i+1]]
			if r.ID == RuleURLCredentials && isPlaceholder(val) {
				continue
			}
			out = append(out, match{rule: r.ID, value: val})
		}
	}
	return out
}

// namedValueRe finds NAME=value and name: "value" assignments.
var namedValueRe = regexp.MustCompile(`(?i)\b([a-z][a-z0-9_.-]{2,63})\b["']?\s*[:=]\s*["']?([^\s"'#,;]{20,256})`)

// entropyMatches flags high-entropy values assigned to credential-named variables.
func entropyMatches(line string) []match {
	var out []match
	for _, m := range namedValueRe.FindAllStringSubmatch(line, -1) {
		if entropyValueOK(m[1], m[2]) {
			out = append(out, match{rule: RuleHighEntropy, value: m[2]})
		}
	}
	return out
}

func entropyValueOK(name, value string) bool {
	if !envredact.SensitiveName(name) || envredact.IsPublicName(name) {
		return false
	}
	if len(value) < entropyMinLen || isPlaceholder(value) || shannon(value) < entropyMinBits {
		return false
	}
	return !strings.Contains(value, "://") || strings.Contains(value, "@")
}
