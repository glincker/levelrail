package probe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func boolPtr(b bool) *bool { return &b }

func asFailure(t *testing.T, err error) *Failure {
	t.Helper()
	var f *Failure
	if !errors.As(err, &f) {
		t.Fatalf("error %v (%T) is not a *Failure", err, err)
	}
	return f
}

func TestCheck_HTTPS(t *testing.T) {
	var gotHost string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHost = r.Host
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	addr := strings.TrimPrefix(srv.URL, "https://")

	tests := []struct {
		name     string
		client   *http.Client
		cfg      Config
		wantKind FailureKind
		wantHost string
		wantMsg  string
	}{
		{name: "self-signed without skip fails verification", client: &http.Client{}, cfg: Config{Path: "/", Scheme: SchemeHTTPS}, wantKind: FailureTLS, wantMsg: "set tls_skip_verify"},
		{name: "self-signed with skip passes", client: &http.Client{}, cfg: Config{Path: "/", Scheme: SchemeHTTPS, TLSSkipVerify: true}},
		{name: "trusted cert with host sets SNI and Host header", client: srv.Client(), cfg: Config{Path: "/", Scheme: SchemeHTTPS, Host: "example.com"}, wantHost: "example.com"},
		{name: "plain http against a TLS port fails", client: srv.Client(), cfg: Config{Path: "/"}, wantKind: FailureStatus, wantMsg: "returned 400"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotHost = ""
			err := New(tt.client, nil, Limits{}).Check(context.Background(), Target{Addr: addr}, withTimeout(tt.cfg))
			if tt.wantKind == "" {
				if err != nil {
					t.Fatalf("Check() error = %v, want nil", err)
				}
				if tt.wantHost != "" && gotHost != tt.wantHost {
					t.Errorf("Host header = %q, want %q", gotHost, tt.wantHost)
				}
				return
			}
			f := asFailure(t, err)
			if f.Kind != tt.wantKind || !strings.Contains(f.Reason, tt.wantMsg) {
				t.Errorf("failure = %s %q, want kind %s containing %q", f.Kind, f.Reason, tt.wantKind, tt.wantMsg)
			}
		})
	}
}

func withTimeout(cfg Config) Config {
	cfg.Timeout = 2 * time.Second
	return cfg
}

func TestCheck_Redirects(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/login", http.StatusFound) })
	mux.HandleFunc("/login", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/loop", func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, "/loop", http.StatusFound) })
	srv := httptest.NewServer(mux)
	defer srv.Close()

	tests := []struct {
		name     string
		cfg      Config
		limits   Limits
		wantKind FailureKind
		wantMsg  string
	}{
		{name: "unset follows like the historical client", cfg: Config{Path: "/"}},
		{name: "explicit follow", cfg: Config{Path: "/", FollowRedirects: boolPtr(true)}},
		{name: "follow off judges the 302 itself", cfg: Config{Path: "/", FollowRedirects: boolPtr(false)}, wantKind: FailureRedirect, wantMsg: "returned 302 to /login; set follow_redirects or expected_status"},
		{name: "follow off with 3xx accepted", cfg: Config{Path: "/", FollowRedirects: boolPtr(false), ExpectedStatus: "200-399"}},
		{name: "redirect loop is bounded", cfg: Config{Path: "/loop", FollowRedirects: boolPtr(true)}, limits: Limits{MaxRedirects: 2}, wantKind: FailureTooManyHops, wantMsg: "more than 2 redirects"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := New(srv.Client(), nil, tt.limits).Check(context.Background(), Target{Addr: hostPort(t, srv)}, withTimeout(tt.cfg))
			if tt.wantKind == "" {
				if err != nil {
					t.Fatalf("Check() error = %v, want nil", err)
				}
				return
			}
			f := asFailure(t, err)
			if f.Kind != tt.wantKind || !strings.Contains(f.Reason, tt.wantMsg) {
				t.Errorf("failure = %s %q, want kind %s containing %q", f.Kind, f.Reason, tt.wantKind, tt.wantMsg)
			}
		})
	}
}

func TestCheck_ExpectedStatus(t *testing.T) {
	tests := []struct {
		status   int
		expected string
		wantOK   bool
	}{
		{status: 200, expected: "", wantOK: true},
		{status: 204, expected: "", wantOK: true},
		{status: 401, expected: "", wantOK: false},
		{status: 401, expected: "200-399,401", wantOK: true},
		{status: 503, expected: "200-499", wantOK: false},
		{status: 301, expected: "200 301-302", wantOK: true},
	}
	for _, tt := range tests {
		t.Run(http.StatusText(tt.status)+"/"+tt.expected, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tt.status >= 300 && tt.status < 400 {
					w.Header().Set("Location", "/elsewhere")
				}
				w.WriteHeader(tt.status)
			}))
			defer srv.Close()

			cfg := withTimeout(Config{Path: "/", ExpectedStatus: tt.expected, FollowRedirects: boolPtr(false)})
			err := New(srv.Client(), nil, Limits{}).Check(context.Background(), Target{Addr: hostPort(t, srv)}, cfg)
			if (err == nil) != tt.wantOK {
				t.Errorf("Check() error = %v, wantOK %v", err, tt.wantOK)
			}
		})
	}
}

