package secretprovider

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const guardValue = "guard-secret-value-8431" //nolint:gosec // test fixture, not a credential

// echoBody is what a hostile or broken provider might send back: both the token and a value.
var echoBody = `{"error":"bad token ` + fakeToken + ` value ` + guardValue + `"}`

func guardCases() map[string]func(w http.ResponseWriter, r *http.Request) {
	return map[string]func(w http.ResponseWriter, r *http.Request){
		"401": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = w.Write([]byte(echoBody))
		},
		"500": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(echoBody))
		},
		"oversize": func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"pad":"` + strings.Repeat("x", 1<<17) + guardValue + `"}`))
		},
		"redirect": func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "http://169.254.169.254/latest?t="+fakeToken+"&v="+guardValue, http.StatusFound)
		},
		"slow": func(w http.ResponseWriter, r *http.Request) {
			select {
			case <-time.After(3 * time.Second):
			case <-r.Context().Done():
			}
			_, _ = w.Write([]byte(echoBody))
		},
	}
}

type guardProvider struct {
	name string
	cfg  func(url string) Config
	ref  string
}

func guardProviders() []guardProvider {
	return []guardProvider{
		{"doppler", func(u string) Config {
			return Config{Name: "dp", Type: TypeDoppler, Project: "web", Environment: "prd", BaseURL: u}
		}, "doppler://dp/DB_URL"},
		{"infisical", func(u string) Config { return selfHosted(u) }, "infisical://inf/API_KEY"},
		{"onepassword", func(u string) Config { return Config{Name: "op1", Type: TypeOnePassword, BaseURL: u, Project: "Prod"} }, "op://op1/Prod/Stripe/api_key"},
	}
}

func TestProviderGuardNoLeaks(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	for caseName, h := range guardCases() {
		for _, p := range guardProviders() {
			t.Run(p.name+"/"+caseName, func(t *testing.T) {
				srv := httptest.NewServer(http.HandlerFunc(h))
				defer srv.Close()
				ref, _, err := ParseRef(p.ref)
				if err != nil {
					t.Fatal(err)
				}
				tn := Tunables{Timeout: 300 * time.Millisecond, MaxResponseBytes: 1 << 12}
				got, err := NewResolver().Fetch(context.Background(), p.cfg(srv.URL), fakeToken, ref, tn)
				if err == nil {
					t.Fatalf("%s must fail closed", caseName)
				}
				if got != "" {
					t.Fatalf("a failed read returned %q", got)
				}
				for _, s := range []string{err.Error(), fmt.Sprintf("%+v", err)} {
					if strings.Contains(s, fakeToken) || strings.Contains(s, guardValue) {
						t.Fatalf("error leaks a secret: %s", s)
					}
				}
			})
		}
	}
	if strings.Contains(logs.String(), fakeToken) || strings.Contains(logs.String(), guardValue) {
		t.Fatal("a log line leaks a secret")
	}
}

func TestProviderGuardRateLimit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		_, _ = w.Write([]byte(`{"secret":{"secretKey":"API_KEY","secretValue":"` + guardValue + `"}}`))
	}))
	defer srv.Close()
	ref, _, _ := ParseRef("infisical://inf/API_KEY")
	r := NewResolver()
	now := time.Now()
	r.now = func() time.Time { return now }
	tn := Tunables{Timeout: time.Second, MaxResponseBytes: 1 << 16, MaxRPS: 2}
	for i := 0; i < 2; i++ {
		if _, err := r.Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tn); err != nil {
			t.Fatalf("read %d under the limit: %v", i, err)
		}
	}
	_, err := r.Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tn)
	if err == nil || !strings.Contains(err.Error(), ErrRateLimited.Error()) {
		t.Fatalf("third read must fail closed with a rate limit error, got %v", err)
	}
	if strings.Contains(err.Error(), fakeToken) || calls != 2 {
		t.Fatalf("limited read reached the provider or leaked: calls=%d err=%v", calls, err)
	}
	now = now.Add(1100 * time.Millisecond)
	if _, err := r.Fetch(context.Background(), selfHosted(srv.URL), fakeToken, ref, tn); err != nil {
		t.Fatalf("next window should allow a read: %v", err)
	}
}
