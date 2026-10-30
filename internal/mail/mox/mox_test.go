package mox

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/mail"
)

const testPW = "s3cret-admin-pw"

type creds struct{ pw string }

func (c creds) AdminPassword(context.Context) (string, error) { return c.pw, nil }

// fake implements the verified Sherpa session protocol.
type fake struct {
	mu      sync.Mutex
	session string
	csrf    string
	logins  int
	calls   []string
	handler func(fn string, params []json.RawMessage) (any, *rpcError)
	delay   time.Duration
	blob    int
}

func (f *fake) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	fn := strings.TrimPrefix(r.URL.Path, "/admin/api/")
	var body struct {
		Params []json.RawMessage `json:"params"`
	}
	_ = json.NewDecoder(r.Body).Decode(&body)
	reply := func(res any, e *rpcError) {
		w.Header().Set("Content-Type", "application/json")
		if e != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"error": e})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"result": res})
	}
	switch fn {
	case "LoginPrep":
		reply("login-token", nil)
		return
	case "Login":
		var tok, pw string
		_ = json.Unmarshal(body.Params[0], &tok)
		_ = json.Unmarshal(body.Params[1], &pw)
		if tok != "login-token" || pw != testPW {
			reply(nil, &rpcError{Code: "user:error", Message: "bad credentials for " + pw})
			return
		}
		f.logins++
		f.session = "sess" + strconv.Itoa(f.logins)
		f.csrf = "csrf" + strconv.Itoa(f.logins)
		http.SetCookie(w, &http.Cookie{Name: "webadminsession", Value: f.session, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		reply(f.csrf, nil)
		return
	}
	ck, err := r.Cookie("webadminsession")
	if err != nil || ck.Value != f.session || r.Header.Get("x-mox-csrf") != f.csrf || r.Header.Get("Content-Type") != "application/json" {
		reply(nil, &rpcError{Code: "user:noAuth", Message: "no auth"})
		return
	}
	f.calls = append(f.calls, fn)
	if f.blob > 0 {
		reply(strings.Repeat("x", f.blob), nil)
		return
	}
	res, e := f.handler(fn, body.Params)
	reply(res, e)
}

func newClient(t *testing.T, f *fake) *Client {
	t.Helper()
	srv := httptest.NewServer(f)
	t.Cleanup(srv.Close)
	c, err := New(Config{BaseURL: srv.URL, Credentials: creds{testPW}})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEngineMethods(t *testing.T) {
	f := &fake{handler: func(fn string, p []json.RawMessage) (any, *rpcError) {
		switch fn {
		case "Domains":
			return []string{"example.test"}, nil
		case "DomainRecords":
			return []string{
				"; comment",
				"example.test. 300 IN MX 10 mail.example.test.",
				`2026a._domainkey.example.test. IN TXT "v=DKIM1;h=sha256;k=rsa;" "p=AB CD"`,
			}, nil
		case "Accounts":
			return [][]string{{"admin", "alice"}, {"alice"}}, nil
		case "QueueSize":
			return 2, nil
		case "QueueList":
			return []map[string]any{{"Attempts": 1, "Results": []map[string]any{{"Success": false, "Error": "dns"}}}, {"Attempts": 0}}, nil
		case "AliasAdd":
			var local, dom string
			_ = json.Unmarshal(p[0], &local)
			_ = json.Unmarshal(p[1], &dom)
			if local != "team" || dom != "example.test" {
				return nil, &rpcError{Code: "user:error", Message: "bad alias"}
			}
			return nil, nil
		}
		return nil, nil
	}}
	c := newClient(t, f)
	ctx := context.Background()

	if d, err := c.Domains(ctx); err != nil || len(d) != 1 || d[0].Name != "example.test" {
		t.Fatalf("domains %v %v", d, err)
	}
	recs, err := c.DNSRecords(ctx, "example.test")
	if err != nil || len(recs) != 2 || recs[0].Type != "MX" || recs[0].TTL != 300 {
		t.Fatalf("records %+v %v", recs, err)
	}
	keys, err := c.DKIMKeys(ctx, "example.test")
	if err != nil || len(keys) != 1 || keys[0].Selector != "2026a" || keys[0].PublicKey != "ABCD" {
		t.Fatalf("keys %+v %v", keys, err)
	}
	if a, err := c.Accounts(ctx); err != nil || len(a) != 2 || a[0].Disabled || !a[1].Disabled {
		t.Fatalf("accounts %v %v", a, err)
	}
	st, err := c.QueueStats(ctx)
	if err != nil || st.Queued != 2 || st.Failing != 1 {
		t.Fatalf("stats %+v %v", st, err)
	}
	steps := []error{
		c.AddDomain(ctx, mail.DomainSpec{Name: "example.test", Account: "admin"}),
		c.RemoveDomain(ctx, "example.test"),
		c.AddAccount(ctx, mail.AccountSpec{Name: "bob", Address: "bob@example.test", Password: "pw-1"}),
		c.RemoveAccount(ctx, "bob"),
		c.AddAddress(ctx, "b2@example.test", "bob"),
		c.AddAlias(ctx, mail.Alias{Address: "team@example.test", Targets: []string{"alice@example.test"}}),
		c.RemoveAlias(ctx, "team@example.test"),
		c.HoldQueue(ctx),
		c.ReleaseQueue(ctx),
		c.Health(ctx),
	}
	for i, e := range steps {
		if e != nil {
			t.Fatalf("step %d: %v", i, e)
		}
	}
	if err := c.SetQuota(ctx, "bob", mail.Quota{MaxBytes: 1}); !errors.Is(err, mail.ErrUnsupported) {
		t.Fatalf("quota: %v", err)
	}
	if err := c.AddAlias(ctx, mail.Alias{Address: "nodomain", Targets: []string{"a"}}); !errors.Is(err, mail.ErrInvalid) {
		t.Fatalf("alias validation: %v", err)
	}
	if f.logins != 1 {
		t.Fatalf("expected one login, got %d", f.logins)
	}
}

func TestReloginOnNoAuth(t *testing.T) {
	f := &fake{handler: func(string, []json.RawMessage) (any, *rpcError) { return 0, nil }}
	c := newClient(t, f)
	ctx := context.Background()
	if err := c.Health(ctx); err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.session = "expired"
	f.mu.Unlock()
	if err := c.Health(ctx); err != nil {
		t.Fatalf("expected transparent re-login: %v", err)
	}
	if f.logins != 2 {
		t.Fatalf("logins %d", f.logins)
	}
}

func TestWrongPasswordNeverLeaks(t *testing.T) {
	f := &fake{handler: func(string, []json.RawMessage) (any, *rpcError) { return nil, nil }}
	srv := httptest.NewServer(f)
	defer srv.Close()
	const bad = "wrong-pw-xyz"
	c, _ := New(Config{BaseURL: srv.URL, Credentials: creds{bad}})
	err := c.Health(context.Background())
	if err == nil || strings.Contains(err.Error(), bad) {
		t.Fatalf("err %v", err)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		e    rpcError
		want error
	}{
		{rpcError{"user:error", "domain does not exist"}, mail.ErrNotFound},
		{rpcError{"user:error", "account already exists"}, mail.ErrExists},
		{rpcError{"user:error", "weird"}, mail.ErrInvalid},
		{rpcError{"sherpa:badParams", "x"}, mail.ErrInvalid},
	}
	for _, tc := range cases {
		e := tc.e
		f := &fake{handler: func(string, []json.RawMessage) (any, *rpcError) { return nil, &e }}
		c := newClient(t, f)
		if err := c.RemoveDomain(context.Background(), "d"); !errors.Is(err, tc.want) {
			t.Errorf("%v: got %v", tc.e, err)
		}
	}
	f := &fake{handler: func(string, []json.RawMessage) (any, *rpcError) {
		return nil, &rpcError{Code: "server:error", Message: "boom"}
	}}
	err := newClient(t, f).RemoveDomain(context.Background(), "d")
	if err == nil || errors.Is(err, mail.ErrInvalid) || errors.Is(err, mail.ErrNotFound) {
		t.Fatalf("server error: %v", err)
	}
}

func TestPasswordArgumentRedacted(t *testing.T) {
	const pw = "hunter2-argument"
	f := &fake{handler: func(string, []json.RawMessage) (any, *rpcError) {
		return nil, &rpcError{Code: "user:error", Message: "password " + pw + " too weak"}
	}}
	err := newClient(t, f).SetPassword(context.Background(), "alice", pw)
	if err == nil || strings.Contains(err.Error(), pw) || strings.Contains(err.Error(), testPW) {
		t.Fatalf("leak: %v", err)
	}
}

func TestTimeout(t *testing.T) {
	t.Setenv(TimeoutEnv, "50ms")
	f := &fake{delay: 300 * time.Millisecond, handler: func(string, []json.RawMessage) (any, *rpcError) { return 0, nil }}
	err := newClient(t, f).Health(context.Background())
	if err == nil || strings.Contains(err.Error(), testPW) {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestOversizedBodyRejected(t *testing.T) {
	t.Setenv(MaxBytesEnv, "2048")
	f := &fake{blob: 10000, handler: func(string, []json.RawMessage) (any, *rpcError) { return nil, nil }}
	err := newClient(t, f).RemoveDomain(context.Background(), "d")
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestBaseURLRules(t *testing.T) {
	bad := []string{"ftp://10.0.0.1", "http://169.254.169.254", "http://[fe80::1]:1080", "http://100.100.100.200", "http://[fd00:ec2::254%25eth0]", "http://[fe80::1%25eth0]", "http://[::ffff:169.254.169.254]", "", "10.0.0.1:80"}
	for _, u := range bad {
		if _, err := New(Config{BaseURL: u, Credentials: creds{"x"}}); !errors.Is(err, mail.ErrInvalid) {
			t.Errorf("%q accepted: %v", u, err)
		}
	}
	for _, u := range []string{"http://127.0.0.1:1080", "https://10.0.0.5", "http://mail.internal:1080"} {
		if _, err := New(Config{BaseURL: u, Credentials: creds{"x"}}); err != nil {
			t.Errorf("%q refused: %v", u, err)
		}
	}
	if _, err := New(Config{BaseURL: "http://127.0.0.1"}); err == nil {
		t.Error("missing credentials accepted")
	}
}

func TestParseRecords(t *testing.T) {
	recs := ParseRecords([]string{
		"",
		"example.test. IN TXT \"v=spf1 mx -all\"",
		"_dmarc.example.test. 3600 IN TXT \"v=DMARC1; p=reject\" ; note",
		"x._domainkey.example.test. IN TXT \"v=DKIM1;k=ed25519;\" \"p=QUJD\"",
		"short",
	})
	if len(recs) != 3 || recs[0].Value != "v=spf1 mx -all" || recs[1].TTL != 3600 || recs[1].Value != "v=DMARC1; p=reject" {
		t.Fatalf("%+v", recs)
	}
	k := ExtractDKIM(recs)
	if len(k) != 1 || k[0].Selector != "x" || k[0].PublicKey != "QUJD" {
		t.Fatalf("%+v", k)
	}
}

func TestAdminBaseURL(t *testing.T) {
	t.Setenv(AdminPortEnv, "")
	if got := AdminBaseURL("10.0.0.5"); got != "http://10.0.0.5:8080" {
		t.Fatalf("default %q", got)
	}
	t.Setenv(AdminPortEnv, "9090")
	if got := AdminBaseURL("10.0.0.5"); got != "http://10.0.0.5:9090" {
		t.Fatalf("env %q", got)
	}
}
