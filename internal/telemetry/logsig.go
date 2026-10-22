package telemetry

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxTemplateLen = 240
	maxSampleLen   = 600
)

// Normalisation steps run in order: whole timestamps and identifiers first so
// the generic number rule never sees their digits.
var (
	sigUUID      = regexp.MustCompile(`\b[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}\b`)
	sigISOTime   = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}[T ]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?(?:Z|[+-]\d{2}:?\d{2})?`)
	sigSlashTime = regexp.MustCompile(`\b\d{4}/\d{2}/\d{2}[ T]\d{2}:\d{2}:\d{2}(?:[.,]\d+)?`)
	sigSyslog    = regexp.MustCompile(`\b(?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec)[a-z]*\s+\d{1,2}\s+\d{2}:\d{2}:\d{2}(?:[.,]\d+)?`)
	sigClock     = regexp.MustCompile(`\b\d{2}:\d{2}:\d{2}(?:[.,]\d+)?\b`)
	sigDate      = regexp.MustCompile(`\b\d{4}-\d{2}-\d{2}\b`)
	sigEmail     = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9\-]+(?:\.[A-Za-z0-9\-]+)+`)
	sigIPv4      = regexp.MustCompile(`\b\d{1,3}(?:\.\d{1,3}){3}(?::\d{1,5})?\b`)
	sigIPv6      = regexp.MustCompile(`\b(?:[0-9a-fA-F]{1,4}:){4,7}[0-9a-fA-F]{1,4}\b`)
	sigHex0x     = regexp.MustCompile(`\b0[xX][0-9a-fA-F]+\b`)
	sigLongHex   = regexp.MustCompile(`\b[0-9a-fA-F]{8,}\b`)
	sigToken     = regexp.MustCompile(`\b[A-Za-z0-9_\-]{24,}\b`)
	sigNumber    = regexp.MustCompile(`-?\b\d+(?:\.\d+)?(?:ms|us|ns|s|m|h|kb|mb|gb|kib|mib|gib|b|%)?\b`)
	sigLevelTok  = regexp.MustCompile(`(?i)(?:\blevel[=:]\s*"?(?:` + levelTokens + `)\b"?|\[\s*(?:` + levelTokens + `)\s*\]|^\s*(?:` + levelTokens + `)\s*:)`)

	secretKV    = regexp.MustCompile(`(?i)\b(password|passwd|pwd|secret|token|api[_-]?key|apikey|access[_-]?key|authorization|auth|cookie|session[_-]?id|private[_-]?key)(["']?\s*[=:]\s*)("[^"]*"|'[^']*'|(?:bearer|basic)\s+\S+|\S+)`)
	secretBear  = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/\-]+=*`)
	secretURI   = regexp.MustCompile(`(?i)([a-z][a-z0-9+.\-]*://[^\s:/@]*:)[^\s@/]+(@)`)
	secretKnown = regexp.MustCompile(`\b(?:sk|pk|ghp|gho|ghs|xox[abp])[-_][A-Za-z0-9\-_]{16,}\b`)
)

// RedactLine masks credentials in a log line before it leaves the store:
// key=value secrets, bearer tokens, URL passwords and well known key shapes.
func RedactLine(s string) string {
	s = secretKV.ReplaceAllString(s, "${1}${2}***")
	s = secretBear.ReplaceAllString(s, "${1} ***")
	s = secretURI.ReplaceAllString(s, "${1}***${2}")
	return secretKnown.ReplaceAllString(s, "***")
}

// mixedAlnum reports whether s holds both a digit and a letter, which is what
// separates an identifier from an ordinary long word.
func mixedAlnum(s string) bool {
	var digit, letter bool
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			letter = true
		}
	}
	return digit && letter
}

// signatureText picks the part of a line that describes the event: the
// message-like field of a JSON line, else the line itself.
func signatureText(e LogEntry) string {
	if e.Structured && e.FieldsJSON != "" {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal([]byte(e.FieldsJSON), &fields); err == nil {
			var parts []string
			for _, key := range []string{"msg", "message", "event", "error", "err", "exception"} {
				raw, ok := fields[key]
				if !ok {
					continue
				}
				var s string
				if err := json.Unmarshal(raw, &s); err == nil && s != "" {
					parts = append(parts, s)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, ": ")
			}
		}
	}
	return e.Message
}

// Normalize reduces a log message to its template: timestamps, ids, numbers,
// addresses and hashes collapse to placeholders so repeated occurrences of
// one error share one string.
func Normalize(msg string) string {
	s := stripBareLevel(sigLevelTok.ReplaceAllString(msg, " "))
	s = sigUUID.ReplaceAllString(s, "<uuid>")
	s = sigISOTime.ReplaceAllString(s, "<ts>")
	s = sigSlashTime.ReplaceAllString(s, "<ts>")
	s = sigSyslog.ReplaceAllString(s, "<ts>")
	s = sigClock.ReplaceAllString(s, "<time>")
	s = sigDate.ReplaceAllString(s, "<date>")
	s = sigEmail.ReplaceAllString(s, "<email>")
	s = sigIPv4.ReplaceAllString(s, "<ip>")
	s = sigIPv6.ReplaceAllString(s, "<ip>")
	s = sigHex0x.ReplaceAllString(s, "<hex>")
	s = sigLongHex.ReplaceAllStringFunc(s, func(m string) string {
		if mixedAlnum(m) {
			return "<hex>"
		}
		return m
	})
	s = sigToken.ReplaceAllStringFunc(s, func(m string) string {
		if mixedAlnum(m) {
			return "<token>"
		}
		return m
	})
	s = sigNumber.ReplaceAllString(s, "<num>")
	s = stripLeadingStamp(condenseSpaces(s))
	return truncateRunes(s, maxTemplateLen)
}

