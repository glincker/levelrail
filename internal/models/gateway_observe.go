package models

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/http"
	"time"
)

// tokenUsage is the token count an engine reported for one response.
type tokenUsage struct {
	input  int64
	output int64
}

// responseObserver watches a proxied response for its time to first byte and
// its usage object. It keeps only the last scanBytes of a JSON or SSE body,
// never the whole body, and never logs it.
type responseObserver struct {
	start     time.Time
	scanBytes int
	ttft      time.Duration
	seen      bool
	capture   bool
	decided   bool
	tail      []byte
}

func newResponseObserver(start time.Time, scanBytes int) *responseObserver {
	return &responseObserver{start: start, scanBytes: scanBytes}
}

// decide chooses from the response headers whether the body can carry a
// readable usage object: compressed bodies cannot.
func (o *responseObserver) decide(h http.Header) {
	if o.decided {
		return
	}
	o.decided = true
	if o.scanBytes <= 0 || h.Get("Content-Encoding") != "" {
		return
	}
	mt, _, err := mime.ParseMediaType(h.Get("Content-Type"))
	if err != nil {
		return
	}
	o.capture = mt == "application/json" || mt == "text/event-stream"
}

func (o *responseObserver) wrote(h http.Header, p []byte) {
	o.decide(h)
	if !o.seen && len(p) > 0 {
		o.seen = true
		o.ttft = time.Since(o.start)
	}
	if !o.capture {
		return
	}
	o.tail = append(o.tail, p...)
	if len(o.tail) > 2*o.scanBytes {
		o.tail = append([]byte(nil), o.tail[len(o.tail)-o.scanBytes:]...)
	}
}

// usage returns the usage object found in the captured tail, if any.
func (o *responseObserver) usage() (tokenUsage, bool) {
	if !o.capture {
		return tokenUsage{}, false
	}
	return extractUsage(o.tail)
}

var usageKey = []byte(`"usage"`)

// extractUsage finds the last non-null "usage" object in b. It scans from
// the end because engines put usage last, and a stream without
// stream_options.include_usage only carries "usage":null.
func extractUsage(b []byte) (tokenUsage, bool) {
	end := len(b)
	for {
		i := bytes.LastIndex(b[:end], usageKey)
		if i < 0 {
			return tokenUsage{}, false
		}
		end = i
		if i > 0 && b[i-1] == '\\' {
			continue
		}
		rest := bytes.TrimLeft(b[i+len(usageKey):], " \t\r\n")
		if len(rest) == 0 || rest[0] != ':' {
			continue
		}
		rest = bytes.TrimLeft(rest[1:], " \t\r\n")
		if len(rest) == 0 || rest[0] != '{' {
			continue
		}
		var u struct {
			Prompt     *int64 `json:"prompt_tokens"`
			Completion *int64 `json:"completion_tokens"`
			Input      *int64 `json:"input_tokens"`
			Output     *int64 `json:"output_tokens"`
		}
		if err := json.NewDecoder(bytes.NewReader(rest)).Decode(&u); err != nil {
			continue
		}
		in, out := firstSet(u.Prompt, u.Input), firstSet(u.Completion, u.Output)
		if in == nil && out == nil {
			continue
		}
		return tokenUsage{input: max(deref(in), 0), output: max(deref(out), 0)}, true
	}
}

func firstSet(a, b *int64) *int64 {
	if a != nil {
		return a
	}
	return b
}

func deref(p *int64) int64 {
	if p == nil {
		return 0
	}
	return *p
}
