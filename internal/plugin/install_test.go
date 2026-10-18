package plugin

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
)

const testDigest2 = "sha256:0000000000000000000000000000000000000000000000000000000000000002"

func manifestV(version, digest, abilities, hosts, extra string) []byte {
	return []byte("api_version: 1\nname: example\nversion: " + version + "\nkind: notification_channel\npublisher: Example Org\n" +
		"image: registry.example.com/team/example@" + digest + "\nabilities: " + abilities + "\noutbound_hosts: " + hosts + "\n" + extra)
}

func (h *harness) requestV2(manifest []byte) InstallRequest {
	return InstallRequest{Manifest: manifest, KeyID: h.keyID, Signature: Sign(h.priv, manifest, testDigest2)}
}

func TestManifestValidationTable(t *testing.T) {
	cases := []struct {
		name    string
		raw     []byte
		wantErr string
	}{
		{"valid", manifestV("1.0.0", testDigest, "[read]", "[]", ""), ""},
		{"valid with panel", manifestV("1.0.0", testDigest, "[read]", "[]", "contributes:\n  panels:\n    - id: status\n      title: Status\n      rows:\n        - {label: Region, value: eu}\n"), ""},
		{"unknown field", manifestV("1.0.0", testDigest, "[read]", "[]", "surprise: true\n"), "parse manifest"},
		{"tag instead of digest", []byte(strings.Replace(string(manifestV("1.0.0", testDigest, "[read]", "[]", "")), "@"+testDigest, ":latest", 1)), "tags are refused"},
		{"root ability", manifestV("1.0.0", testDigest, "[read, root]", "[]", ""), "root"},
		{"write ability", manifestV("1.0.0", testDigest, "[read, write]", "[]", ""), "not available"},
		{"missing read", manifestV("1.0.0", testDigest, "[read:sensitive]", "[]", ""), "read ability"},
		{"bad host", manifestV("1.0.0", testDigest, "[read]", "['http://x.example']", ""), "bare hostname"},
		{"bad version", manifestV("bad version", testDigest, "[read]", "[]", ""), "version"},
		{"panel id uppercase", manifestV("1.0.0", testDigest, "[read]", "[]", "contributes:\n  panels:\n    - id: Status\n      title: S\n"), "manifest schema"},
		{"panel without title", manifestV("1.0.0", testDigest, "[read]", "[]", "contributes:\n  panels:\n    - id: status\n"), "manifest schema"},
		{"panel row extra key", manifestV("1.0.0", testDigest, "[read]", "[]", "contributes:\n  panels:\n    - id: s\n      title: S\n      rows:\n        - {label: a, value: b, script: x}\n"), "parse manifest"},
		{"too many panels", manifestV("1.0.0", testDigest, "[read]", "[]", "contributes:\n  panels:\n"+strings.Repeat("    - {id: p, title: T}\n", 5)), "manifest schema"},
		{"empty", nil, "manifest must be"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseManifest(tc.raw)
			if tc.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			var inv *InvalidError
			if !errors.As(err, &inv) || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want InvalidError containing %q", err, tc.wantErr)
			}
		})
	}
}

func TestInstallRefusesUnsignedUnlessAllowed(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	req := InstallRequest{Manifest: manifestYAML("[read]")}
	if _, err := h.m.Install(ctx, req, "admin"); !errors.Is(err, ErrUnsigned) {
		t.Fatalf("unsigned install err = %v, want ErrUnsigned", err)
	}
	pv, err := h.m.Inspect(ctx, req)
	if err != nil || pv.Verification.Installable || pv.Verification.Status != VerifyUnsignedBlock {
		t.Fatalf("inspect = %+v, %v; want a refused unsigned preview", pv.Verification, err)
	}
	h.m.cfg.AllowUnsigned = true
	rec, err := h.m.Install(ctx, req, "admin")
	if err != nil || rec.KeyID != "" {
		t.Fatalf("allowed unsigned install = %+v, %v", rec, err)
	}
	if st := h.m.StatusOf(ctx, rec); st.Verification != VerifyUnsigned {
		t.Fatalf("status = %+v, want unsigned", st)
	}
}

