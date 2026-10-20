package attention

import (
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/apiclient"
)

func reenrollNode(left time.Duration, now time.Time, state string, r *apiclient.NodeCertRenewal) apiclient.NodeResource {
	na := now.Add(left)
	c := &apiclient.NodeCertResource{State: state, NotAfter: &na, KeyOrigin: "reenroll_required", WarningDays: 21, CriticalDays: 7, Renewal: r}
	return apiclient.NodeResource{ID: "a", Name: "a", Status: "online", Cert: c}
}

func TestBuild_NodeReenrollRequired(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	retry := now.Add(time.Hour)
	tests := []struct {
		name     string
		node     apiclient.NodeResource
		wantSev  string
		wantKind []string
	}{
		{"plenty of time", reenrollNode(90*day, now, "ok", nil), Info, []string{KindNodeReenroll}},
		{"inside warning window", reenrollNode(15*day, now, "expiring", nil), Warning, []string{KindNodeReenroll}},
		{"near expiry", reenrollNode(3*day, now, "critical", nil), Critical, []string{KindNodeReenroll}},
		{"already expired", reenrollNode(-day, now, "expired", nil), Critical, []string{KindNodeReenroll}},
		{"renewal failure too", reenrollNode(3*day, now, "critical", &apiclient.NodeCertRenewal{LastError: "refused", NextRetryAt: &retry}), Critical, []string{KindNodeReenroll}},
		{"revoked keeps both", reenrollNode(30*day, now, "revoked", nil), Info, []string{"node_cert", KindNodeReenroll}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Build(Input{Nodes: []apiclient.NodeResource{tt.node}, Now: now})
			if len(got) != len(tt.wantKind) {
				t.Fatalf("got %+v, want kinds %v", got, tt.wantKind)
			}
			for i, k := range tt.wantKind {
				if got[i].Kind != k {
					t.Fatalf("item %d kind %s, want %s", i, got[i].Kind, k)
				}
			}
			var it Item
			for _, g := range got {
				if g.Kind == KindNodeReenroll {
					it = g
				}
			}
			if it.Severity != tt.wantSev || it.ID != "node_reenroll:a" || it.Title != "Node a needs to be re-enrolled" || it.Link != "/nodes/a?reenroll=1" {
				t.Fatalf("unexpected item %+v", it)
			}
		})
	}
}

func TestBuild_NodeReenrollGoneOrLocal(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	re := reenrollNode(30*24*time.Hour, now, "ok", nil)
	re.Cert.KeyOrigin = "agent"
	local := apiclient.NodeResource{ID: "l", Name: "local", Status: "online", IsLocal: true}
	tests := []struct {
		name  string
		nodes []apiclient.NodeResource
	}{
		{"re-enrolled node disappears", []apiclient.NodeResource{re}},
		{"deleted node is not listed", nil},
		{"local node never appears", []apiclient.NodeResource{local}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Build(Input{Nodes: tt.nodes, Now: now}); len(got) != 0 {
				t.Fatalf("got %+v, want none", got)
			}
		})
	}
}

func TestBuild_RenewalFailingCarriesError(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	retry := now.Add(time.Hour)
	pem := "-----BEGIN PRIVATE KEY-----\nMIIEvQIBADANBgkq\n-----END PRIVATE KEY-----"
	tests := []struct {
		name, errText string
		wantHas       string
		wantNot       string
	}{
		{"plain", "dial tcp: refused", "Last error: dial tcp: refused", ""},
		{"long truncated", strings.Repeat("x", 5000), strings.Repeat("x", MaxErrorText) + "...", strings.Repeat("x", MaxErrorText+1)},
		{"pem never shown", pem, "Last error: [redacted]", "MIIEvQ"},
		{"control chars stripped", "bad\x1b[31m\nline", "bad [31m line", "\x1b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := reenrollNode(3*24*time.Hour, now, "critical", &apiclient.NodeCertRenewal{LastError: tt.errText, NextRetryAt: &retry})
			n.Cert.KeyOrigin = "agent"
			got := Build(Input{Nodes: []apiclient.NodeResource{n}, Now: now})
			if len(got) != 1 || got[0].Kind != "node_cert" {
				t.Fatalf("got %+v", got)
			}
			d := got[0].Detail
			if !strings.Contains(d, tt.wantHas) || !strings.Contains(d, "Next retry 2026-10-16 13:00 UTC") {
				t.Fatalf("detail %q missing %q", d, tt.wantHas)
			}
			if tt.wantNot != "" && strings.Contains(d, tt.wantNot) {
				t.Fatalf("detail %q must not contain %q", d, tt.wantNot)
			}
		})
	}
}
