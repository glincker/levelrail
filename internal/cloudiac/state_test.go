package cloudiac

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

type fixedKeys struct{ key []byte }

func (k fixedKeys) DeriveKey(string) []byte { return k.key }

func TestStateSealRoundTripNeverPlaintextAtRest(t *testing.T) {
	ctx := context.Background()
	db, err := store.Open(ctx, filepath.Join(t.TempDir(), "levelrail.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	keys := fixedKeys{key: bytes.Repeat([]byte{7}, 32)}
	ss, err := NewStateStore(db, keys, 4096)
	if err != nil {
		t.Fatal(err)
	}
	canary := []byte(`{"secret":"lr-canary-5b1e9f0c-state"}`)
	lock, err := db.AcquireCloudStateLock(ctx, "acct", "net", "runner", time.Minute, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	v, err := ss.Save(ctx, lock, 0, canary)
	if err != nil || v != 1 {
		t.Fatalf("save = %d, %v", v, err)
	}
	row, err := db.GetCloudState(ctx, "acct", "net")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(row.Sealed, []byte("lr-canary")) || bytes.Contains(row.Sealed, canary) {
		t.Fatal("canary is readable in the stored row")
	}
	got, ver, err := ss.Load(ctx, "acct", "net")
	if err != nil || ver != 1 || !bytes.Equal(got, canary) {
		t.Fatalf("load = %q, %d, %v", got, ver, err)
	}
	if _, err := ss.Save(ctx, lock, 1, make([]byte, 4097)); !errors.Is(err, ErrStateTooLarge) {
		t.Fatalf("oversize err = %v", err)
	}
	wrong, _ := NewStateStore(db, fixedKeys{key: bytes.Repeat([]byte{8}, 32)}, 4096)
	if _, _, err := wrong.Load(ctx, "acct", "net"); !errors.Is(err, ErrStateTampered) {
		t.Fatalf("wrong master key err = %v", err)
	}
	noKey, _ := NewStateStore(db, fixedKeys{}, 4096)
	if _, _, err := noKey.Load(ctx, "acct", "net"); !errors.Is(err, ErrStateKeyUnavailable) {
		t.Fatalf("missing master key err = %v", err)
	}
	if empty, ver, err := ss.Load(ctx, "acct", "fresh"); err != nil || empty != nil || ver != 0 {
		t.Fatalf("fresh stack = %q, %d, %v", empty, ver, err)
	}
}

func TestStateSealDetectsTampering(t *testing.T) {
	kek := bytes.Repeat([]byte{3}, 32)
	blob, err := sealLayers(kek, "key", "state", "acct", "net", []byte("state body"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := openLayers(kek, "key", "state", "acct", "net", blob); err != nil {
		t.Fatalf("clean open: %v", err)
	}
	for i := range blob {
		bad := bytes.Clone(blob)
		bad[i] ^= 0x01
		if _, err := openLayers(kek, "key", "state", "acct", "net", bad); !errors.Is(err, ErrStateTampered) {
			t.Fatalf("flip at byte %d not detected: %v", i, err)
		}
	}
	if _, err := openLayers(kek, "key", "state", "acct", "other", blob); !errors.Is(err, ErrStateTampered) {
		t.Fatalf("blob moved to another stack err = %v", err)
	}
	if _, err := openLayers(kek, "key", "state", "acct", "net", blob[:len(blob)-1]); !errors.Is(err, ErrStateTampered) {
		t.Fatalf("truncated err = %v", err)
	}
}
