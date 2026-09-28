package build

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
)

func testS3() *S3Cache {
	return &S3Cache{
		Region: "auto", Bucket: "bkt", Prefix: "build-cache/web", Endpoint: "https://x.r2.cloudflarestorage.com",
		PathStyle: true, AccessKeyID: "AKIDEXAMPLE", SecretAccessKey: "s3cr3t-value", Mode: CacheModeMin,
	}
}

func TestS3CacheEntries(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*S3Cache)
		wantErr  bool
		wantMode string
		wantKV   map[string]string
	}{
		{name: "min", wantMode: "min", wantKV: map[string]string{"bucket": "bkt", "prefix": "build-cache/web/", "use_path_style": "true", "endpoint_url": "https://x.r2.cloudflarestorage.com", "region": "auto"}},
		{name: "default mode is max", mutate: func(s *S3Cache) { s.Mode = "" }, wantMode: "max"},
		{name: "no endpoint or region omitted", mutate: func(s *S3Cache) { s.Endpoint = ""; s.Region = ""; s.PathStyle = false }, wantMode: "min", wantKV: map[string]string{"use_path_style": "false"}},
		{name: "missing bucket", mutate: func(s *S3Cache) { s.Bucket = "" }, wantErr: true},
		{name: "missing secret", mutate: func(s *S3Cache) { s.SecretAccessKey = "" }, wantErr: true},
		{name: "bad mode", mutate: func(s *S3Cache) { s.Mode = "all" }, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := testS3()
			if tt.mutate != nil {
				tt.mutate(s)
			}
			imports, exports, err := CacheConfig{S3: s}.entries()
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if len(imports) != 1 || len(exports) != 1 || imports[0].Type != "s3" || exports[0].Type != "s3" {
				t.Fatalf("entries = %+v %+v", imports, exports)
			}
			if _, ok := imports[0].Attrs["mode"]; ok {
				t.Error("import entry must not carry a mode")
			}
			if exports[0].Attrs["mode"] != tt.wantMode {
				t.Errorf("mode = %q, want %q", exports[0].Attrs["mode"], tt.wantMode)
			}
			for k, v := range tt.wantKV {
				if imports[0].Attrs[k] != v {
					t.Errorf("attr %s = %q, want %q", k, imports[0].Attrs[k], v)
				}
			}
			if tt.mutate != nil && s.Endpoint == "" {
				if _, ok := exports[0].Attrs["endpoint_url"]; ok {
					t.Error("endpoint_url should be omitted")
				}
			}
		})
	}
}

func TestCacheConfigStringHasNoCredentials(t *testing.T) {
	s := CacheConfig{S3: testS3()}.String()
	if strings.Contains(s, "s3cr3t") || strings.Contains(s, "AKID") {
		t.Fatalf("String leaked credentials: %q", s)
	}
}

func TestSolveFailOpen(t *testing.T) {
	cacheErr := errors.New("failed to solve: error writing cache: AccessDenied")
	buildErr := errors.New("failed to solve: process did not complete successfully: exit code 1")
	tests := []struct {
		name         string
		firstErr     error
		firstWrites  bool
		wantErr      bool
		wantCalls    int
		wantWarn     bool
		secondFailed error
	}{
		{name: "success no warning", wantCalls: 1},
		{name: "cache error before output retries without s3", firstErr: cacheErr, wantCalls: 2, wantWarn: true},
		{name: "cache error after output fails instead of returning an unverified image", firstErr: cacheErr, firstWrites: true, wantErr: true, wantCalls: 1, wantWarn: true},
		{name: "plain build error is not retried", firstErr: buildErr, wantErr: true, wantCalls: 1},
		{name: "retry failure surfaces", firstErr: cacheErr, secondFailed: buildErr, wantErr: true, wantCalls: 2, wantWarn: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			calls := 0
			var warns []string
			progress := func(ev ProgressEvent) {
				if ev.CacheWarning != "" {
					warns = append(warns, ev.CacheWarning)
				}
			}
			cache := CacheConfig{S3: testS3()}
			res, err := solveFailOpen(context.Background(), cache, &out, progress, func(c CacheConfig, w io.Writer) (*Result, error) {
				calls++
				if calls == 1 {
					if tt.firstWrites {
						_, _ = w.Write([]byte("tar"))
					}
					return &Result{}, tt.firstErr
				}
				if c.S3 != nil {
					t.Error("retry must drop s3")
				}
				return &Result{}, tt.secondFailed
			})
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && res != nil && res.Tag == "" && tt.firstWrites {
				t.Errorf("returned success with an empty image tag")
			}
			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}
			if (len(warns) > 0) != tt.wantWarn {
				t.Errorf("warns = %v, wantWarn %v", warns, tt.wantWarn)
			}
			for _, w := range warns {
				if strings.Contains(w, "s3cr3t") {
					t.Errorf("warning leaked secret: %q", w)
				}
			}
		})
	}
}

func TestRedact(t *testing.T) {
	got := testS3().redact("denied for AKIDEXAMPLE with s3cr3t-value " + strings.Repeat("x", 400))
	if strings.Contains(got, "AKIDEXAMPLE") || strings.Contains(got, "s3cr3t-value") {
		t.Fatalf("not redacted: %q", got)
	}
	if len(got) > maxCacheWarningLen+3 {
		t.Fatalf("not truncated: %d", len(got))
	}
}
