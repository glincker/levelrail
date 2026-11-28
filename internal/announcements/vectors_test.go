package announcements

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

const vectorsDir = "../../tools/announce/testdata/vectors"

type vectorRoles struct {
	Keys []struct {
		KeyID     string   `json:"key_id"`
		PublicKey string   `json:"public_key"`
		Tiers     []string `json:"tiers"`
	} `json:"keys"`
}

type vectorContext struct {
	Now          time.Time `json:"now"`
	LastSequence int64     `json:"last_sequence"`
	MaxBytes     int64     `json:"max_bytes"`
	Revoked      []string  `json:"revoked"`
}

type vectorDrop struct {
	ID   string `json:"id"`
	Code string `json:"code"`
}

type vectorExpect struct {
	OK        bool         `json:"ok"`
	ErrorCode string       `json:"error_code"`
	Accepted  []string     `json:"accepted"`
	Dropped   []vectorDrop `json:"dropped"`
}

func readJSON(t *testing.T, path string, out any) {
	t.Helper()
	b, err := os.ReadFile(path) //nolint:gosec // fixed testdata path
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

// TestInstanceVerifiesAnnounceToolVectors pins the instance verifier to the
// golden vectors the announce tool ships: same accepts, same drops, same codes.
func TestInstanceVerifiesAnnounceToolVectors(t *testing.T) {
	var roles vectorRoles
	readJSON(t, filepath.Join(vectorsDir, "roles.json"), &roles)
	var keys []Key
	for _, k := range roles.Keys {
		raw, err := base64.StdEncoding.DecodeString(k.PublicKey)
		if err != nil || len(raw) != ed25519.PublicKeySize {
			t.Fatalf("role key %s is not an ed25519 key", k.KeyID)
		}
		keys = append(keys, Key{ID: k.KeyID, Public: ed25519.PublicKey(raw), Tiers: k.Tiers})
	}
	dirs, err := os.ReadDir(vectorsDir)
	if err != nil {
		t.Fatal(err)
	}
	cases := 0
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		cases++
		dir := filepath.Join(vectorsDir, d.Name())
		t.Run(d.Name(), func(t *testing.T) {
			var ctx vectorContext
			var want vectorExpect
			readJSON(t, filepath.Join(dir, "context.json"), &ctx)
			readJSON(t, filepath.Join(dir, "expect.json"), &want)
			data, err := os.ReadFile(filepath.Join(dir, "feed.v1.json")) //nolint:gosec // fixed testdata path
			if err != nil {
				t.Fatal(err)
			}
			sig, err := os.ReadFile(filepath.Join(dir, "feed.v1.json.sig")) //nolint:gosec // fixed testdata path
			if err != nil {
				t.Fatal(err)
			}
			lim := testLimits()
			lim.MaxAge = 0
			if ctx.MaxBytes > 0 {
				lim.MaxBytes = ctx.MaxBytes
			}
			revoked := map[string]bool{}
			for _, id := range ctx.Revoked {
				revoked[id] = true
			}
			got, err := VerifyFeed(VerifyInput{
				Data: data, SigData: sig, Keys: keys, Revoked: revoked,
				LastSequence: ctx.LastSequence, Now: ctx.Now, Limits: lim,
			})
			if code := ErrorCode(err); code != want.ErrorCode || (err == nil) != want.OK {
				t.Fatalf("outcome: err=%v code=%q, want ok=%v code=%q", err, code, want.OK, want.ErrorCode)
			}
			if err != nil {
				return
			}
			accepted := []string{}
			for _, it := range got.Items {
				accepted = append(accepted, it.ID)
			}
			if !reflect.DeepEqual(accepted, append([]string{}, want.Accepted...)) {
				t.Fatalf("accepted %v, want %v", accepted, want.Accepted)
			}
			dropped := []vectorDrop{}
			for _, r := range got.Rejected {
				dropped = append(dropped, vectorDrop{ID: r.ID, Code: r.Code})
			}
			if !reflect.DeepEqual(dropped, append([]vectorDrop{}, want.Dropped...)) {
				t.Fatalf("dropped %v, want %v", dropped, want.Dropped)
			}
		})
	}
	if cases < 12 {
		t.Fatalf("expected 12 golden vectors, found %d", cases)
	}
}
