// Package erroringest decodes Sentry SDK payloads into grouped error events.
package erroringest

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"strings"
)

const defaultFingerprintToken = "{{ default }}" //nolint:gosec // Sentry fingerprint placeholder, not a credential

var (
	uuidRe = regexp.MustCompile(`(?i)\b[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}\b`)
	hexRe  = regexp.MustCompile(`(?i)\b(?:0x)?[0-9a-f]{8,}\b`)
	numRe  = regexp.MustCompile(`\d+`)
)

// NormalizeMessage collapses ids and numbers so messages that differ only by
// a request id, port or count land in one issue.
func NormalizeMessage(s string) string {
	s = uuidRe.ReplaceAllString(s, "<id>")
	s = hexRe.ReplaceAllString(s, "<id>")
	s = numRe.ReplaceAllString(s, "<n>")
	return strings.TrimSpace(s)
}

type frame struct {
	Filename string `json:"filename"`
	Module   string `json:"module"`
	Function string `json:"function"`
	InApp    *bool  `json:"in_app"`
}

type exceptionValue struct {
	Type       string `json:"type"`
	Value      string `json:"value"`
	Module     string `json:"module"`
	Stacktrace *struct {
		Frames []frame `json:"frames"`
	} `json:"stacktrace"`
}

// topFrame is the innermost in-app frame (SDKs send frames oldest first),
// or the innermost frame when none is marked in-app.
func topFrame(frames []frame) (frame, bool) {
	for i := len(frames) - 1; i >= 0; i-- {
		if frames[i].InApp != nil && *frames[i].InApp {
			return frames[i], true
		}
	}
	if len(frames) > 0 {
		return frames[len(frames)-1], true
	}
	return frame{}, false
}

// defaultGroupKey is the human-readable string a fingerprint is hashed from
// when the SDK did not supply one: no line numbers, so it survives edits.
func defaultGroupKey(ex *exceptionValue, message string) string {
	if ex != nil && ex.Type != "" {
		parts := []string{"exc", ex.Type}
		if ex.Stacktrace != nil {
			if f, ok := topFrame(ex.Stacktrace.Frames); ok {
				where := f.Module
				if where == "" {
					where = f.Filename
				}
				parts = append(parts, where, f.Function)
				return strings.Join(parts, "|")
			}
		}
		return strings.Join(append(parts, NormalizeMessage(ex.Value)), "|")
	}
	return "msg|" + NormalizeMessage(message)
}

// fingerprint returns the stable issue id: the SDK's own fingerprint when it
// carries information, otherwise the computed default key.
func fingerprint(client []string, ex *exceptionValue, message string) string {
	def := defaultGroupKey(ex, message)
	key := def
	informative := false
	for _, p := range client {
		if p != defaultFingerprintToken {
			informative = true
			break
		}
	}
	if informative {
		parts := make([]string, len(client))
		for i, p := range client {
			if p == defaultFingerprintToken {
				p = def
			}
			parts[i] = p
		}
		key = "client|" + strings.Join(parts, "\x00")
	}
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:16])
}
