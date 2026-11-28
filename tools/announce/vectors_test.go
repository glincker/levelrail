package main

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite testdata/vectors from the table below")

const vectorsDir = "testdata/vectors"

// Test keys come from public seed phrases, never from a stored key file.
// They are not production keys and sign nothing outside these vectors.
func testKey(id string) (ed25519.PrivateKey, RoleKey) {
	seed := sha256.Sum256([]byte("levelrail-announce-test-vector-key/" + id))
	priv := ed25519.NewKeyFromSeed(seed[:])
	return priv, RoleKey{KeyID: id, PublicKey: base64.StdEncoding.EncodeToString(priv.Public().(ed25519.PublicKey))}
}

func testRoles() *Roles {
	var r Roles
	for id, tiers := range map[string][]string{
		"N":  {"info", "feature", "maintenance", "newsletter"},
		"S1": {"advisory", "critical"},
		"S2": {"advisory", "critical"},
	} {
		_, k := testKey(id)
		k.Tiers = tiers
		r.Keys = append(r.Keys, k)
	}
	sortRoleKeys(r.Keys)
	return &r
}

func sortRoleKeys(ks []RoleKey) {
	for i := 1; i < len(ks); i++ {
		for j := i; j > 0 && ks[j].KeyID < ks[j-1].KeyID; j-- {
			ks[j], ks[j-1] = ks[j-1], ks[j]
		}
	}
}

type vectorContext struct {
	Now          string   `json:"now"`
	LastSequence int64    `json:"last_sequence"`
	MaxBytes     int      `json:"max_bytes,omitempty"`
	Revoked      []string `json:"revoked,omitempty"`
}

type vectorExpect struct {
	OK       bool     `json:"ok"`
	ErrCode  string   `json:"error_code"`
	Accepted []string `json:"accepted"`
	Dropped  []Drop   `json:"dropped"`
}

func vItem(id, tier, body string) Item {
	return Item{
		ID: id, Tier: tier, Kind: "announcement", Title: "Title " + id, Body: body,
		AppliesTo: "<1.5.14", IssuedAt: "2026-11-20T00:00:00Z", ExpiresAt: "2027-02-20T00:00:00Z",
	}
}

func vFeed(seq int64, expires string, items ...Item) Feed {
	return Feed{Schema: 1, Sequence: seq, GeneratedAt: "2026-11-25T00:00:00Z", ExpiresAt: expires, Items: items}
}

type vectorCase struct {
	name    string
	feed    Feed
	signers []string
	ctx     vectorContext
	mutate  func([]byte) []byte
	expect  vectorExpect
}

func vectorCases() []vectorCase {
	const now, farFuture = "2026-12-01T00:00:00Z", "2026-12-25T00:00:00Z"
	info := vItem("info-one", "info", "Hello from the feed.")
	adv := vItem("advisory-one", "advisory", "Upgrade from Settings > Updates.")
	crit := vItem("critical-one", "critical", "Upgrade now from Settings > Updates.")
	three := vFeed(5, farFuture, info, adv, crit)
	ctx := vectorContext{Now: now, LastSequence: 4}
	none := []Drop{}
	return []vectorCase{
		{name: "valid", feed: three, signers: []string{"N", "S1", "S2"}, ctx: ctx,
			expect: vectorExpect{OK: true, Accepted: []string{"info-one", "advisory-one", "critical-one"}, Dropped: none}},
		{name: "critical_one_signature", feed: three, signers: []string{"N", "S1"}, ctx: ctx,
			expect: vectorExpect{OK: true, Accepted: []string{"info-one", "advisory-one"}, Dropped: []Drop{{ID: "critical-one", Code: dropUnsigned}}}},
		{name: "role_mismatch", feed: vFeed(5, farFuture, adv), signers: []string{"N"}, ctx: ctx,
			expect: vectorExpect{OK: true, Accepted: []string{}, Dropped: []Drop{{ID: "advisory-one", Code: dropUnsigned}}}},
		{name: "revoked_key", feed: three, signers: []string{"N", "S1", "S2"}, ctx: vectorContext{Now: now, LastSequence: 4, Revoked: []string{"S2"}},
			expect: vectorExpect{OK: true, Accepted: []string{"info-one", "advisory-one"}, Dropped: []Drop{{ID: "critical-one", Code: dropUnsigned}}}},
		{name: "tampered", feed: three, signers: []string{"N", "S1", "S2"}, ctx: ctx,
			mutate: func(b []byte) []byte { return []byte(strings.Replace(string(b), "Hello from", "Hullo from", 1)) },
			expect: vectorExpect{ErrCode: errBadSig, Accepted: []string{}, Dropped: none}},
		{name: "wrong_key", feed: three, signers: []string{"X"}, ctx: ctx,
			expect: vectorExpect{ErrCode: errNoTrusted, Accepted: []string{}, Dropped: none}},
		{name: "replay", feed: three, signers: []string{"N", "S1", "S2"}, ctx: vectorContext{Now: now, LastSequence: 5},
			expect: vectorExpect{ErrCode: errReplay, Accepted: []string{}, Dropped: none}},
		{name: "expired_feed", feed: vFeed(5, "2026-11-20T00:00:00Z", info), signers: []string{"N"}, ctx: ctx,
			expect: vectorExpect{ErrCode: errExpired, Accepted: []string{}, Dropped: none}},
		{name: "expired_item", feed: func() Feed {
			old := info
			old.ExpiresAt = "2026-11-25T00:00:00Z"
			return vFeed(5, farFuture, old, adv)
		}(), signers: []string{"N", "S1"}, ctx: ctx,
			expect: vectorExpect{OK: true, Accepted: []string{"advisory-one"}, Dropped: []Drop{{ID: "info-one", Code: dropExpired}}}},
		{name: "html", feed: vFeed(5, farFuture, info, vItem("html-one", "info", "Click <script>alert(1)</script> now.")),
			signers: []string{"N"}, ctx: ctx,
			expect: vectorExpect{OK: true, Accepted: []string{"info-one"}, Dropped: []Drop{{ID: "html-one", Code: dropContent}}}},
		{name: "bad_link", feed: vFeed(5, farFuture, info, vItem("link-one", "info", "See [the fix](https://evil.example/fix).")),
			signers: []string{"N"}, ctx: ctx,
			expect: vectorExpect{OK: true, Accepted: []string{"info-one"}, Dropped: []Drop{{ID: "link-one", Code: dropContent}}}},
		{name: "oversize", feed: three, signers: []string{"N", "S1", "S2"}, ctx: vectorContext{Now: now, LastSequence: 4, MaxBytes: 512},
			expect: vectorExpect{ErrCode: errTooLarge, Accepted: []string{}, Dropped: none}},
	}
}

