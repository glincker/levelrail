package secrets

import (
	"context"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/secretmask"
)

func TestPreloadRegistersEverySecretAndToleratesBadRows(t *testing.T) {
	ctx := context.Background()
	mgr, st := testManager(t)
	vals := map[string]string{"A": "preload-canary-aaaa1111", "B": "preload-canary-bbbb2222", "C": "preload-canary-cccc3333"}
	for k, v := range vals {
		if err := mgr.SetValue(ctx, "svc-one", k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := mgr.SetValue(ctx, "svc-two", "D", "preload-canary-dddd4444"); err != nil {
		t.Fatal(err)
	}
	// A corrupt row between good ones must not stop the rest.
	st.values["svc-one"]["B"] = []byte("not a ciphertext")

	loaded, failed := mgr.Preload(ctx)
	if loaded != 3 || failed != 1 {
		t.Fatalf("loaded=%d failed=%d, want 3 and 1", loaded, failed)
	}
	for _, v := range []string{vals["A"], vals["C"], "preload-canary-dddd4444"} {
		if strings.Contains(secretmask.Mask("x "+v), v) {
			t.Errorf("%s was not registered by Preload", v)
		}
	}
}
