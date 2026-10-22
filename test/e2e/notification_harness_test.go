package e2e

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

// fakeVendorRequest is one request a fake vendor endpoint received.
type fakeVendorRequest struct {
	Method string
	Path   string
	Query  url.Values
	Header http.Header
	Body   []byte
}

// JSON decodes the request body into a generic map for contract checks.
func (r fakeVendorRequest) JSON(t *testing.T) map[string]any {
	t.Helper()
	var out map[string]any
	if err := json.Unmarshal(r.Body, &out); err != nil {
		t.Fatalf("vendor request body %q is not a JSON object: %v", r.Body, err)
	}
	return out
}

// fakeVendor is an httptest server that records every request and can be
// told to answer 5xx for its next N requests (a transient outage) or for
// every request (a hard outage), so retry and failure history are
// exercised against a receiver that behaves like a real vendor edge.
type fakeVendor struct {
	srv        *httptest.Server
	mu         sync.Mutex
	requests   []fakeVendorRequest
	failNext   atomic.Int32
	failAlways atomic.Bool
	failStatus atomic.Int32
}

func newFakeVendor(t *testing.T) *fakeVendor {
	t.Helper()
	v := &fakeVendor{}
	v.failStatus.Store(http.StatusServiceUnavailable)
	v.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		v.mu.Lock()
		v.requests = append(v.requests, fakeVendorRequest{
			Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Header: r.Header.Clone(), Body: body,
		})
		v.mu.Unlock()
		if v.failAlways.Load() || v.failNext.Add(-1) >= 0 {
			w.WriteHeader(int(v.failStatus.Load()))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(v.srv.Close)
	return v
}

func (v *fakeVendor) URL() string { return v.srv.URL }

func (v *fakeVendor) snapshot() []fakeVendorRequest {
	v.mu.Lock()
	defer v.mu.Unlock()
	return append([]fakeVendorRequest(nil), v.requests...)
}

func (v *fakeVendor) reset() {
	v.mu.Lock()
	v.requests = nil
	v.mu.Unlock()
	v.failNext.Store(0)
	v.failAlways.Store(false)
}

// vendorHostRedirect sends requests for a vendor's fixed public host to a
// local fake, so kinds that dial a hardcoded endpoint (PagerDuty,
// Opsgenie, Resend) can be tested without changing production code.
type vendorHostRedirect struct {
	routes map[string]*fakeVendor
	base   http.RoundTripper
}

func (r vendorHostRedirect) RoundTrip(req *http.Request) (*http.Response, error) {
	fake, ok := r.routes[req.URL.Host]
	if !ok {
		return r.base.RoundTrip(req)
	}
	target, err := url.Parse(fake.URL())
	if err != nil {
		return nil, err
	}
	clone := req.Clone(req.Context())
	clone.URL.Scheme = target.Scheme
	clone.URL.Host = target.Host
	clone.Host = target.Host
	return r.base.RoundTrip(clone)
}

// fakeSMTP is a minimal SMTP server: plaintext, no auth, one message per
// DATA. failRcptNext makes the next N RCPT commands answer 421 (a
// transient failure) so the sender's retry path runs for real.
type fakeSMTP struct {
	ln           net.Listener
	mu           sync.Mutex
	messages     []fakeSMTPMessage
	failRcptNext atomic.Int32
	rcptAttempts atomic.Int32
}

type fakeSMTPMessage struct {
	From string
	To   string
	Data string
}

func newFakeSMTP(t *testing.T) *fakeSMTP {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeSMTP{ln: ln}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *fakeSMTP) Addr() string { return s.ln.Addr().String() }

func (s *fakeSMTP) received() []fakeSMTPMessage {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]fakeSMTPMessage(nil), s.messages...)
}

// smtpAddress extracts the address from "<addr> PARAM=..." command arguments.
func smtpAddress(arg string) string {
	arg = strings.TrimSpace(arg)
	if end := strings.Index(arg, ">"); end >= 0 {
		arg = arg[:end]
	}
	return strings.TrimPrefix(arg, "<")
}

func (s *fakeSMTP) serve(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	rd := bufio.NewReader(conn)
	reply := func(line string) { _, _ = fmt.Fprint(conn, line+"\r\n") }
	reply("220 fake.smtp ESMTP")
	var cur fakeSMTPMessage
	for {
		line, err := rd.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))
		switch {
		case strings.HasPrefix(cmd, "EHLO"), strings.HasPrefix(cmd, "HELO"):
			reply("250-fake.smtp")
			reply("250 8BITMIME")
		case strings.HasPrefix(cmd, "MAIL FROM:"):
			cur = fakeSMTPMessage{From: smtpAddress(line[len("MAIL FROM:"):])}
			reply("250 ok")
		case strings.HasPrefix(cmd, "RCPT TO:"):
			s.rcptAttempts.Add(1)
			if s.failRcptNext.Add(-1) >= 0 {
				reply("421 try again later")
				return
			}
			cur.To = smtpAddress(line[len("RCPT TO:"):])
			reply("250 ok")
		case cmd == "DATA":
			reply("354 go ahead")
			var data strings.Builder
			for {
				l, err := rd.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimRight(l, "\r\n") == "." {
					break
				}
				data.WriteString(l)
			}
			cur.Data = data.String()
			s.mu.Lock()
			s.messages = append(s.messages, cur)
			s.mu.Unlock()
			reply("250 queued")
		case cmd == "QUIT":
			reply("221 bye")
			return
		case cmd == "RSET", cmd == "NOOP":
			reply("250 ok")
		default:
			reply("502 not implemented")
		}
	}
}

// recordingPush is a PushSender that records what a browser-push channel
// would have fanned out. It is a stand-in for the push service, not a
// vendor contract.
type recordingPush struct {
	mu    sync.Mutex
	sends []string
}

func (p *recordingPush) Send(_ context.Context, title, body string) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sends = append(p.sends, title+"|"+body)
	return nil
}

func (p *recordingPush) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.sends)
}
