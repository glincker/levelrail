package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestIsActiveContentType(t *testing.T) {
	tests := map[string]bool{
		"text/html; charset=utf-8": true, "image/svg+xml": true, "application/javascript": true,
		"image/png": false, "text/plain": false, "application/octet-stream": false,
	}
	for ct, want := range tests {
		if got := isActiveContentType(ct); got != want {
			t.Errorf("isActiveContentType(%q) = %v, want %v", ct, got, want)
		}
	}
}

func makePublicBucket(t *testing.T, e *storageEnv, targetID string) storageBucketResource {
	t.Helper()
	rec := e.do(t, http.MethodPost, "/api/v1/storage/buckets", `{"name":"pub","target_id":"`+targetID+`","public":true}`)
	var b storageBucketResource
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", rec.Code, err)
	}
	return b
}

func publicBucketGet(e *storageEnv, b storageBucketResource, key string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	e.rt.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, publicBucketPathPrefix+b.ID+"/"+key, nil))
	return rec
}

func TestPublicBucketRouteGuards(t *testing.T) {
	e, d := newBucketEnv(t)
	b := makePublicBucket(t, e, d.ID)
	e.srv.Put(b.Prefix+"page.html", []byte("<script>x</script>"), time.Now())
	e.srv.Put(b.Prefix+"big.bin", []byte("0123456789"), time.Now())

	rec := publicBucketGet(e, b, "page.html")
	if rec.Code != http.StatusOK || !strings.HasPrefix(rec.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("html on public route = %d %v, want forced download", rec.Code, rec.Header())
	}

	t.Setenv(envPublicBucketMaxBytes, "5")
	if rec := publicBucketGet(e, b, "big.bin"); rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize public object = %d, want 413", rec.Code)
	}
}

func TestPublicBucketRouteRateLimit(t *testing.T) {
	e, d := newBucketEnv(t)
	b := makePublicBucket(t, e, d.ID)
	e.rt.publicBucketLimiterOnce.Do(func() { e.rt.publicBucketLimiter = newAPIRateLimiter(2) })
	for i := 0; i < 2; i++ {
		if rec := publicBucketGet(e, b, "missing.txt"); rec.Code != http.StatusNotFound {
			t.Fatalf("request %d = %d, want 404", i, rec.Code)
		}
	}
	rec := publicBucketGet(e, b, "missing.txt")
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") == "" {
		t.Fatalf("over budget = %d %v, want 429 with Retry-After", rec.Code, rec.Header())
	}
}

func TestPresignPutQuotaError(t *testing.T) {
	e, d := newBucketEnv(t)
	rec := e.do(t, http.MethodPost, "/api/v1/storage/buckets", `{"name":"small","target_id":"`+d.ID+`","quota_bytes":4}`)
	var b storageBucketResource
	if err := json.NewDecoder(rec.Body).Decode(&b); err != nil || rec.Code != http.StatusCreated {
		t.Fatalf("create = %d %v", rec.Code, err)
	}
	path := "/api/v1/storage/buckets/" + b.ID + "/presign"
	if rec := e.do(t, http.MethodPost, path, `{"key":"a.txt","method":"PUT"}`); rec.Code != http.StatusOK {
		t.Fatalf("presign PUT under quota = %d", rec.Code)
	}
	e.srv.Put(b.Prefix+"full.txt", []byte("12345678"), time.Now())
	rec = e.do(t, http.MethodPost, path, `{"key":"a.txt","method":"PUT"}`)
	if rec.Code != http.StatusRequestEntityTooLarge || !strings.Contains(rec.Body.String(), "quota of 4 bytes is full") {
		t.Fatalf("presign PUT over quota = %d %s", rec.Code, rec.Body.String())
	}
	if rec := e.do(t, http.MethodPost, path, `{"key":"a.txt","method":"GET"}`); rec.Code != http.StatusOK {
		t.Fatalf("presign GET over quota = %d, want 200", rec.Code)
	}
}

func TestBoundEnvListsMaskedAndBucketBindRestarts(t *testing.T) {
	e, d := newBucketEnv(t)
	rec := e.do(t, http.MethodPost, "/api/v1/storage/buckets", `{"name":"uploads","target_id":"`+d.ID+`","app_name":"web"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("create = %d", rec.Code)
	}
	rec = e.do(t, http.MethodGet, "/api/v1/apps/web/env/bound", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("bound = %d: %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var rows []boundEnvEntry
	if err := json.Unmarshal([]byte(body), &rows); err != nil || len(rows) != 6 {
		t.Fatalf("rows = %d %v", len(rows), err)
	}
	if strings.Contains(body, "topsecretvalue") || strings.Contains(body, "AKIDSECRET") {
		t.Fatal("bound env leaked a credential")
	}
	for _, r := range rows {
		if r.Key == "S3_SECRET_ACCESS_KEY" && !r.Masked {
			t.Errorf("secret key row not masked: %+v", r)
		}
	}

	svc, _ := e.db.GetDesiredService(t.Context(), "web")
	before := svc.RestartNonce
	other := e.do(t, http.MethodPost, "/api/v1/storage/buckets", `{"name":"media","target_id":"`+d.ID+`"}`)
	var media storageBucketResource
	_ = json.NewDecoder(other.Body).Decode(&media)
	rec = e.do(t, http.MethodPut, "/api/v1/apps/web/buckets/"+media.ID, `{"env_prefix":"MEDIA"}`)
	var bound storageBucketResource
	_ = json.NewDecoder(rec.Body).Decode(&bound)
	svc, _ = e.db.GetDesiredService(t.Context(), "web")
	if !bound.AppRestarted || svc.RestartNonce == before {
		t.Fatalf("bind did not restart the app: restarted=%v nonce %q -> %q", bound.AppRestarted, before, svc.RestartNonce)
	}
}
