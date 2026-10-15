package secrets

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func fakeTransit(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Vault-Token") != "tok" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"errors":["permission denied"]}`))
			return
		}
		var in map[string]string
		_ = json.NewDecoder(r.Body).Decode(&in)
		switch {
		case strings.HasSuffix(r.URL.Path, "/encrypt/master"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"ciphertext": "vault:v1:" + in["plaintext"]}})
		case strings.HasSuffix(r.URL.Path, "/decrypt/master"):
			pt := strings.TrimPrefix(in["ciphertext"], "vault:v1:")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"plaintext": pt}})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestTransitRoundTripAndMasterKey(t *testing.T) {
	srv := fakeTransit(t)
	defer srv.Close()
	w, err := NewVaultTransit(srv.URL, "tok", "master", "", "")
	if err != nil {
		t.Fatalf("new transit: %v", err)
	}
	mk, err := GenerateMasterKey()
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	blob, err := WrapMasterKey(context.Background(), mk.String(), w)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	if !IsWrappedMasterKey(blob) || strings.Contains(blob, mk.String()) {
		t.Fatalf("blob must be marked and must not contain the key: %q", blob)
	}
	plain, err := UnwrapMasterKey(context.Background(), blob, w)
	if err != nil || plain != mk.String() {
		t.Fatalf("unwrap = %q err=%v", plain, err)
	}
	if _, err := LoadMasterKey(plain); err != nil {
		t.Fatalf("unwrapped key must load: %v", err)
	}
}

func TestUnwrapMasterKeyRefusals(t *testing.T) {
	srv := fakeTransit(t)
	defer srv.Close()
	good, _ := NewVaultTransit(srv.URL, "tok", "master", "", "")
	bad, _ := NewVaultTransit(srv.URL, "wrong", "master", "", "")
	blob, err := WrapMasterKey(context.Background(), "AGE-SECRET-KEY-PQ-1X", good)
	if err != nil {
		t.Fatalf("wrap: %v", err)
	}
	tests := []struct {
		name string
		blob string
		w    KeyWrapper
		want error
	}{
		{name: "no wrapper configured", blob: blob, w: nil, want: ErrWrapperRequired},
		{name: "backend mismatch", blob: WrappedPrefix + "other:abc", w: good},
		{name: "malformed", blob: WrappedPrefix + "vault-transit", w: good},
		{name: "vault denies", blob: blob, w: bad},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := UnwrapMasterKey(context.Background(), tt.blob, tt.w)
			if err == nil {
				t.Fatal("want an error, got nil")
			}
			if tt.want != nil && !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func TestNewVaultTransitRejectsInsecureAddr(t *testing.T) {
	if _, err := NewVaultTransit("http://vault.example.com:8200", "t", "k", "", ""); err == nil {
		t.Fatal("plain http to a remote host must be refused")
	}
	if _, err := NewVaultTransit("http://127.0.0.1:8200", "t", "k", "", ""); err != nil {
		t.Fatalf("loopback http must be allowed: %v", err)
	}
}