func TestInstallSignatureRefusals(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	good := h.request(manifestYAML("[read]"))

	tampered := good
	tampered.Manifest = append(append([]byte{}, good.Manifest...), []byte("# edited\n")...)
	if _, err := h.m.Install(ctx, tampered, "admin"); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("tampered manifest err = %v, want ErrBadSignature", err)
	}
	unknownKey := good
	unknownKey.KeyID = "ed25519:nope"
	if _, err := h.m.Install(ctx, unknownKey, "admin"); !errors.Is(err, ErrUntrusted) {
		t.Fatalf("unknown key err = %v, want ErrUntrusted", err)
	}
	keyNoSig := InstallRequest{Manifest: good.Manifest, KeyID: h.keyID}
	h.m.cfg.AllowUnsigned = true
	if _, err := h.m.Install(ctx, keyNoSig, "admin"); !errors.Is(err, ErrBadSignature) {
		t.Fatalf("key without signature err = %v, want ErrBadSignature even when unsigned is allowed", err)
	}
	wrongDigest := good
	wrongDigest.ExpectedDigest = testDigest2
	if _, err := h.m.Install(ctx, wrongDigest, "admin"); !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("digest pin err = %v, want ErrDigestMismatch", err)
	}
	if recs, _ := h.m.List(ctx); len(recs) != 0 {
		t.Fatalf("a refused install left %d records", len(recs))
	}
}