func condenseSpaces(s string) string {
	needsRewrite := false
	inSpace := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		isSpace := (c == ' ' || c == '\t' || c == '\n' || c == '\r')
		if isSpace {
			if inSpace || i == 0 || i == len(s)-1 {
				needsRewrite = true
				break
			}
			if c != ' ' {
				needsRewrite = true
				break
			}
			inSpace = true
		} else {
			inSpace = false
		}
	}
	if !needsRewrite {
		return s
	}

	var b strings.Builder
	b.Grow(len(s))
	inSpace = true // skip leading spaces
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
		} else {
			b.WriteByte(c)
			inSpace = false
		}
	}
	res := b.String()
	if len(res) > 0 && res[len(res)-1] == ' ' {
		return res[:len(res)-1]
	}
	return res
}

// stripBareLevel drops a lone upper case level word near the start of the
// line ("<ts> ERROR msg"), so one error logged at different levels clusters.
func stripBareLevel(s string) string {
	head := s
	if len(head) > levelPrefixLen {
		head = head[:levelPrefixLen]
	}
	loc := upperLevelRE.FindStringIndex(head)
	if loc == nil {
		return s
	}
	return s[:loc[0]] + " " + s[loc[1]:]
}

func stripLeadingStamp(s string) string {
	for _, p := range []string{"<ts>", "<time>", "<date>"} {
		for strings.HasPrefix(s, p) {
			s = strings.TrimSpace(strings.TrimPrefix(s, p))
		}
	}
	return s
}

func truncateRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	s = s[:n]
	for !utf8.ValidString(s) && len(s) > 0 {
		s = s[:len(s)-1]
	}
	return s
}

// SignatureOf returns the stable id and template of an entry's message.
func SignatureOf(e LogEntry) (id, template string) {
	template = Normalize(signatureText(e))
	sum := sha256.Sum256([]byte(template))
	return hex.EncodeToString(sum[:6]), template
}

// Signature is one cluster of similar log lines.
type Signature struct {
	ID        string    `json:"id"`
	Template  string    `json:"template"`
	Level     string    `json:"level"`
	Count     int       `json:"count"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	// Buckets is the per-interval count across the query window, for a sparkline.
	Buckets []int `json:"buckets"`
	// Sample is the newest line of the cluster, redacted and truncated.
	Sample string `json:"sample"`
	// New is set when the signature has no occurrence before the baseline cut.
	New bool `json:"new,omitempty"`
}

// SignatureOptions tunes AggregateSignatures.
type SignatureOptions struct {
	// MinLevel keeps lines at or above this level; "" keeps every line.
	MinLevel string
	// Buckets is the sparkline resolution; zero means 24.
	Buckets int
	// Top caps the result; zero means no cap.
	Top int
}

// AggregateSignatures clusters entries inside [from, to] by signature, most
// frequent first (ties by most recent).
func AggregateSignatures(entries []LogEntry, from, to time.Time, opts SignatureOptions) []Signature {
	n := opts.Buckets
	if n <= 0 {
		n = 24
	}
	span := to.Sub(from)
	minRank := LevelRank(opts.MinLevel)
	byID := map[string]*Signature{}
	for _, e := range entries {
		level := ClassifyLevel(e)
		if minRank > 0 && LevelRank(level) < minRank {
			continue
		}
		id, tpl := SignatureOf(e)
		if tpl == "" {
			continue
		}
		s := byID[id]
		if s == nil {
			s = &Signature{ID: id, Template: tpl, Level: level, FirstSeen: e.Timestamp, Buckets: make([]int, n)}
			byID[id] = s
		}
		s.Count++
		if e.Timestamp.Before(s.FirstSeen) {
			s.FirstSeen = e.Timestamp
		}
		if !e.Timestamp.Before(s.LastSeen) {
			s.LastSeen = e.Timestamp
			s.Sample = truncateRunes(RedactLine(e.Message), maxSampleLen)
		}
		if LevelRank(level) > LevelRank(s.Level) {
			s.Level = level
		}
		if span > 0 {
			i := int(float64(e.Timestamp.Sub(from)) / float64(span) * float64(n))
			s.Buckets[min(max(i, 0), n-1)]++
		}
	}
	out := make([]Signature, 0, len(byID))
	for _, s := range byID {
		out = append(out, *s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		if !out[i].LastSeen.Equal(out[j].LastSeen) {
			return out[i].LastSeen.After(out[j].LastSeen)
		}
		return out[i].ID < out[j].ID
	})
	if opts.Top > 0 && len(out) > opts.Top {
		out = out[:opts.Top]
	}
	return out
}

// MarkNewSignatures sets New on every signature in current that does not
// appear in baseline.
func MarkNewSignatures(current, baseline []Signature) (newCount int) {
	seen := make(map[string]struct{}, len(baseline))
	for _, b := range baseline {
		seen[b.ID] = struct{}{}
	}
	for i := range current {
		if _, ok := seen[current[i].ID]; !ok {
			current[i].New = true
			newCount++
		}
	}
	return newCount
}
