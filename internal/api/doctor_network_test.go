package api

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeDoctorDoer is a hand-written doctorHTTPDoer fake, the same
// "fake the single-method seam, no real request" pattern
// fakeDoctorOfflineDoer (doctor_test.go) already establishes.
type fakeDoctorDoer struct {
	respond func(req *http.Request) (*http.Response, error)
}

func (f fakeDoctorDoer) Do(req *http.Request) (*http.Response, error) {
	return f.respond(req)
}

func fakeDoctorResponse(status int, body string, headers map[string]string) *http.Response {
	resp := &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
	for k, v := range headers {
		resp.Header.Set(k, v)
	}
	return resp
}

func TestDoctorCheckPublicIP(t *testing.T) {
	tests := []struct {
		name       string
		doer       doctorHTTPDoer
		wantStatus string
		wantIP     string
	}{
		{
			name:       "valid ip",
			doer:       fakeDoctorDoer{respond: func(*http.Request) (*http.Response, error) { return fakeDoctorResponse(200, "203.0.113.5", nil), nil }},
			wantStatus: doctorStatusOK,
			wantIP:     "203.0.113.5",
		},
		{
			name:       "invalid body",
			doer:       fakeDoctorDoer{respond: func(*http.Request) (*http.Response, error) { return fakeDoctorResponse(200, "not-an-ip", nil), nil }},
			wantStatus: doctorStatusUnknown,
		},
		{
			name:       "non-200 status",
			doer:       fakeDoctorDoer{respond: func(*http.Request) (*http.Response, error) { return fakeDoctorResponse(503, "", nil), nil }},
			wantStatus: doctorStatusUnknown,
		},
		{
			name:       "network error",
			doer:       fakeDoctorDoer{respond: func(*http.Request) (*http.Response, error) { return nil, errors.New("dial tcp: no route") }},
			wantStatus: doctorStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, _ := newDoctorTestRouter(t)
			rt.doctorHTTPClient = tt.doer
			check, ip := rt.doctorCheckPublicIP(context.Background())
			if check.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (message %q)", check.Status, tt.wantStatus, check.Message)
			}
			if check.Status == doctorStatusFail {
				t.Error("public_ip must never report fail: an offline host is a supported deployment")
			}
			if ip != tt.wantIP {
				t.Errorf("ip = %q, want %q", ip, tt.wantIP)
			}
		})
	}
}

func TestDoctorCheckExternalReachability(t *testing.T) {
	t.Run("no public ip detected", func(t *testing.T) {
		rt, _ := newDoctorTestRouter(t)
		c := rt.doctorCheckExternalReachability(context.Background(), "", 443)
		if c.Status != doctorStatusUnknown {
			t.Errorf("status = %q, want %q", c.Status, doctorStatusUnknown)
		}
		if c.DocsPath == "" {
			t.Error("DocsPath = \"\", want a troubleshooting link")
		}
	})

	t.Run("dial succeeds", func(t *testing.T) {
		serverConn, clientConn := net.Pipe()
		defer func() { _ = serverConn.Close() }()
		rt, _ := newDoctorTestRouter(t)
		rt.doctorDialContext = func(context.Context, string, string) (net.Conn, error) { return clientConn, nil }

		c := rt.doctorCheckExternalReachability(context.Background(), "203.0.113.5", 443)
		if c.Status != doctorStatusOK {
			t.Errorf("status = %q, want %q", c.Status, doctorStatusOK)
		}
		if c.Code != "external_reachability_443" {
			t.Errorf("code = %q, want %q", c.Code, "external_reachability_443")
		}
	})

	t.Run("dial fails degrades to unknown, not fail (hairpin caveat)", func(t *testing.T) {
		rt, _ := newDoctorTestRouter(t)
		c := rt.doctorCheckExternalReachability(context.Background(), "203.0.113.5", 80)
		if c.Status != doctorStatusUnknown {
			t.Errorf("status = %q, want %q", c.Status, doctorStatusUnknown)
		}
		if c.Fix == "" {
			t.Error("Fix = \"\", want a suggested verification step")
		}
	})
}

