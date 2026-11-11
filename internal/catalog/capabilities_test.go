package catalog

import (
	"errors"
	"strings"
	"testing"

	"github.com/GLINCKER/levelrail/internal/dockerguard"
	"github.com/GLINCKER/levelrail/internal/spec"
)

const capCompose = `services:
  web:
    image: nginx:alpine
    ports: ["80"]
    cap_add: [CHOWN, net_raw]
`

const capManifestBase = testManifest

func capManifest(blocks string) string { return capManifestBase + blocks }

func TestManifestCapabilities_Validation(t *testing.T) {
	cases := []struct {
		name, caps, compose, want string
	}{
		{"net_admin class reaches compose as an Elevated grant", "capabilities:\n  - {class: cap_add, service: web, reason: chown data dir, caps: [CHOWN]}\n  - {class: net_admin, service: web, reason: raw sockets, caps: [NET_RAW]}\n", capCompose, ""},
		{"undeclared cap_add", "", capCompose, "does not declare it"},
		{"partly declared", "capabilities:\n  - {class: cap_add, service: web, reason: chown, caps: [CHOWN]}\n", capCompose, "NET_RAW in cap_add but the manifest does not declare"},
		{"unused declaration", "capabilities:\n  - {class: cap_add, service: web, reason: chown, caps: [CHOWN, FOWNER]}\n  - {class: net_admin, service: web, reason: r, caps: [NET_RAW]}\n", capCompose, "FOWNER is declared for service \"web\" but its cap_add does not use it"},
		{"unknown class", "capabilities:\n  - {class: root_shell, service: web, reason: x, caps: [CHOWN]}\n", testCompose, "unknown class"},
		{"privileged never", "capabilities:\n  - {class: privileged, service: web, reason: x}\n", testCompose, "never allowed"},
		{"reserved class", "capabilities:\n  - {class: devices, service: web, reason: x}\n", testCompose, "not available yet"},
		{"cap outside allowlist", "capabilities:\n  - {class: cap_add, service: web, reason: x, caps: [SYS_ADMIN]}\n", testCompose, "outside the cap_add allowlist"},
		{"net_admin cap in review class", "capabilities:\n  - {class: cap_add, service: web, reason: x, caps: [NET_ADMIN]}\n", testCompose, "outside the cap_add allowlist"},
		{"missing reason", "capabilities:\n  - {class: cap_add, service: web, caps: [CHOWN]}\n", testCompose, "missing reason"},
		{"unknown service", "capabilities:\n  - {class: cap_add, service: ghost, reason: x, caps: [CHOWN]}\n", testCompose, "service \"ghost\""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			compose := testCompose
			if tc.compose == capCompose {
				compose = strings.Replace(testCompose, "    ports: [\"80\"]\n", "    ports: [\"80\"]\n    cap_add: [CHOWN, net_raw]\n", 1)
			}
			_, err := LoadManifests(demoFS(capManifest(tc.caps), compose), "m")
			if tc.want == "" {
				if err != nil {
					t.Fatalf("error = %v, want none", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestManifestCapabilities_SubmissionRejectsPrivileged(t *testing.T) {
	m := Manifest{Capabilities: []ManifestCapability{{Class: CapClassPrivileged, Service: "web", Reason: "x"}}}
	errs := m.validateSubmissionCapabilities()
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "never allowed") {
		t.Fatalf("errs = %v", errs)
	}
}

func TestCapabilityGrants_Policy(t *testing.T) {
	m := Manifest{Capabilities: []ManifestCapability{
		{Class: CapClassCapAdd, Service: "web", Reason: "r", Caps: []string{"chown"}},
		{Class: CapClassNetAdmin, Service: "vpn", Reason: "r", Caps: []string{"CAP_NET_ADMIN"}},
	}}
	if _, err := m.CapabilityGrants(CapabilityPolicy{}); !errors.Is(err, ErrCapabilityDisabled) {
		t.Fatalf("setting off: err = %v, want ErrCapabilityDisabled", err)
	}
	grants, err := m.CapabilityGrants(CapabilityPolicy{Enabled: map[string]bool{CapClassNetAdmin: true}})
	if err != nil || len(grants) != 2 {
		t.Fatalf("setting on: %v, %v", grants, err)
	}
	if got := CapGrantMap(grants); got["web"][0] != "CHOWN" || got["vpn"][0] != "NET_ADMIN" {
		t.Errorf("grant map = %v", got)
	}
	review := Manifest{Capabilities: m.Capabilities[:1]}
	if _, err := review.CapabilityGrants(CapabilityPolicy{}); err != nil {
		t.Errorf("review tier needs no setting: %v", err)
	}
}

func TestRequirements_ReportsDeclaredCapabilities(t *testing.T) {
	m := Manifest{Capabilities: []ManifestCapability{{Class: CapClassCapAdd, Service: "web", Reason: "chown", Caps: []string{"CHOWN"}}}}
	r := ComputeRequirementsFor(capCompose, &m)
	if len(r.Capabilities) != 1 || r.Capabilities[0].Tier != CapTierReview || r.Capabilities[0].Reason != "chown" {
		t.Fatalf("capabilities = %+v", r.Capabilities)
	}
}

func TestCapabilityGrants_CapAddOutsideReviewAllowlist(t *testing.T) {
	m := Manifest{Capabilities: []ManifestCapability{{Class: CapClassCapAdd, Service: "web", Reason: "r", Caps: []string{"NET_ADMIN"}}}}
	if _, err := m.CapabilityGrants(CapabilityPolicy{Enabled: map[string]bool{CapClassNetAdmin: true}}); err == nil {
		t.Fatal("cap_add class carrying NET_ADMIN must be refused even when Validate was bypassed")
	}
}

func TestElevatedCapListsAgree(t *testing.T) {
	for _, c := range dockerguard.GrantableCaps {
		if !spec.IsElevatedCap(c) {
			t.Errorf("spec.IsElevatedCap(%q) = false, but the guard lists it as grantable", c)
		}
	}
	if spec.IsElevatedCap("CHOWN") {
		t.Error("CHOWN is a Review cap")
	}
}
