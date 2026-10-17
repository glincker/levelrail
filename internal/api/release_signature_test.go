package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/selfupgrade"
)

func TestSelfUpgradePlanReportsSignaturePin(t *testing.T) {
	rt, cookie, _ := newSelfUpgradeRouter(t, true, nil)
	plan := func() selfUpgradePlanResource {
		t.Helper()
		code, body := suDo(t, rt, cookie, http.MethodGet, "/api/v1/updates/self-upgrade/plan?target=v1.2.0", "")
		if code != http.StatusOK {
			t.Fatalf("status %d: %s", code, body)
		}
		var p selfUpgradePlanResource
		if err := json.Unmarshal(body, &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	if p := plan(); p.SignaturePinned {
		t.Fatalf("unpinned host reported pinned: %+v", p)
	}
	if err := selfupgrade.WritePin(rt.dataDir, selfupgrade.Pin{Version: "v1.0.0"}); err != nil {
		t.Fatal(err)
	}
	if p := plan(); !p.SignaturePinned || p.SignaturePinnedVersion != "v1.0.0" {
		t.Fatalf("pinned host: %+v", p)
	}
}

func TestDoctorCheckReleaseSignature(t *testing.T) {
	tests := []struct {
		name    string
		dataDir bool
		pinned  bool
		want    string
	}{
		{name: "no data dir", want: doctorStatusUnknown},
		{name: "not yet verified", dataDir: true, want: doctorStatusWarn},
		{name: "pinned", dataDir: true, pinned: true, want: doctorStatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt, _ := newTestRouter(t)
			if tt.dataDir {
				rt.dataDir = t.TempDir()
			}
			if tt.pinned {
				if err := selfupgrade.WritePin(rt.dataDir, selfupgrade.Pin{Version: "v0.4.0"}); err != nil {
					t.Fatal(err)
				}
			}
			c := rt.doctorCheckReleaseSignature()
			if c.Code != "signature_pin" || c.Status != tt.want {
				t.Fatalf("check = %+v, want status %s", c, tt.want)
			}
			if tt.want == doctorStatusWarn && (!strings.Contains(c.Message, "not signature verified") || c.Fix != "" || strings.Contains(strings.ToLower(c.Message), "cosign")) {
				t.Fatalf("warn must explain and carry no command: %+v", c)
			}
		})
	}
}

func TestSignaturePinPreflightCheck(t *testing.T) {
	signed := []string{"checksums.txt", "checksums.txt.sigstore.json"}
	unsigned := []string{"checksums.txt"}
	trust := func(state string) func() string { return func() string { return state } }
	tests := []struct {
		name       string
		in         signaturePinInputs
		wantStatus string
	}{
		{name: "not pinned is a plain note", in: signaturePinInputs{ReleaseKnown: true, Assets: signed}, wantStatus: "ok"},
		{name: "not pinned, unsigned release, still no block", in: signaturePinInputs{ReleaseKnown: true, Assets: unsigned}, wantStatus: "ok"},
		{name: "pinned, signed, trust root ready", in: signaturePinInputs{Pinned: true, ReleaseKnown: true, Assets: signed, TrustRoot: trust("ready")}, wantStatus: "ok"},
		{name: "pinned, signed, trust root cached", in: signaturePinInputs{Pinned: true, ReleaseKnown: true, Assets: signed, TrustRoot: trust("cached")}, wantStatus: "ok"},
		{name: "pinned, signed, trust root still checking", in: signaturePinInputs{Pinned: true, ReleaseKnown: true, Assets: signed, TrustRoot: trust("checking")}, wantStatus: "ok"},
		{name: "pinned, signed, trust root unavailable and uncached blocks", in: signaturePinInputs{Pinned: true, ReleaseKnown: true, Assets: signed, TrustRoot: trust("unavailable")}, wantStatus: "fail"},
		{name: "pinned, no probe configured", in: signaturePinInputs{Pinned: true, ReleaseKnown: true, Assets: signed}, wantStatus: "ok"},
		{name: "pinned, unsigned release blocks", in: signaturePinInputs{Pinned: true, ReleaseKnown: true, Assets: unsigned, TrustRoot: trust("ready")}, wantStatus: "fail"},
		{name: "pinned, release unknown", in: signaturePinInputs{Pinned: true, TrustRoot: trust("ready")}, wantStatus: "ok"},
		{name: "edge image skipped", in: signaturePinInputs{Pinned: true, Edge: true}, wantStatus: "skipped"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := signaturePinCheck(tt.in)
			if c.Code != "signature_pin" || c.Status != tt.wantStatus {
				t.Fatalf("check = %+v, want status %s", c, tt.wantStatus)
			}
			if c.Fix != nil && c.Fix.Action != "recheck" {
				t.Fatalf("only a re-check fix is allowed: %+v", c.Fix)
			}
			if strings.Contains(strings.ToLower(c.Message+c.Hint), "cosign") {
				t.Fatalf("copy must not mention cosign: %+v", c)
			}
		})
	}
}
