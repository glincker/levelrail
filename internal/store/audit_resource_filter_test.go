package store

import (
	"context"
	"reflect"
	"testing"
)

func TestListAuditEntries_PathLikeAndActionPrefixes(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	rows := []struct {
		id, at, path, action string
	}{
		{"a1", "2026-02-01T00:00:00.000000000Z", "/api/v1/apps/shop/domains/shop.example.com/redirect", ""},
		{"a2", "2026-02-02T00:00:00.000000000Z", "/api/v1/apps/shop/domains/shop.example.com.evil.io/redirect", ""},
		{"a3", "2026-02-03T00:00:00.000000000Z", "/api/v1/certificates/shop.example.com", ""},
		{"a4", "2026-02-04T00:00:00.000000000Z", "/api/v1/apps/shop", "dns_record.created"},
		{"a5", "2026-02-05T00:00:00.000000000Z", "/api/v1/apps/shop_x", "proxy_route.written"},
		{"a6", "2026-02-06T00:00:00.000000000Z", "/api/v1/apps/shopx", "domain.added"},
	}
	for _, r := range rows {
		e := testAuditEntry(r.id, r.at)
		e.Path, e.Action = r.path, r.action
		if err := db.SaveAuditEntry(ctx, e); err != nil {
			t.Fatalf("SaveAuditEntry(%s) error = %v", r.id, err)
		}
	}
	domain := AuditLikeLiteral("shop.example.com")
	app := AuditLikeLiteral("shop")

	tests := []struct {
		name   string
		filter AuditEntryFilter
		want   []string
	}{
		{"domain paths", AuditEntryFilter{PathLike: []string{"%/domains/" + domain, "%/domains/" + domain + "/%", "/api/v1/certificates/" + domain}}, []string{"a3", "a1"}},
		{"app exact or child, underscore literal", AuditEntryFilter{PathLike: []string{"/api/v1/apps/" + app, "/api/v1/apps/" + app + "/%"}}, []string{"a4", "a2", "a1"}},
		{"action prefixes", AuditEntryFilter{ActionPrefixes: []string{"dns_record.", "domain."}}, []string{"a6", "a4"}},
		{"underscore in prefix is literal", AuditEntryFilter{ActionPrefixes: []string{"proxy_"}}, []string{"a5"}},
		{"both narrow", AuditEntryFilter{PathLike: []string{"/api/v1/apps/" + app}, ActionPrefixes: []string{"dns_record."}}, []string{"a4"}},
		{"empty patterns ignored", AuditEntryFilter{PathLike: []string{""}}, []string{"a6", "a5", "a4", "a3", "a2", "a1"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := db.ListAuditEntries(ctx, 50, nil, tt.filter)
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
