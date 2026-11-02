package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const cdnGetOff = `{"app":"web","proxied":null,"global_proxied":false,"effective":false}`
const cdnGetOn = `{"app":"web","proxied":true,"global_proxied":false,"effective":true,"ssl_mode":"flexible","ssl_warning":"set it to Full (strict)"}`

func cdnServer(t *testing.T, calls *[]string, resync string) *httptest.Server {
	t.Helper()
	saved := false
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		*calls = append(*calls, r.Method+" "+r.URL.Path+" "+string(body))
		switch {
		case r.Method == http.MethodGet:
			if saved {
				_, _ = w.Write([]byte(cdnGetOn))
				return
			}
			_, _ = w.Write([]byte(cdnGetOff))
		case r.Method == http.MethodPut:
			saved = true
			_, _ = w.Write([]byte(cdnGetOn))
		case strings.HasSuffix(r.URL.Path, "/cdn/resync"):
			_, _ = w.Write([]byte(resync))
		default:
			http.NotFound(w, r)
		}
	}))
}

const resyncOK = `{"app":"web","effective":true,"dry_run":false,"updated":1,"failed":0,"records":[{"domain":"a.example.com","name":"a","type":"A","result":"updated"}]}`

func TestRun_AppsCDNGetShowsSSLWarning(t *testing.T) {
	var calls []string
	srv := cdnServer(t, &calls, resyncOK)
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"apps", "cdn", "get", "web", "--api-url", srv.URL})
	for _, want := range []string{"proxy override: inherit", "effective: off"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestRun_AppsCDNSetResyncsWhenTheSettingChanged(t *testing.T) {
	var calls []string
	srv := cdnServer(t, &calls, resyncOK)
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"apps", "cdn", "set", "web", "--proxied", "on", "--api-url", srv.URL})
	if !strings.Contains(out, "1 updated, 0 failed") {
		t.Fatalf("output:\n%s", out)
	}
	joined := strings.Join(calls, "\n")
	if !strings.Contains(joined, `PUT /api/v1/apps/web/cdn {"proxied":true}`) || !strings.Contains(joined, "POST /api/v1/apps/web/cdn/resync") {
		t.Fatalf("calls:\n%s", joined)
	}
}

func TestRun_AppsCDNSetNoResyncSkipsDNS(t *testing.T) {
	var calls []string
	srv := cdnServer(t, &calls, resyncOK)
	defer srv.Close()
	runCLIExpectOK(t, []string{"apps", "cdn", "set", "web", "--proxied", "on", "--no-resync", "--api-url", srv.URL})
	if strings.Contains(strings.Join(calls, "\n"), "resync") {
		t.Fatalf("resync called: %v", calls)
	}
}

func TestRun_AppsCDNResyncDryRunSendsFlag(t *testing.T) {
	var calls []string
	srv := cdnServer(t, &calls, `{"app":"web","effective":true,"dry_run":true,"updated":0,"failed":0,"records":[{"domain":"a.example.com","name":"a","type":"A","result":"would_update"}]}`)
	defer srv.Close()
	out, _ := runCLIExpectOK(t, []string{"apps", "cdn", "resync", "web", "--dry-run", "--api-url", srv.URL})
	if !strings.Contains(out, "would_update") || !strings.Contains(strings.Join(calls, "\n"), `{"dry_run":true}`) {
		t.Fatalf("out %s calls %v", out, calls)
	}
}
