package store

import (
	"context"
	"reflect"
	"testing"
	"time"
)

func TestListAuditEntries_SearchAndFailed(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	alice := testAuditEntry("aud_s1", "2026-01-01T00:00:00.000000000Z")
	alice.ActorName = "Alice"
	bob := testAuditEntry("aud_s2", "2026-01-02T00:00:00.000000000Z")
	bob.ActorName = "bob"
	bob.Path = "/api/v1/apps/100%_done"
	bob.StatusCode = 500
	carol := testAuditEntry("aud_s3", "2026-01-03T00:00:00.000000000Z")
	carol.ActorName = "carol"
	carol.RemoteAddr = "10.9.8.7"
	carol.StatusCode = 403
	for _, e := range []AuditEntry{alice, bob, carol} {
		if err := db.SaveAuditEntry(ctx, e); err != nil {
			t.Fatalf("SaveAuditEntry(%s) error = %v", e.ID, err)
		}
	}
	cursor := time.Date(2026, 1, 3, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name   string
		filter AuditEntryFilter
		before *time.Time
		want   []string
	}{
		{"case-insensitive actor", AuditEntryFilter{Search: "ALICE"}, nil, []string{"aud_s1"}},
		{"remote addr", AuditEntryFilter{Search: "10.9.8"}, nil, []string{"aud_s3"}},
		{"wildcards are literal", AuditEntryFilter{Search: "100%_"}, nil, []string{"aud_s2"}},
		{"percent alone does not match all", AuditEntryFilter{Search: "%zzz"}, nil, nil},
		{"failed only", AuditEntryFilter{FailedOnly: true}, nil, []string{"aud_s3", "aud_s2"}},
		{"failed and search", AuditEntryFilter{FailedOnly: true, Search: "carol"}, nil, []string{"aud_s3"}},
		{"failed with cursor", AuditEntryFilter{FailedOnly: true}, &cursor, []string{"aud_s2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.ListAuditEntries(ctx, 50, tt.before, tt.filter)
			if err != nil {
				t.Fatalf("ListAuditEntries() error = %v", err)
			}
			var ids []string
			for _, e := range got {
				ids = append(ids, e.ID)
			}
			if !reflect.DeepEqual(ids, tt.want) {
				t.Fatalf("ids = %v, want %v", ids, tt.want)
			}
		})
	}
}
