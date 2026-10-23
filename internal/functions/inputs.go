package functions

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	envInputEntries   = "FUNCTION_INPUT_MAX_ENTRIES"
	envInputBodyBytes = "FUNCTION_INPUT_MAX_BODY_BYTES"
	envInputRetention = "FUNCTION_INPUT_RETENTION"
	envBodyBytes      = "FUNCTION_TEST_MAX_BODY_BYTES"
	envResponseBytes  = "FUNCTION_TEST_MAX_RESPONSE_BYTES"
	envTimeout        = "FUNCTION_TEST_TIMEOUT"
	envLogPad         = "FUNCTION_LOG_PADDING"

	defaultInputEntries   = 50
	defaultInputBodyBytes = 64 << 10
	defaultInputRetention = 24 * time.Hour
	defaultBodyBytes      = 256 << 10
	defaultResponseBytes  = 256 << 10
	defaultTimeout        = 90 * time.Second
	defaultLogPad         = 5 * time.Second
)

// InvokeLimits are the size and time caps for test invocations and stored inputs.
type InvokeLimits struct {
	InputEntries   int
	InputBodyBytes int
	InputRetention time.Duration
	RequestBytes   int
	ResponseBytes  int
	Timeout        time.Duration
	LogPad         time.Duration
}

// InvokeLimitsFromEnv reads the caps through lookup (os.LookupEnv when nil), falling back to defaults.
func InvokeLimitsFromEnv(lookup func(string) (string, bool)) InvokeLimits {
	if lookup == nil {
		lookup = os.LookupEnv
	}
	num := func(key string, def int) int {
		if v, ok := lookup(key); ok {
			if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
				return n
			}
		}
		return def
	}
	dur := func(key string, def time.Duration) time.Duration {
		if v, ok := lookup(key); ok {
			if d, err := time.ParseDuration(strings.TrimSpace(v)); err == nil && d > 0 {
				return d
			}
		}
		return def
	}
	return InvokeLimits{
		InputEntries:   num(envInputEntries, defaultInputEntries),
		InputBodyBytes: num(envInputBodyBytes, defaultInputBodyBytes),
		InputRetention: dur(envInputRetention, defaultInputRetention),
		RequestBytes:   num(envBodyBytes, defaultBodyBytes),
		ResponseBytes:  num(envResponseBytes, defaultResponseBytes),
		Timeout:        dur(envTimeout, defaultTimeout),
		LogPad:         dur(envLogPad, defaultLogPad),
	}
}

// Input is the stored request of one invocation, kept only for opted-in functions.
type Input struct {
	Function string      `json:"function"`
	Method   string      `json:"method"`
	Path     string      `json:"path"`
	Headers  http.Header `json:"headers"`
	Body     string      `json:"body"`
	At       time.Time   `json:"at"`
}

// InputStore keeps recent inputs in memory, bounded by count, body size and age.
type InputStore struct {
	mu    sync.Mutex
	lim   InvokeLimits
	order []string
	byID  map[string]Input
	nowFn func() time.Time
}

// NewInputStore builds a store honouring lim.
func NewInputStore(lim InvokeLimits) *InputStore {
	return &InputStore{lim: lim, byID: map[string]Input{}, nowFn: time.Now}
}

var redactedHeaders = map[string]bool{
	"authorization":       true,
	"cookie":              true,
	"proxy-authorization": true,
	"set-cookie":          true,
	"x-api-key":           true,
}

// RedactHeaders returns a copy of h without credential headers.
func RedactHeaders(h http.Header) http.Header {
	out := http.Header{}
	for k, v := range h {
		if redactedHeaders[strings.ToLower(k)] {
			continue
		}
		out[k] = append([]string(nil), v...)
	}
	return out
}

// Put stores in under id and drops the oldest entries past the count cap. A body past the byte cap is cut.
func (s *InputStore) Put(id string, in Input) {
	if s == nil || id == "" {
		return
	}
	if len(in.Body) > s.lim.InputBodyBytes {
		in.Body = in.Body[:s.lim.InputBodyBytes]
	}
	in.Headers = RedactHeaders(in.Headers)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	if _, ok := s.byID[id]; !ok {
		s.order = append(s.order, id)
	}
	s.byID[id] = in
	for len(s.order) > s.lim.InputEntries {
		delete(s.byID, s.order[0])
		s.order = s.order[1:]
	}
}

// Get returns the stored input for id when it has not expired.
func (s *InputStore) Get(id string) (Input, bool) {
	if s == nil {
		return Input{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked()
	in, ok := s.byID[id]
	return in, ok
}

// Forget drops every stored input of a function, used when capture is turned off.
func (s *InputStore) Forget(function string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	kept := s.order[:0]
	for _, id := range s.order {
		if s.byID[id].Function == function {
			delete(s.byID, id)
			continue
		}
		kept = append(kept, id)
	}
	s.order = kept
}

func (s *InputStore) pruneLocked() {
	cutoff := s.nowFn().Add(-s.lim.InputRetention)
	for len(s.order) > 0 {
		id := s.order[0]
		if !s.byID[id].At.Before(cutoff) {
			return
		}
		delete(s.byID, id)
		s.order = s.order[1:]
	}
}