func writeJSON(t *testing.T, path string, v any) {
	t.Helper()
	raw, err := canonical(v)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeVectors(t *testing.T) {
	t.Helper()
	if err := os.MkdirAll(vectorsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeJSON(t, filepath.Join(vectorsDir, "roles.json"), testRoles())
	for _, c := range vectorCases() {
		dir := filepath.Join(vectorsDir, c.name)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		raw, err := canonical(c.feed)
		if err != nil {
			t.Fatal(err)
		}
		var sigs []SigEntry
		for _, id := range c.signers {
			priv, _ := testKey(id)
			sigs = append(sigs, SigEntry{KeyID: id, Sig: base64.StdEncoding.EncodeToString(ed25519.Sign(priv, raw))})
		}
		if c.mutate != nil {
			raw = c.mutate(raw)
		}
		if err := os.WriteFile(filepath.Join(dir, "feed.v1.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
		writeJSON(t, filepath.Join(dir, "feed.v1.json.sig"), sigs)
		writeJSON(t, filepath.Join(dir, "context.json"), c.ctx)
		writeJSON(t, filepath.Join(dir, "expect.json"), c.expect)
	}
}

func TestVectors(t *testing.T) {
	if *update {
		writeVectors(t)
	}
	roles, err := loadRoles(filepath.Join(vectorsDir, "roles.json"))
	if err != nil {
		t.Fatal(err)
	}
	cases := vectorCases()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := filepath.Join(vectorsDir, c.name)
			raw, _ := os.ReadFile(filepath.Join(dir, "feed.v1.json"))
			sig, _ := os.ReadFile(filepath.Join(dir, "feed.v1.json.sig"))
			var ctx vectorContext
			var want vectorExpect
			for name, dst := range map[string]any{"context.json": &ctx, "expect.json": &want} {
				b, rerr := os.ReadFile(filepath.Join(dir, name))
				if rerr != nil || json.Unmarshal(b, dst) != nil {
					t.Fatalf("read %s: %v", name, rerr)
				}
			}
			now, _ := time.Parse(time.RFC3339, ctx.Now)
			lim := defaultLimits()
			if ctx.MaxBytes > 0 {
				lim.MaxBytes = ctx.MaxBytes
			}
			rev := map[string]bool{}
			for _, id := range ctx.Revoked {
				rev[id] = true
			}
			got := verifyFeed(raw, sig, roles, VerifyOpts{Now: now, LastSequence: ctx.LastSequence, Lim: lim, Revoked: rev})
			gotE := vectorExpect{OK: got.OK, ErrCode: got.ErrCode, Accepted: got.Accepted, Dropped: nonNilDrops(got.Dropped)}
			for i := range gotE.Dropped {
				gotE.Dropped[i].Detail = ""
			}
			if !reflect.DeepEqual(gotE, want) || !reflect.DeepEqual(want, c.expect) {
				t.Fatalf("got %+v\nwant %+v\ntable %+v", gotE, want, c.expect)
			}
		})
	}
}

func nonNilDrops(d []Drop) []Drop {
	if d == nil {
		return []Drop{}
	}
	return d
}
