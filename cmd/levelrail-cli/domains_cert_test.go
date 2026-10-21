package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

type certServer struct {
	mu       sync.Mutex
	ready    bool
	pollsTo  int
	polls    int
	posted   []apiclient.GetCertRequest
	query    string
	finalErr string
}

func (c *certServer) handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c.mu.Lock()
		defer c.mu.Unlock()
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/cert/preflight"):
			c.query = r.URL.RawQuery
			items := []apiclient.CertPreflightItem{{Code: "dns", State: "pass", Detail: "points here"}}
			if !c.ready {
				items = []apiclient.CertPreflightItem{{Code: "caa", State: "fail", Detail: "CAA blocks", Fix: "add a CAA record"}}
			}
			_ = json.NewEncoder(w).Encode(apiclient.CertPreflight{Domain: "shop.dev", Email: "me@shop.dev", Ready: c.ready, Items: items})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/cert"):
			var req apiclient.GetCertRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			c.posted = append(c.posted, req)
			w.WriteHeader(http.StatusAccepted)
			_ = json.NewEncoder(w).Encode(apiclient.DomainCertStatus{Domain: "shop.dev", State: "pending", Staging: req.Staging, AppliesToAll: true})
		case strings.HasSuffix(r.URL.Path, "/cert/status"):
			c.polls++
			st := apiclient.DomainCertStatus{Domain: "shop.dev", State: "pending", AppliesToAll: true}
			if c.polls >= c.pollsTo {
				now := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
				if c.finalErr != "" {
					st.State, st.Error, st.Explanation = "failed", c.finalErr, "Let's Encrypt could not reach this server on port 80"
				} else {
					st.State, st.Issuer, st.Kind, st.NotAfter, st.RenewalAt = "issued", "R11", "letsencrypt", &now, &now
				}
			}
			_ = json.NewEncoder(w).Encode(st)
		default:
			http.NotFound(w, r)
		}
	})
}

func TestRun_DomainsCert_BlockedByPreflight(t *testing.T) {
	cs := &certServer{}
	srv := httptest.NewServer(cs.handler())
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", []string{"domains", "cert", "shop.dev", "--api-url", srv.URL}, &out, &errb, envMap())
	if code != exitValidation {
		t.Fatalf("exit = %d, want %d (%s)", code, exitValidation, errb.String())
	}
	for _, want := range []string{"fail", "CAA blocks", "next: add a CAA record", "not ready"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("stdout missing %q: %q", want, out.String())
		}
	}
	if len(cs.posted) != 0 {
		t.Fatal("a failed preflight must not request a certificate")
	}
}

func TestRun_DomainsCert_StagingWaitIssued(t *testing.T) {
	old := certWaitPoll
	certWaitPoll = time.Millisecond
	defer func() { certWaitPoll = old }()
	cs := &certServer{ready: true, pollsTo: 2}
	srv := httptest.NewServer(cs.handler())
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"domains", "cert", "shop.dev", "--staging", "--wait", "--email", "me@shop.dev", "--api-url", srv.URL})
	if len(cs.posted) != 1 || !cs.posted[0].Staging || cs.posted[0].Email != "me@shop.dev" {
		t.Fatalf("posted = %+v", cs.posted)
	}
	if !strings.Contains(cs.query, "staging=true") {
		t.Fatalf("preflight query = %q", cs.query)
	}
	for _, want := range []string{"certificate: issued", "R11 (letsencrypt)", "renews:", "every domain"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout missing %q: %q", want, stdout)
		}
	}
}

func TestRun_DomainsCert_FailedWaitExitsNonZero(t *testing.T) {
	old := certWaitPoll
	certWaitPoll = time.Millisecond
	defer func() { certWaitPoll = old }()
	cs := &certServer{ready: true, pollsTo: 1, finalErr: "Timeout during connect"}
	srv := httptest.NewServer(cs.handler())
	defer srv.Close()
	var out, errb bytes.Buffer
	code := run("levelrail-cli-test", []string{"domains", "cert", "shop.dev", "--wait", "--api-url", srv.URL}, &out, &errb, envMap())
	if code != exitAPIError {
		t.Fatalf("exit = %d, want %d", code, exitAPIError)
	}
	if !strings.Contains(out.String(), "could not reach this server on port 80") || !strings.Contains(out.String(), "CA said:  Timeout during connect") {
		t.Fatalf("stdout = %q", out.String())
	}
}

func TestRun_DomainsCert_PreflightOnlyJSON(t *testing.T) {
	cs := &certServer{ready: true}
	srv := httptest.NewServer(cs.handler())
	defer srv.Close()
	stdout, _ := runCLIExpectOK(t, []string{"domains", "cert", "shop.dev", "--preflight", "--json", "--api-url", srv.URL})
	var got certResult
	if err := json.Unmarshal([]byte(stdout), &got); err != nil || !got.Preflight.Ready || got.Status != nil {
		t.Fatalf("json = %q err %v", stdout, err)
	}
	if len(cs.posted) != 0 {
		t.Fatal("--preflight must not request a certificate")
	}
}

func TestRun_DomainsCert_StagingRefusedShowsReasonAndNextStep(t *testing.T) {
	msg := "Staging would replace the working Let's Encrypt certificate for shop.dev. Next step: request the real certificate without staging."
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusConflict)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": msg, "code": "replaces_certificate"})
			return
		}
		_ = json.NewEncoder(w).Encode(apiclient.CertPreflight{Domain: "shop.dev", Ready: true})
	}))
	defer srv.Close()
	var out, errb bytes.Buffer
	if code := run("levelrail-cli-test", []string{"domains", "cert", "shop.dev", "--staging", "--api-url", srv.URL}, &out, &errb, envMap()); code != exitAPIError {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(errb.String(), "would replace the working") || !strings.Contains(errb.String(), "Next step") {
		t.Fatalf("stderr = %q", errb.String())
	}

	blocked := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(apiclient.CertPreflight{Domain: "shop.dev", Items: []apiclient.CertPreflightItem{{Code: "replaces_certificate", State: "fail", Detail: msg, Fix: "Request the real certificate without staging."}}})
	}))
	defer blocked.Close()
	out.Reset()
	errb.Reset()
	if code := run("levelrail-cli-test", []string{"domains", "cert", "shop.dev", "--staging", "--api-url", blocked.URL}, &out, &errb, envMap()); code != exitValidation {
		t.Fatalf("exit = %d", code)
	}
	if !strings.Contains(out.String(), "next: Request the real certificate without staging.") {
		t.Fatalf("stdout = %q", out.String())
	}
}
