package alerting

import (
	"context"
	"strings"
	"testing"
	"time"
)

type fakeDNSCreds []DNSCredentialHealth

func (f fakeDNSCreds) DNSCredentialHealth(context.Context, time.Duration) ([]DNSCredentialHealth, error) {
	return f, nil
}

func TestEvaluateDNSCredentialFailing(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		cred     DNSCredentialHealth
		wantFire bool
	}{
		{"ok", DNSCredentialHealth{Provider: "cloudflare", Outcome: "ok"}, false},
		{"unreachable", DNSCredentialHealth{Provider: "cloudflare", Outcome: "unreachable"}, false},
		{"revoked", DNSCredentialHealth{Provider: "cloudflare", Outcome: "invalid"}, true},
		{"no zone rights", DNSCredentialHealth{Provider: "route53", Outcome: "denied", Missing: []string{"route53:ListHostedZones"}}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, notice, err := EvaluateDNSCredentialFailing(context.Background(), fakeDNSCreds{tt.cred}, Rule{ID: "r1", Kind: KindDNSCredentialFailing}, now)
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if (notice != "") != tt.wantFire {
				t.Fatalf("notice = %q, wantFire %v", notice, tt.wantFire)
			}
			if tt.wantFire && !strings.Contains(notice, DNSCredentialFixPath) {
				t.Fatalf("notice lacks the fix pointer: %q", notice)
			}
		})
	}
}
