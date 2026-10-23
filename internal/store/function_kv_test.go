package store

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

var kvNow = time.Date(2026, 10, 21, 12, 0, 0, 0, time.UTC)

func TestFunctionKVLimits(t *testing.T) {
	lim := KVLimits{MaxKeyBytes: 8, MaxValueBytes: 10, MaxKeys: 3, MaxTotalBytes: 30}
	tests := []struct {
		name  string
		setup func(t *testing.T, db *DB)
		key   string
		value string
		want  error
	}{
		{name: "fits", key: "a", value: "1234567890"},
		{name: "empty key", key: "", value: "x", want: ErrKVKeyInvalid},
		{name: "key too long", key: "123456789", value: "x", want: ErrKVKeyInvalid},
		{name: "value too large", key: "a", value: "12345678901", want: ErrKVValueTooLarge},
		{
			name: "key count reached",
			setup: func(t *testing.T, db *DB) {
				mustPut(t, db, "ns", "k1", "v", 0, lim)
				mustPut(t, db, "ns", "k2", "v", 0, lim)
				mustPut(t, db, "ns", "k3", "v", 0, lim)
			},
			key: "k4", value: "v", want: ErrKVTooManyKeys,
		},
		{
			name: "overwriting at the key limit is allowed",
			setup: func(t *testing.T, db *DB) {
				mustPut(t, db, "ns", "k1", "v", 0, lim)
				mustPut(t, db, "ns", "k2", "v", 0, lim)
				mustPut(t, db, "ns", "k3", "v", 0, lim)
			},
			key: "k3", value: "w",
		},
		{
			name: "total bytes exceeded",
			setup: func(t *testing.T, db *DB) {
				mustPut(t, db, "ns", "a", "1234567890", 0, KVLimits{})
				mustPut(t, db, "ns", "b", "1234567890", 0, KVLimits{})
			},
			key: "c", value: "1234567890", want: ErrKVNamespaceFull,
		},
		{
			name: "overwrite counts only the growth",
			setup: func(t *testing.T, db *DB) {
				mustPut(t, db, "ns", "a", "1234567890", 0, KVLimits{})
				mustPut(t, db, "ns", "b", "1234567890", 0, KVLimits{})
			},
			key: "a", value: "123456789",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := openTestDB(t)
			if tt.setup != nil {
				tt.setup(t, db)
			}
			err := db.PutFunctionKV(context.Background(), "ns", tt.key, []byte(tt.value), 0, lim, kvNow)
			if !errors.Is(err, tt.want) {
				t.Fatalf("err = %v, want %v", err, tt.want)
			}
		})
	}
}

func mustPut(t *testing.T, db *DB, ns, key, value string, ttl time.Duration, lim KVLimits) {
	t.Helper()
	if err := db.PutFunctionKV(context.Background(), ns, key, []byte(value), ttl, lim, kvNow); err != nil {
		t.Fatalf("put %s/%s: %v", ns, key, err)
	}
}

func TestFunctionKVTTL(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	mustPut(t, db, "ns", "short", "v", time.Minute, KVLimits{})
	mustPut(t, db, "ns", "forever", "v", 0, KVLimits{})

	if _, err := db.GetFunctionKV(ctx, "ns", "short", kvNow.Add(30*time.Second)); err != nil {
		t.Fatalf("live key: %v", err)
	}
	later := kvNow.Add(2 * time.Minute)
	if _, err := db.GetFunctionKV(ctx, "ns", "short", later); !errors.Is(err, ErrKVNotFound) {
		t.Fatalf("expired key err = %v", err)
	}
	keys, err := db.ListFunctionKV(ctx, "ns", "", 0, false, later)
	if err != nil || len(keys) != 1 || keys[0].Key != "forever" {
		t.Fatalf("list after expiry = %+v err=%v", keys, err)
	}
	if u, _ := db.FunctionKVUsage(ctx, "ns", later); u.Keys != 1 {
		t.Fatalf("usage keys = %d, want 1", u.Keys)
	}

	small := KVLimits{MaxKeys: 1}
	if err := db.PutFunctionKV(ctx, "ns", "new", []byte("v"), 0, small, later); !errors.Is(err, ErrKVTooManyKeys) {
		t.Fatalf("live key still counts: %v", err)
	}
	if err := db.PutFunctionKV(ctx, "other", "x", []byte("v"), time.Minute, small, kvNow); err != nil {
		t.Fatal(err)
	}
	if err := db.PutFunctionKV(ctx, "other", "y", []byte("v"), 0, small, later); err != nil {
		t.Fatalf("expired key must free its slot: %v", err)
	}
}

