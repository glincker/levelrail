package catalog

import "testing"

func TestTrustLevels(t *testing.T) {
	signed := &SignedBy{Method: "keyless", Identity: "x"}
	e := Marketplace{Images: []ImageFact{{SourceClass: SourceUpstream, Signed: signed, Provenance: true}}}
	hist := VerifiedRecord{Arches: []string{"arm64"}}
	got := trustFor(e, hist, true)
	if got.Level != TrustVerifiedSigned || got.EvidenceURL != StatusPageURL {
		t.Errorf("arm64 local pass = %+v, want signed with status page evidence", got)
	}
	e.Images[0].Signed = nil
	got = trustFor(e, hist, true)
	if got.Level != TrustVerified {
		t.Errorf("arm64 local pass unsigned = %q, want verified", got.Level)
	}
	if got := trustFor(e, VerifiedRecord{}, true).Level; got != TrustListed {
		t.Errorf("record without arches = %q, want listed", got)
	}
	e.Images[0].Signed = signed
	ci := VerifiedRecord{Arches: []string{"amd64"}, RunURL: "https://example.com/run/1"}
	if got := trustFor(e, ci, true).Level; got != TrustVerifiedSigned {
		t.Errorf("signed CI run = %q, want verified_signed", got)
	}
	e.Images[0].Signed = nil
	if got := trustFor(e, ci, true).Level; got != TrustVerified {
		t.Errorf("unsigned CI run = %q, want verified", got)
	}
	if got := trustFor(e, ci, false).Level; got != TrustListed {
		t.Errorf("no record = %q, want listed", got)
	}
}

func TestArchStates(t *testing.T) {
	imgs := []ImageFact{{Platforms: []string{"linux/amd64"}}}
	got := archStates(imgs, VerifiedRecord{Arches: []string{"amd64"}}, true)
	if got["amd64"] != "tested" || got["arm64"] != "absent" {
		t.Errorf("archStates = %v", got)
	}
	if archStates([]ImageFact{{}}, VerifiedRecord{}, false)["amd64"] != "unknown" {
		t.Error("unscanned image must be unknown")
	}
}
