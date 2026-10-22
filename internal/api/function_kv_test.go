package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func kvTestFunction(t *testing.T, db *store.DB, name string) string {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, store.DesiredService{Name: name, Image: "nginx:1", Port: 80}); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleepIdle(ctx, name, 10, time.Now()); err != nil {
		t.Fatal(err)
	}
	if err := db.SetAppSleepHold(ctx, name, true, time.Now()); err != nil {
		t.Fatal(err)
	}
	tok, err := db.RotateFunctionKVToken(ctx, name, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

func kvCall(_ *Router, h http.HandlerFunc, method, target, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, target, strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.SetPathValue("name", "fn-a")
	rec := httptest.NewRecorder()
	h(rec, req)
	return rec
}

func TestFunctionKVTokenIsolationOverHTTP(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	tokA := kvTestFunction(t, db, "fn-a")
	tokB := kvTestFunction(t, db, "fn-b")
	entry := "/api/v1/function-kv/entry?key=shared"

	if rec := kvCall(rt, rt.handleFunctionKVEntry, http.MethodPut, entry, tokA, "from-a"); rec.Code != http.StatusNoContent {
		t.Fatalf("put as A = %d %s", rec.Code, rec.Body)
	}
	if rec := kvCall(rt, rt.handleFunctionKVEntry, http.MethodGet, entry, tokB, ""); rec.Code != http.StatusNotFound {
		t.Fatalf("B reading A's key = %d, want 404", rec.Code)
	}
	if rec := kvCall(rt, rt.handleFunctionKVEntry, http.MethodGet, entry, tokA, ""); rec.Code != http.StatusOK || rec.Body.String() != "from-a" {
		t.Fatalf("A reading own key = %d %q", rec.Code, rec.Body)
	}
	if rec := kvCall(rt, rt.handleFunctionKVEntry, http.MethodDelete, entry, tokB, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("B delete = %d", rec.Code)
	}
	if rec := kvCall(rt, rt.handleFunctionKVEntry, http.MethodGet, entry, tokA, ""); rec.Code != http.StatusOK {
		t.Fatal("B deleted A's key")
	}
	rec := kvCall(rt, rt.handleFunctionKVKeys, http.MethodGet, "/api/v1/function-kv/keys", tokB, "")
	if rec.Code != http.StatusOK || strings.Contains(rec.Body.String(), "shared") {
		t.Fatalf("B lists A's keys: %d %s", rec.Code, rec.Body)
	}
	for _, bad := range []string{"", "lrkv_nope", "Bearer"} {
		if rec := kvCall(rt, rt.handleFunctionKVEntry, http.MethodGet, entry, bad, ""); rec.Code != http.StatusUnauthorized {
			t.Errorf("token %q = %d, want 401", bad, rec.Code)
		}
	}
	if _, err := db.RotateFunctionKVToken(context.Background(), "fn-a", time.Now()); err != nil {
		t.Fatal(err)
	}
	if rec := kvCall(rt, rt.handleFunctionKVEntry, http.MethodGet, entry, tokA, ""); rec.Code != http.StatusUnauthorized {
		t.Fatalf("rotated token = %d, want 401", rec.Code)
	}
}

func TestFunctionKVLimitsOverHTTP(t *testing.T) {
	t.Setenv("APP_FUNCTION_KV_MAX_VALUE_BYTES", "8")
	t.Setenv("APP_FUNCTION_KV_MAX_KEYS", "1")
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	tok := kvTestFunction(t, db, "fn-a")
	put := func(key, body string) int {
		return kvCall(rt, rt.handleFunctionKVEntry, http.MethodPut, "/api/v1/function-kv/entry?key="+key, tok, body).Code
	}
	if code := put("a", "123456789"); code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize value = %d", code)
	}
	if code := put("a", "1234"); code != http.StatusNoContent {
		t.Fatalf("put = %d", code)
	}
	if code := put("b", "1"); code != http.StatusInsufficientStorage {
		t.Fatalf("second key = %d, want 507", code)
	}
}

func TestFunctionKVDashboardTruncatesValues(t *testing.T) {
	rt, db, _ := newSafetyRouter(t, stubResolver{digest: newDigest})
	kvTestFunction(t, db, "fn-a")
	long := strings.Repeat("x", 500)
	body, _ := json.Marshal(kvPutRequest{Key: "big", Value: long, TTLSeconds: 60})
	if rec := kvCall(rt, rt.handlePutFunctionKVValue, http.MethodPut, "/api/v1/functions/fn-a/kv/value", "", string(body)); rec.Code != http.StatusNoContent {
		t.Fatalf("put = %d %s", rec.Code, rec.Body)
	}
	rec := kvCall(rt, rt.handleListFunctionKV, http.MethodGet, "/api/v1/functions/fn-a/kv", "", "")
	var list kvListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if !list.Enabled || len(list.Keys) != 1 || list.Keys[0].Size != 500 || len(list.Keys[0].Preview) >= 500 || list.Keys[0].ExpiresAt == nil {
		t.Fatalf("list = %+v", list)
	}
	full := kvCall(rt, rt.handleGetFunctionKVValue, http.MethodGet, "/api/v1/functions/fn-a/kv/value?key=big", "", "")
	if !strings.Contains(full.Body.String(), long) {
		t.Fatal("single-value read must return the full value")
	}
	if rec := kvCall(rt, rt.handleDeleteFunctionKVValue, http.MethodDelete, "/api/v1/functions/fn-a/kv/value?key=big", "", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete = %d", rec.Code)
	}
	if rec := kvCall(rt, rt.handleDeleteFunctionKVValue, http.MethodDelete, "/api/v1/functions/fn-a/kv/value?key=big", "", ""); rec.Code != http.StatusNotFound {
		t.Fatalf("second delete = %d", rec.Code)
	}
}