func TestParseStatusSet(t *testing.T) {
	tests := []struct {
		in       string
		want     string
		contains []int
		excludes []int
		wantErr  string
	}{
		{in: "", want: "200-299", contains: []int{200, 299}, excludes: []int{199, 300}},
		{in: "200-399", want: "200-399", contains: []int{302}, excludes: []int{404}},
		{in: " 200, 204 ,301-302", want: "200,204,301-302", contains: []int{204, 302}, excludes: []int{201, 303}},
		{in: "399-200", wantErr: "runs backwards"},
		{in: "abc", wantErr: "not an HTTP status code"},
		{in: "600", wantErr: "not an HTTP status code"},
		{in: "200-", wantErr: "not an HTTP status code"},
	}
	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			set, err := ParseStatusSet(tt.in)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("ParseStatusSet(%q) error = %v, want containing %q", tt.in, err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseStatusSet(%q) error = %v", tt.in, err)
			}
			if set.String() != tt.want {
				t.Errorf("String() = %q, want %q", set.String(), tt.want)
			}
			for _, c := range tt.contains {
				if !set.Contains(c) {
					t.Errorf("Contains(%d) = false, want true", c)
				}
			}
			for _, c := range tt.excludes {
				if set.Contains(c) {
					t.Errorf("Contains(%d) = true, want false", c)
				}
			}
		})
	}
}

func TestCheck_TimeoutReason(t *testing.T) {
	block := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-block
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	defer close(block)

	err := New(srv.Client(), nil, Limits{}).Check(context.Background(), Target{Addr: hostPort(t, srv)}, Config{Path: "/slow", Timeout: 30 * time.Millisecond})
	f := asFailure(t, err)
	if f.Kind != FailureTimeout || !strings.Contains(f.Reason, "GET http://") || !strings.Contains(f.Reason, "/slow timed out after 30ms") {
		t.Errorf("failure = %s %q", f.Kind, f.Reason)
	}
}

func TestCheck_ConnectionRefusedReason(t *testing.T) {
	err := New(nil, nil, Limits{}).Check(context.Background(), Target{Addr: "127.0.0.1:1"}, Config{Path: "/", Timeout: time.Second})
	f := asFailure(t, err)
	if f.Kind != FailureConnect || !strings.Contains(f.Reason, "GET http://127.0.0.1:1/") {
		t.Errorf("failure = %s %q", f.Kind, f.Reason)
	}
}

func TestWaitReady_WrapsLastFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	err := New(srv.Client(), nil, Limits{}).WaitReady(ctx, Target{Addr: hostPort(t, srv)}, Config{Path: "/", Interval: 20 * time.Millisecond, Timeout: 50 * time.Millisecond})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("error %v does not wrap the context deadline", err)
	}
	if f := asFailure(t, err); f.Kind != FailureStatus || !strings.Contains(f.Reason, "returned 503, expected 200-299") {
		t.Errorf("last failure = %s %q", f.Kind, f.Reason)
	}
}

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		wantErr string
	}{
		{name: "plain http", cfg: Config{Path: "/healthz"}},
		{name: "full https", cfg: Config{Path: "/h", Scheme: "https", Host: "app.example.com:8443", TLSSkipVerify: true, FollowRedirects: boolPtr(true), ExpectedStatus: "200-399"}},
		{name: "exec", cfg: Config{Exec: []string{"pg_isready"}}},
		{name: "neither", cfg: Config{}, wantErr: "path (for an HTTP probe) or exec"},
		{name: "both", cfg: Config{Path: "/", Exec: []string{"true"}}, wantErr: "not both"},
		{name: "relative path", cfg: Config{Path: "healthz"}, wantErr: "must start with /"},
		{name: "bad scheme", cfg: Config{Path: "/", Scheme: "ftp"}, wantErr: "must be http or https"},
		{name: "skip verify on http", cfg: Config{Path: "/", TLSSkipVerify: true}, wantErr: "only applies to scheme: https"},
		{name: "host with scheme", cfg: Config{Path: "/", Host: "https://x"}, wantErr: "bare hostname"},
		{name: "bad status", cfg: Config{Path: "/", ExpectedStatus: "2xx"}, wantErr: "not an HTTP status code"},
		{name: "http field on exec", cfg: Config{Exec: []string{"true"}, ExpectedStatus: "200"}, wantErr: "HTTP probes only"},
		{name: "empty exec command", cfg: Config{Exec: []string{" "}}, wantErr: "must not be empty"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Validate() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Validate() error = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestLimitsFromEnv(t *testing.T) {
	env := map[string]string{
		EnvMaxRedirects:    "3",
		EnvExecOutputBytes: "not-a-number",
		EnvDefaultTimeout:  "7s",
	}
	got := LimitsFromEnv(func(k string) (string, bool) { v, ok := env[k]; return v, ok })
	want := Limits{MaxRedirects: 3, ExecOutputBytes: DefaultExecOutputBytes, DefaultTimeout: 7 * time.Second, DefaultInterval: DefaultInterval}
	if got != want {
		t.Errorf("LimitsFromEnv() = %+v, want %+v", got, want)
	}
}
