package plugin

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func b64(pub ed25519.PublicKey) string { return base64.StdEncoding.EncodeToString(pub) }

func TestInstallSignatureVerification(t *testing.T) {
	ctx := context.Background()
	manifest := manifestYAML("[read]")
	otherPub, otherPriv, _ := ed25519.GenerateKey(rand.Reader)
	otherID := Fingerprint(otherPub)

	tests := []struct {
		name    string
		mutate  func(h *harness, r *InstallRequest)
		wantErr error
	}{
		{"valid", func(*harness, *InstallRequest) {}, nil},
		{"wrong key signs", func(_ *harness, r *InstallRequest) { r.Signature = Sign(otherPriv, r.Manifest, testDigest) }, ErrBadSignature},
		{"key id not in trust list", func(_ *harness, r *InstallRequest) { r.KeyID = otherID }, ErrUntrusted},
		{"empty key id", func(_ *harness, r *InstallRequest) { r.KeyID = "" }, ErrUntrusted},
		{"tampered manifest", func(_ *harness, r *InstallRequest) {
			r.Manifest = []byte(strings.Replace(string(r.Manifest), "abilities: [read]", "abilities: [read, read:sensitive]", 1))
		}, ErrBadSignature},
		{"signature over another digest", func(h *harness, r *InstallRequest) {
			r.Signature = Sign(h.priv, r.Manifest, "sha256:"+strings.Repeat("a", 64))
		}, ErrBadSignature},
		{"operator pinned digest differs", func(_ *harness, r *InstallRequest) {
			r.ExpectedDigest = "sha256:" + strings.Repeat("b", 64)
		}, ErrDigestMismatch},
		{"operator pinned digest matches", func(_ *harness, r *InstallRequest) { r.ExpectedDigest = testDigest }, nil},
		{"garbage signature", func(_ *harness, r *InstallRequest) { r.Signature = "not-base64!!" }, ErrBadSignature},
		{"empty trust list rejects everything", func(h *harness, _ *InstallRequest) { h.store.keys = nil }, ErrUntrusted},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := newHarness(t)
			req := h.request(manifest)
			tt.mutate(h, &req)
			_, err := h.m.Install(ctx, req, "admin")
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("Install error = %v, want %v", err, tt.wantErr)
			}
			recs, _ := h.store.ListPlugins(ctx)
			if tt.wantErr != nil && len(recs) != 0 {
				t.Fatalf("a rejected install left %d records", len(recs))
			}
		})
	}
}

func TestInstallDoesNotApproveOrStart(t *testing.T) {
	h := newHarness(t)
	rec, err := h.m.Install(context.Background(), h.request(manifestYAML("[read, read:sensitive]")), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != StateInstalled || len(rec.ApprovedAbilities) != 0 || rec.ApprovedAt != "" {
		t.Fatalf("fresh install = %+v, want installed with nothing approved", rec)
	}
	h.m.Reconcile(context.Background())
	if h.dk.count() != 0 {
		t.Fatal("an unapproved plugin was started")
	}
}

func TestParseManifestRefusals(t *testing.T) {
	base := string(manifestYAML("[read]"))
	tests := []struct{ name, yaml string }{
		{"tag instead of digest", strings.Replace(base, "@"+testDigest, ":latest", 1)},
		{"root ability", strings.Replace(base, "[read]", "[read, root]", 1)},
		{"write ability", strings.Replace(base, "[read]", "[read, write]", 1)},
		{"no read", strings.Replace(base, "[read]", "[read:sensitive]", 1)},
		{"unknown field", base + "privileged: true\n"},
		{"bad kind", strings.Replace(base, "notification_channel", "dns_provider", 1)},
		{"bad name", strings.Replace(base, "name: example", "name: Bad_Name", 1)},
		{"url as outbound host", strings.Replace(base, "outbound_hosts: []", "outbound_hosts: [\"https://evil.example\"]", 1)},
		{"missing publisher", strings.Replace(base, "publisher: Example Org\n", "", 1)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := ParseManifest([]byte(tt.yaml)); err == nil {
				t.Fatal("manifest was accepted")
			}
		})
	}
	if _, err := ParseManifest([]byte(base)); err != nil {
		t.Fatalf("valid manifest rejected: %v", err)
	}
}
