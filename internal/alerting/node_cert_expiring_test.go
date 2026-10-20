package alerting

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

func certNode(name string, left time.Duration, now time.Time) store.Node {
	na := now.Add(left)
	return store.Node{ID: "id-" + name, Name: name, CertNotAfter: &na}
}

func TestClassifyNodeCert(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	revoked := now.Add(-time.Hour)
	tests := []struct {
		name string
		node store.Node
		want NodeCertState
	}{
		{"healthy", certNode("a", 40*day, now), NodeCertOK},
		{"inside warning", certNode("a", 20*day, now), NodeCertExpiring},
		{"inside critical", certNode("a", 3*day, now), NodeCertCritical},
		{"expired", certNode("a", -time.Minute, now), NodeCertExpired},
		{"unknown expiry", store.Node{ID: "x"}, NodeCertUnknown},
		{"revoked wins over expiry", store.Node{ID: "x", CertRevokedAt: &revoked, CertNotAfter: &revoked}, NodeCertRevoked},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := ClassifyNodeCert(tt.node, DefaultNodeCertWarning, DefaultNodeCertCritical, now); got != tt.want {
				t.Errorf("ClassifyNodeCert() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestEvaluateNodeCertExpiring(t *testing.T) {
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	revoked := now
	tests := []struct {
		name        string
		nodes       []store.Node
		forDuration time.Duration
		prevFiring  bool
		wantFiring  bool
		wantNotices []string
		wantValue   *float64
	}{
		{"all healthy", []store.Node{certNode("a", 60*day, now)}, 0, false, false, nil, nil},
		{"one expiring", []store.Node{certNode("a", 60*day, now), certNode("web-2", 10*day, now)}, 0, false, true, []string{"web-2: agent certificate expires in 240h0m0s"}, ptr(10.0)},
		{"expired asks for re-enroll", []store.Node{certNode("gone", -2*day, now)}, 0, false, true, []string{"re-enroll"}, ptr(-2.0)},
		{"revoked is not alerted", []store.Node{{ID: "r", Name: "r", CertRevokedAt: &revoked}}, 0, false, false, nil, nil},
		{"rule window overrides default", []store.Node{certNode("a", 25*day, now)}, 30 * day, false, true, []string{"a:"}, ptr(25.0)},
		{"renewed resolves", []store.Node{certNode("a", 89*day, now)}, 0, true, false, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := Rule{ID: "r1", Kind: KindNodeCertExpiring, Enabled: true, ForDuration: tt.forDuration, Firing: tt.prevFiring}
			got, notices, err := EvaluateNodeCertExpiring(context.Background(), &fakeNodeSource{nodes: tt.nodes}, r, DefaultNodeCertWarning, DefaultNodeCertCritical, now)
			if err != nil {
				t.Fatal(err)
			}
			if got.Firing != tt.wantFiring || len(notices) != len(tt.wantNotices) {
				t.Fatalf("firing=%v notices=%v, want firing=%v notices=%v", got.Firing, notices, tt.wantFiring, tt.wantNotices)
			}
			for i, want := range tt.wantNotices {
				if !strings.Contains(notices[i], want) {
					t.Errorf("notice %q does not contain %q", notices[i], want)
				}
			}
			if (got.LastValue == nil) != (tt.wantValue == nil) || (got.LastValue != nil && *got.LastValue != *tt.wantValue) {
				t.Errorf("LastValue = %v, want %v", got.LastValue, tt.wantValue)
			}
		})
	}
}

func TestEvaluateNodeCertExpiring_ListError(t *testing.T) {
	if _, _, err := EvaluateNodeCertExpiring(context.Background(), errNodeSource{}, Rule{ID: "r"}, time.Hour, time.Hour, time.Now()); err == nil {
		t.Fatal("want error")
	}
}

func TestSummaryText_IncludesNodeCertNotices(t *testing.T) {
	ev := Event{Rule: Rule{Name: "certs", Kind: KindNodeCertExpiring, Firing: true}, NodeCertNotices: []string{"web-1: agent certificate expires in 48h0m0s"}}
	if got := summaryText(ev); !strings.Contains(got, "Node certificates:\n- web-1") {
		t.Fatalf("summaryText() = %q", got)
	}
}

func ptr(f float64) *float64 { return &f }