func TestDoctorCheckACMEReachability(t *testing.T) {
	t.Run("reachable", func(t *testing.T) {
		rt, _ := newDoctorTestRouter(t)
		rt.doctorHTTPClient = fakeDoctorDoer{respond: func(*http.Request) (*http.Response, error) { return fakeDoctorResponse(200, "{}", nil), nil }}
		c := rt.doctorCheckACMEReachability(context.Background())
		if c.Status != doctorStatusOK {
			t.Errorf("status = %q, want %q", c.Status, doctorStatusOK)
		}
	})

	t.Run("unreachable, ACME not enabled: warn not fail", func(t *testing.T) {
		rt, _ := newDoctorTestRouter(t)
		c := rt.doctorCheckACMEReachability(context.Background())
		if c.Status != doctorStatusWarn {
			t.Errorf("status = %q, want %q", c.Status, doctorStatusWarn)
		}
	})

	t.Run("unreachable, ACME enabled: fail", func(t *testing.T) {
		db := openTestDB(t)
		rt := withOfflineDoctorNetwork(NewRouter(discardLogger(), testBrand(), db))
		if err := db.UpdateIngressSettings(context.Background(), store.IngressSettings{ACMEEnabled: true, ACMEEmail: "ops@example.com"}); err != nil {
			t.Fatalf("UpdateIngressSettings() error = %v", err)
		}
		c := rt.doctorCheckACMEReachability(context.Background())
		if c.Status != doctorStatusFail {
			t.Errorf("status = %q, want %q (ACME enabled and unreachable is a real problem)", c.Status, doctorStatusFail)
		}
	})

	t.Run("uses the configured directory URL override", func(t *testing.T) {
		db := openTestDB(t)
		rt := NewRouter(discardLogger(), testBrand(), db)
		if err := db.UpdateIngressSettings(context.Background(), store.IngressSettings{ACMEDirectoryURL: "https://acme-staging-v02.api.letsencrypt.org/directory"}); err != nil {
			t.Fatalf("UpdateIngressSettings() error = %v", err)
		}
		var gotURL string
		rt.doctorHTTPClient = fakeDoctorDoer{respond: func(req *http.Request) (*http.Response, error) {
			gotURL = req.URL.String()
			return fakeDoctorResponse(200, "{}", nil), nil
		}}
		rt.doctorCheckACMEReachability(context.Background())
		if gotURL != "https://acme-staging-v02.api.letsencrypt.org/directory" {
			t.Errorf("requested URL = %q, want the configured override", gotURL)
		}
	})
}

func TestDoctorCheckClockSkew(t *testing.T) {
	tests := []struct {
		name       string
		doer       doctorHTTPDoer
		warnAge    time.Duration
		wantStatus string
		wantFixSet bool
	}{
		{
			name: "clock in sync",
			doer: fakeDoctorDoer{respond: func(*http.Request) (*http.Response, error) {
				return fakeDoctorResponse(200, "", map[string]string{"Date": time.Now().UTC().Format(http.TimeFormat)}), nil
			}},
			wantStatus: doctorStatusOK,
		},
		{
			name: "clock badly skewed",
			doer: fakeDoctorDoer{respond: func(*http.Request) (*http.Response, error) {
				return fakeDoctorResponse(200, "", map[string]string{"Date": time.Now().Add(-time.Hour).UTC().Format(http.TimeFormat)}), nil
			}},
			warnAge:    time.Minute,
			wantStatus: doctorStatusWarn,
			wantFixSet: true,
		},
		{
			name:       "no Date header",
			doer:       fakeDoctorDoer{respond: func(*http.Request) (*http.Response, error) { return fakeDoctorResponse(200, "", nil), nil }},
			wantStatus: doctorStatusUnknown,
		},
		{
			name:       "network error",
			doer:       fakeDoctorDoer{respond: func(*http.Request) (*http.Response, error) { return nil, errors.New("offline") }},
			wantStatus: doctorStatusUnknown,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, _ := newDoctorTestRouter(t)
			rt.doctorHTTPClient = tt.doer
			rt.doctorClockSkewWarnAge = tt.warnAge
			c := rt.doctorCheckClockSkew(context.Background())
			if c.Status != tt.wantStatus {
				t.Errorf("status = %q, want %q (message %q)", c.Status, tt.wantStatus, c.Message)
			}
			if tt.wantFixSet && c.Fix == "" {
				t.Error("Fix = \"\", want an NTP suggestion")
			}
		})
	}
}

func TestDoctorRunNetworkChecks_ReturnsAllFiveCodes(t *testing.T) {
	rt, _ := newDoctorTestRouter(t)
	checks := rt.doctorRunNetworkChecks(context.Background(), 80, 443)
	wantCodes := []string{"public_ip", "external_reachability_80", "external_reachability_443", "acme_reachability", "clock_skew"}
	if len(checks) != len(wantCodes) {
		t.Fatalf("len(checks) = %d, want %d", len(checks), len(wantCodes))
	}
	for _, code := range wantCodes {
		doctorCheckByCode(t, checks, code)
	}
}
