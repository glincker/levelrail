package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type decodeProbe struct {
	Name string `json:"name"`
}

func runDecode(body string, limit int64, opts ...decodeOpt) (decodeProbe, bool, *httptest.ResponseRecorder) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	v, ok := decodeJSON[decodeProbe](w, r, limit, opts...)
	return v, ok, w
}

func TestDecodeJSON(t *testing.T) {
	if v, ok, _ := runDecode(`{"name":"a"}`, 64); !ok || v.Name != "a" {
		t.Fatalf("valid body: %v %v", v, ok)
	}
	if _, ok, w := runDecode(`{"name":"`+strings.Repeat("x", 100)+`"}`, 16); ok || w.Code != http.StatusBadRequest || !strings.Contains(w.Body.String(), "invalid request body") {
		t.Fatalf("limit: %v %d %s", ok, w.Code, w.Body.String())
	}
	if _, ok, w := runDecode(`{bad`, 64, decodeMessage("invalid JSON body")); ok || !strings.Contains(w.Body.String(), "invalid JSON body") {
		t.Fatalf("bad json: %v %s", ok, w.Body.String())
	}
	if _, ok, _ := runDecode(`{"name":"a","extra":1}`, 64); !ok {
		t.Fatal("unknown fields must pass by default")
	}
	if _, ok, w := runDecode(`{"name":"a","extra":1}`, 64, decodeStrict()); ok || w.Code != http.StatusBadRequest {
		t.Fatalf("strict: %v %d", ok, w.Code)
	}
}