func TestInstallFindsSigningKeyAndApprovesExactSet(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	req := h.request(manifestYAML("[read, read:sensitive]"))
	req.KeyID = ""
	req.Approve = []string{"read"}
	if _, err := h.m.Install(ctx, req, "admin"); err == nil || !strings.Contains(err.Error(), "exactly") {
		t.Fatalf("partial approval err = %v, want the exact-set refusal", err)
	}
	req.Approve = []string{"read:sensitive", "read"}
	rec, err := h.m.Install(ctx, req, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if rec.KeyID != h.keyID || !slices.Equal(rec.ApprovedAbilities, []string{"read", "read:sensitive"}) || rec.ApprovedBy != "admin" {
		t.Fatalf("record = %+v, want key detected and both abilities approved", rec)
	}
}

func TestUpgradeAbilityWideningNeedsReapproval(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.installApprovedEnabled("[read]", []string{"read"})

	pv, err := h.m.Inspect(ctx, h.requestV2(manifestV("2.0.0", testDigest2, "[read, read:sensitive]", "[]", "")))
	if err != nil || pv.Action != "upgrade" || !pv.ReapprovalRequired || !slices.Equal(pv.AddedAbilities, []string{"read:sensitive"}) {
		t.Fatalf("preview = %+v, %v; want an upgrade needing re-approval", pv, err)
	}

	rec, err := h.m.Upgrade(ctx, h.requestV2(manifestV("2.0.0", testDigest2, "[read, read:sensitive]", "[]", "")), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if rec.Version != "2.0.0" || rec.Digest != testDigest2 || rec.ApprovedAt != "" || len(rec.ApprovedAbilities) != 0 || rec.State != StateDisabled {
		t.Fatalf("widened upgrade = %+v, want new package, approval cleared and plugin stopped", rec)
	}
	if _, err := h.m.Enable(ctx, "example", "admin"); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("enable after widening err = %v, want ErrNotApproved", err)
	}
	if st := h.m.StatusOf(ctx, rec); st.Fix != FixApprove {
		t.Fatalf("status = %+v, want the approve fix", st)
	}
	if _, err := h.m.Approve(ctx, "example", []string{"read", "read:sensitive"}, "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Enable(ctx, "example", "admin"); err != nil {
		t.Fatalf("enable after re-approval: %v", err)
	}
}

func TestUpgradeKeepsApprovalWhenNotWidened(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.installApprovedEnabled("[read, read:sensitive]", []string{"read"})
	rec, err := h.m.Upgrade(ctx, h.requestV2(manifestV("1.1.0", testDigest2, "[read]", "[]", "")), "admin")
	if err != nil {
		t.Fatal(err)
	}
	if rec.State != StateEnabled || !slices.Equal(rec.ApprovedAbilities, []string{"read"}) || rec.ApprovedAt == "" {
		t.Fatalf("narrowing upgrade = %+v, want approval and state kept", rec)
	}
	if _, err := h.m.Upgrade(ctx, h.requestV2(manifestV("1.1.0", testDigest2, "[read]", "[]", "")), "admin"); err == nil || !strings.Contains(err.Error(), "already installed") {
		t.Fatalf("repeat upgrade err = %v, want already installed", err)
	}
	hosts := h.request(manifestV("1.2.0", testDigest, "[read]", "[hooks.example.com]", ""))
	rec, err = h.m.Upgrade(ctx, hosts, "admin")
	if err != nil || rec.ApprovedAt != "" || rec.State != StateDisabled {
		t.Fatalf("new outbound host = %+v, %v; want re-approval", rec, err)
	}
}

func TestUpgradeWithExactApprovalAndSignedToUnsignedRefusal(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.installApprovedEnabled("[read]", []string{"read"})
	req := h.requestV2(manifestV("2.0.0", testDigest2, "[read, read:sensitive]", "[]", ""))
	req.Approve = []string{"read", "read:sensitive"}
	rec, err := h.m.Upgrade(ctx, req, "admin")
	if err != nil || rec.State != StateEnabled || len(rec.ApprovedAbilities) != 2 {
		t.Fatalf("approved widening = %+v, %v; want it to stay enabled", rec, err)
	}
	h.m.cfg.AllowUnsigned = true
	if _, err := h.m.Upgrade(ctx, InstallRequest{Manifest: manifestV("3.0.0", testDigest2, "[read]", "[]", "")}, "admin"); err == nil || !strings.Contains(err.Error(), "unsigned") {
		t.Fatalf("signed to unsigned err = %v, want refusal", err)
	}
	if _, err := h.m.Upgrade(ctx, h.request([]byte(strings.Replace(string(manifestYAML("[read]")), "name: example", "name: other", 1))), "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("upgrade of an uninstalled plugin err = %v, want ErrNotFound", err)
	}
}

func TestRestartResetsBackoff(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.installApprovedEnabled("[read]", []string{"read"})
	if _, err := h.m.Restart(ctx, "missing", "admin"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("restart unknown err = %v", err)
	}
	h.m.Reconcile(ctx)
	rec, err := h.m.Restart(ctx, "example", "admin")
	if err != nil {
		t.Fatal(err)
	}
	if st := h.m.StatusOf(ctx, rec); st.Health != HealthHealthy || st.Fix != "" {
		t.Fatalf("status after restart = %+v, want healthy with no fix", st)
	}
	if _, err := h.m.Disable(ctx, "example", "admin"); err != nil {
		t.Fatal(err)
	}
	if _, err := h.m.Restart(ctx, "example", "admin"); !errors.Is(err, ErrState) {
		t.Fatalf("restart of a disabled plugin err = %v, want ErrState", err)
	}
}

func TestStatusReportsUntrustedKeyAfterRevocation(t *testing.T) {
	h := newHarness(t)
	ctx := context.Background()
	h.installApprovedEnabled("[read]", []string{"read"})
	if err := h.m.TrustRemove(ctx, h.keyID, "admin"); err != nil {
		t.Fatal(err)
	}
	rec, _ := h.m.Get(ctx, "example")
	if st := h.m.StatusOf(ctx, rec); st.Verification != VerifyUntrustedKey || st.Fix != FixTrustKey {
		t.Fatalf("status = %+v, want untrusted key with the trust_key fix", st)
	}
}
