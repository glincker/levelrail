package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type assetsFake struct {
	mu          sync.Mutex
	reqs        []recordedReq
	directCode  int
	provider    *httptest.Server
	providerGot []recordedReq
	providerHdr http.Header
}

func (f *assetsFake) handler(w http.ResponseWriter, r *http.Request) {
	b, _ := io.ReadAll(r.Body)
	f.mu.Lock()
	f.reqs = append(f.reqs, recordedReq{r.Method, r.URL.Path, r.URL.RawQuery, string(b)})
	f.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	const base = "/api/v1/assets/buckets"
	switch {
	case r.URL.Path == base && r.Method == http.MethodGet:
		_, _ = io.WriteString(w, `[{"id":"ab_1","name":"media","target_id":"t1","visibility":"private","serving_mode":"direct","referrer_allow":[]}]`)
	case r.URL.Path == base+"/ab_1" && r.Method == http.MethodPatch:
		if !strings.Contains(string(b), `"confirm_name":"media"`) {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `{"error":"needs confirm_name","code":"asset_confirm_required"}`)
			return
		}
		_, _ = io.WriteString(w, `{"id":"ab_1","name":"media","visibility":"public","serving_mode":"direct","referrer_allow":[]}`)
	case r.URL.Path == base+"/ab_1/objects" && r.Method == http.MethodPut:
		if f.directCode != 0 {
			w.WriteHeader(f.directCode)
			_, _ = io.WriteString(w, `{"error":"file is too large for a direct upload"}`)
			return
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(w, `{"key":"a.txt","size":5}`)
	case r.URL.Path == base+"/ab_1/uploads" && r.Method == http.MethodPost:
		_, _ = io.WriteString(w, `{"key":"a.txt","method":"PUT","url":"`+f.provider.URL+`/signed","headers":{"Content-Type":"text/plain","X-Signed":"yes"},"max_bytes":5}`)
	case r.URL.Path == base+"/ab_1/uploads/confirm":
		_, _ = io.WriteString(w, `{"key":"a.txt","size":5}`)
	case r.URL.Path == base+"/ab_1/urls":
		_, _ = io.WriteString(w, `{"key":"a.txt","visibility":"public","origin_url":"https://o.example/a.txt","cdn_url":"https://cdn.example/a.txt","notes":[]}`)
	case r.URL.Path == base+"/ab_1/usage":
		_, _ = io.WriteString(w, `{"bytes":2048,"objects":2,"at_least":false,"quota_bytes":0,"serving_mode":"direct","cost":{"estimated":true,"price_table_date":"2026-10-01","stored_bytes":2048,"egress_gb":5,"current":{"preset":"r2","total_usd":0.01},"compare":[{"preset":"aws","total_usd":0.5}],"notes":["Estimated from list prices"]}}`)
	default:
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":"not found"}`)
	}
}

func newAssetsFake(t *testing.T) (*assetsFake, *httptest.Server) {
	t.Helper()
	f := &assetsFake{}
	f.provider = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.providerGot = append(f.providerGot, recordedReq{r.Method, r.URL.Path, "", string(b)})
		f.providerHdr = r.Header.Clone()
		f.mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(f.provider.Close)
	srv := httptest.NewServer(http.HandlerFunc(f.handler))
	t.Cleanup(srv.Close)
	return f, srv
}

func runAssetsCLI(t *testing.T, srv *httptest.Server, env map[string]string, args ...string) (int, string, string) {
	t.Helper()
	var out, errb bytes.Buffer
	lookup := func(k string) (string, bool) { v, ok := env[k]; return v, ok }
	code := run("lr", append(args, "--api-url", srv.URL, "--token", "tok"), &out, &errb, lookup)
	return code, out.String(), errb.String()
}

func writeAssetFile(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "a.txt")
	if err := os.WriteFile(p, []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func (f *assetsFake) paths() []string {
	var out []string
	for _, r := range f.reqs {
		out = append(out, r.method+" "+strings.TrimPrefix(r.path, "/api/v1/assets/buckets"))
	}
	return out
}

func TestAssetsUpload_Direct(t *testing.T) {
	f, srv := newAssetsFake(t)
	code, out, errb := runAssetsCLI(t, srv, nil, "assets", "upload", "media", writeAssetFile(t), "--key", "a.txt")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	for _, want := range []string{"origin URL: https://o.example/a.txt", "CDN URL:    https://cdn.example/a.txt"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q: %s", want, out)
		}
	}
	got := strings.Join(f.paths(), ",")
	if strings.Contains(got, "uploads") || !strings.Contains(got, "PUT /ab_1/objects") {
		t.Errorf("requests = %s", got)
	}
	for _, r := range f.reqs {
		if r.method == http.MethodPut && (r.body != "hello" || r.query != "key=a.txt") {
			t.Errorf("direct PUT = %+v", r)
		}
	}
}

func TestAssetsUpload_FallsBackOn413(t *testing.T) {
	f, srv := newAssetsFake(t)
	f.directCode = http.StatusRequestEntityTooLarge
	code, out, errb := runAssetsCLI(t, srv, nil, "assets", "upload", "media", writeAssetFile(t), "--key", "a.txt")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	got := strings.Join(f.paths(), ",")
	if !strings.Contains(got, "PUT /ab_1/objects") || !strings.Contains(got, "POST /ab_1/uploads/confirm") {
		t.Errorf("requests = %s", got)
	}
	if len(f.providerGot) != 1 || f.providerGot[0].body != "hello" {
		t.Errorf("provider got %+v", f.providerGot)
	}
	if !strings.Contains(out, "origin URL") {
		t.Errorf("output = %s", out)
	}
}

