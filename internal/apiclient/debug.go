package apiclient

import (
	"fmt"
	"io"
	"net/url"
	"strings"
	"sync/atomic"
	"time"
)

// debugTarget holds the writer every request traces to, nil (the zero
// value) when tracing is off. atomic.Value rather than a bare var: the
// race detector has no way to see that SetDebugTrace's write always
// happens before any request fires, so this makes that provable instead
// of merely true in practice.
var debugTarget atomic.Value // holds debugTargetBox

type debugTargetBox struct{ w io.Writer }

// SetDebugTrace enables (non-nil w) or disables (nil) request tracing
// for every call through this package, process-wide. cmd/levelrail-cli's
// --debug flag calls this exactly once per invocation, always (even to
// disable), so no state leaks from one CLI run into the next in a
// process that invokes the CLI's entrypoint repeatedly, such as a test
// binary.
func SetDebugTrace(w io.Writer) {
	debugTarget.Store(debugTargetBox{w})
}

func currentDebugWriter() io.Writer {
	box, _ := debugTarget.Load().(debugTargetBox)
	return box.w
}

// sensitiveQueryParams lists query-string keys TraceRequest redacts
// before printing a URL. No current endpoint puts a token in a query
// string, but a debug trace must never assume that stays true.
var sensitiveQueryParams = map[string]bool{
	"token": true, "access_token": true, "api_key": true,
	"apikey": true, "secret": true, "password": true,
}

// redactURL replaces any sensitiveQueryParams value in raw with
// "REDACTED", leaving the rest (including the path, which this project's
// routes never put a credential in) untouched.
func redactURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	q := u.Query()
	redacted := false
	for key := range q {
		if sensitiveQueryParams[strings.ToLower(key)] {
			q.Set(key, "REDACTED")
			redacted = true
		}
	}
	if redacted {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// TraceRequest writes one --debug trace line for a completed (or failed)
// HTTP round trip, a no-op unless SetDebugTrace enabled tracing.
// Exported so cmd/levelrail-cli's authSessionClient (a separate,
// session-cookie based client that is never a Client, see its own doc
// comment) can share this exact trace format and URL redaction instead
// of a second implementation. Never passed a header or body: this never
// prints an Authorization value or any other credential, in or out of
// debug mode.
func TraceRequest(method, rawURL, status string, elapsed time.Duration, reqErr error) {
	w := currentDebugWriter()
	if w == nil {
		return
	}
	safeURL := redactURL(rawURL)
	if reqErr != nil {
		_, _ = fmt.Fprintf(w, "DEBUG: %s %s -> error: %v (%s)\n", method, safeURL, reqErr, elapsed.Round(time.Millisecond))
		return
	}
	_, _ = fmt.Fprintf(w, "DEBUG: %s %s -> %s (%s)\n", method, safeURL, status, elapsed.Round(time.Millisecond))
}