func TestFunctionKVListPrefix(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	for _, k := range []string{"user:1", "user:2", "userx", "order:1", "100%_a", "100xxa"} {
		mustPut(t, db, "ns", k, "v", 0, KVLimits{})
	}
	tests := []struct {
		prefix string
		limit  int
		want   int
	}{{"user:", 0, 2}, {"user", 0, 3}, {"", 0, 6}, {"", 2, 2}, {"100%_", 0, 1}, {"nope", 0, 0}}
	for _, tt := range tests {
		got, err := db.ListFunctionKV(ctx, "ns", tt.prefix, tt.limit, false, kvNow)
		if err != nil || len(got) != tt.want {
			t.Errorf("prefix %q limit %d: got %d keys (err %v), want %d", tt.prefix, tt.limit, len(got), err, tt.want)
		}
	}
}

func TestFunctionKVNamespaceIsolation(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	mustPut(t, db, "fn-a", "secret", "a-value", 0, KVLimits{})
	mustPut(t, db, "fn-b", "secret", "b-value", 0, KVLimits{})
	mustPut(t, db, "fn-b", "only-b", "x", 0, KVLimits{})

	got, err := db.GetFunctionKV(ctx, "fn-a", "secret", kvNow)
	if err != nil || string(got.Value) != "a-value" {
		t.Fatalf("a reads its own key: %+v %v", got, err)
	}
	if _, err := db.GetFunctionKV(ctx, "fn-a", "only-b", kvNow); !errors.Is(err, ErrKVNotFound) {
		t.Fatalf("a read b's key: %v", err)
	}
	list, _ := db.ListFunctionKV(ctx, "fn-a", "", 0, true, kvNow)
	if len(list) != 1 {
		t.Fatalf("a lists %d keys, want 1", len(list))
	}
	if ok, _ := db.DeleteFunctionKVKey(ctx, "fn-a", "only-b"); ok {
		t.Fatal("a deleted b's key")
	}
	if _, err := db.GetFunctionKV(ctx, "fn-b", "only-b", kvNow); err != nil {
		t.Fatalf("b's key must survive: %v", err)
	}
	if err := db.DeleteFunctionKV(ctx, "fn-a"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetFunctionKV(ctx, "fn-b", "secret", kvNow); err != nil {
		t.Fatalf("deleting a's namespace removed b's data: %v", err)
	}
}

func TestFunctionKVTokenScoping(t *testing.T) {
	ctx := context.Background()
	db := openTestDB(t)
	tokA, err := db.RotateFunctionKVToken(ctx, "fn-a", kvNow)
	if err != nil {
		t.Fatal(err)
	}
	tokB, _ := db.RotateFunctionKVToken(ctx, "fn-b", kvNow)
	if tokA == tokB {
		t.Fatal("tokens must differ per function")
	}
	if ns, err := db.ResolveFunctionKVToken(ctx, tokA); err != nil || ns != "fn-a" {
		t.Fatalf("token A resolves to %q (%v)", ns, err)
	}
	if ns, _ := db.ResolveFunctionKVToken(ctx, tokB); ns != "fn-b" {
		t.Fatalf("token B resolves to %q", ns)
	}
	for _, bad := range []string{"", "lrkv_", "lrkv_" + fmt.Sprint(tokA[5:len(tokA)-1]), HashKVToken(tokA)} {
		if _, err := db.ResolveFunctionKVToken(ctx, bad); !errors.Is(err, ErrKVTokenInvalid) {
			t.Errorf("token %q accepted: %v", bad, err)
		}
	}
	newA, _ := db.RotateFunctionKVToken(ctx, "fn-a", kvNow)
	if _, err := db.ResolveFunctionKVToken(ctx, tokA); !errors.Is(err, ErrKVTokenInvalid) {
		t.Fatal("rotated token must stop working")
	}
	if ns, _ := db.ResolveFunctionKVToken(ctx, newA); ns != "fn-a" {
		t.Fatal("new token must work")
	}
	if has, _ := db.HasFunctionKVToken(ctx, "fn-c"); has {
		t.Fatal("fn-c has no token")
	}
}