func TestAssetsUpload_PresignAboveEnvThreshold(t *testing.T) {
	f, srv := newAssetsFake(t)
	env := map[string]string{envAssetsDirectMaxBytes: "3"}
	code, _, errb := runAssetsCLI(t, srv, env, "assets", "upload", "media", writeAssetFile(t), "--key", "a.txt", "--content-type", "text/plain")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	got := strings.Join(f.paths(), ",")
	if strings.Contains(got, "/objects") {
		t.Errorf("direct route used above the threshold: %s", got)
	}
	var upReq map[string]any
	for _, r := range f.reqs {
		if r.method == http.MethodPost && strings.HasSuffix(r.path, "/uploads") {
			_ = json.Unmarshal([]byte(r.body), &upReq)
		}
	}
	if upReq["size"] != float64(5) || upReq["content_type"] != "text/plain" {
		t.Errorf("presign request = %v", upReq)
	}
	if f.providerHdr.Get("X-Signed") != "yes" || f.providerHdr.Get("Content-Type") != "text/plain" {
		t.Errorf("provider headers = %v", f.providerHdr)
	}
	if f.providerHdr.Get("Authorization") != "" {
		t.Error("control plane token leaked to the provider")
	}
	if f.providerHdr.Get("Content-Length") != "5" {
		t.Errorf("content length = %q", f.providerHdr.Get("Content-Length"))
	}
}

func TestAssetsUpload_BadThresholdEnv(t *testing.T) {
	_, srv := newAssetsFake(t)
	code, _, _ := runAssetsCLI(t, srv, map[string]string{envAssetsDirectMaxBytes: "lots"}, "assets", "upload", "media", writeAssetFile(t))
	if code != exitValidation {
		t.Errorf("exit = %d, want %d", code, exitValidation)
	}
}

func TestAssetsSetPublic_Confirm(t *testing.T) {
	t.Run("with confirm-name sends it", func(t *testing.T) {
		f, srv := newAssetsFake(t)
		code, out, errb := runAssetsCLI(t, srv, nil, "assets", "buckets", "set", "media", "--public", "--confirm-name", "media")
		if code != 0 {
			t.Fatalf("exit %d: %s", code, errb)
		}
		if !strings.Contains(out, "public") {
			t.Errorf("output = %s", out)
		}
		last := f.reqs[len(f.reqs)-1]
		if last.method != http.MethodPatch || !strings.Contains(last.body, `"visibility":"public"`) || !strings.Contains(last.body, `"confirm_name":"media"`) {
			t.Errorf("patch = %+v", last)
		}
	})
	t.Run("without it and no terminal refuses and sends nothing", func(t *testing.T) {
		f, srv := newAssetsFake(t)
		code, _, errb := runAssetsCLI(t, srv, nil, "assets", "buckets", "set", "media", "--public")
		if code != exitValidation || !strings.Contains(errb, "--confirm-name media") {
			t.Errorf("exit %d stderr %q", code, errb)
		}
		for _, r := range f.reqs {
			if r.method == http.MethodPatch {
				t.Errorf("a PATCH was sent without confirmation: %+v", r)
			}
		}
	})
}

func TestAssetsUsage_SaysEstimated(t *testing.T) {
	f, srv := newAssetsFake(t)
	code, out, errb := runAssetsCLI(t, srv, nil, "assets", "usage", "media", "--egress-gb", "5")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	if !strings.Contains(out, "Estimated cost") || !strings.Contains(out, "2026-10-01") {
		t.Errorf("output = %s", out)
	}
	if last := f.reqs[len(f.reqs)-1]; last.query != "egress_gb=5" {
		t.Errorf("query = %q", last.query)
	}
}

func TestAssets_UnknownBucketAndUsage(t *testing.T) {
	_, srv := newAssetsFake(t)
	if code, _, _ := runAssetsCLI(t, srv, nil, "assets", "ls", "nope"); code != exitValidation {
		t.Errorf("unknown bucket exit = %d", code)
	}
	if code, _, _ := runAssetsCLI(t, srv, nil, "assets", "rm", "media"); code != exitValidation {
		t.Errorf("missing key exit = %d", code)
	}
	if code, _, _ := runAssetsCLI(t, srv, nil, "assets", "bogus"); code != exitUsage {
		t.Errorf("unknown command exit = %d", code)
	}
}

func TestAssetsUpload_JSONOutput(t *testing.T) {
	_, srv := newAssetsFake(t)
	code, out, errb := runAssetsCLI(t, srv, nil, "assets", "upload", "media", writeAssetFile(t), "--key", "a.txt", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errb)
	}
	var got struct {
		Asset struct{ Key string }
		URLs  struct {
			OriginURL string `json:"origin_url"`
		}
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil || got.Asset.Key != "a.txt" || got.URLs.OriginURL == "" {
		t.Errorf("json = %s (%v)", out, err)
	}
}
