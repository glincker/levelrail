package cliconsole

import (
	"bytes"
	"regexp"
	"strings"
	"unicode"

	"github.com/GLINCKER/levelrail/internal/envredact"
)

var (
	escOSC   = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)?`)
	escCSI   = regexp.MustCompile(`(?:\x1b\[|\x{9b})[0-?]*[ -/]*[@-~]?`)
	escOther = regexp.MustCompile(`\x1b[ -/]*[0-~]?`)
	// secretPair masks the value after a secret-looking key in KEY=VALUE, key: value and JSON.
	secretPair = regexp.MustCompile(`(?i)([A-Za-z0-9_.-]*(?:password|passwd|secret|token|api[_-]?key|private[_-]?key|access[_-]?key|credential|passphrase|dsn)[A-Za-z0-9_.-]*"?\s*[=:]\s*"?)([^\s",}]+)`)
	urlCreds   = regexp.MustCompile(`([A-Za-z][A-Za-z0-9+.-]*://)([^/\s:@]+):([^/\s@]+)@`)
	bearer     = regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]+`)
	pemBlock   = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----`)
)

// SanitizeLine makes one line of child output safe for the browser terminal
// and the audit log: no terminal escapes or control bytes, secrets masked,
// length capped. secrets are exact values to mask, such as the session token.
func SanitizeLine(line string, maxBytes int, secrets ...string) string {
	line = strings.ToValidUTF8(line, "?")
	line = escOSC.ReplaceAllString(line, "")
	line = escCSI.ReplaceAllString(line, "")
	line = escOther.ReplaceAllString(line, "")
	line = strings.Map(func(r rune) rune {
		if r == '\t' {
			return r
		}
		if unicode.IsControl(r) {
			return -1
		}
		return r
	}, line)
	line = envredact.Secrets(line, secrets...)
	if pemBlock.MatchString(line) {
		return envredact.Marker
	}
	line = urlCreds.ReplaceAllString(line, "${1}${2}:"+envredact.Marker+"@")
	line = bearer.ReplaceAllString(line, "${1}"+envredact.Marker)
	line = secretPair.ReplaceAllString(line, "${1}"+envredact.Marker)
	if maxBytes > 0 && len(line) > maxBytes {
		line = strings.ToValidUTF8(line[:maxBytes], "") + "..."
	}
	return line
}

// lineFilter buffers one output stream and releases whole sanitized lines,
// so an escape sequence or a secret split across reads is still caught.
type lineFilter struct {
	buf      []byte
	maxLine  int
	secrets  []string
	skipping bool
}

func (f *lineFilter) write(p []byte) []byte {
	f.buf = append(f.buf, p...)
	var out []byte
	for {
		i := bytes.IndexByte(f.buf, '\n')
		if i < 0 {
			break
		}
		if !f.skipping {
			out = append(out, SanitizeLine(string(f.buf[:i]), f.maxLine, f.secrets...)...)
			out = append(out, '\n')
		}
		f.skipping = false
		f.buf = f.buf[i+1:]
	}
	// A line longer than the cap is released truncated and the rest dropped.
	if f.maxLine > 0 && len(f.buf) > f.maxLine && !f.skipping {
		out = append(out, SanitizeLine(string(f.buf), f.maxLine, f.secrets...)...)
		out = append(out, '\n')
		f.buf = f.buf[:0]
		f.skipping = true
	} else if f.skipping {
		f.buf = f.buf[:0]
	}
	return out
}

func (f *lineFilter) flush() []byte {
	if len(f.buf) == 0 || f.skipping {
		f.buf = nil
		return nil
	}
	out := SanitizeLine(string(f.buf), f.maxLine, f.secrets...)
	f.buf = nil
	return []byte(out)
}
