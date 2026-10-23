package store

import (
	"context"
	"errors"
	"testing"
)

func TestTrafficExportSinksCRUD(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	a := TrafficExportSink{ID: "tex_a", Name: "Analytics", Kind: TrafficSinkPostHog, Enabled: true, Host: "eu", TargetHost: "eu.i.posthog.com", IPTruncate: true, Sample: 0.5}
	b := TrafficExportSink{ID: "tex_b", Name: "Archive", Kind: TrafficSinkWebhook, Enabled: false, Fields: []string{"client_ip", "user_agent"}, IPTruncate: false, Sample: 1}
	for _, s := range []TrafficExportSink{b, a} {
		if err := db.UpsertTrafficExportSink(ctx, s); err != nil {
			t.Fatalf("upsert %s: %v", s.ID, err)
		}
	}

	list, err := db.ListTrafficExportSinks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[0].ID != "tex_a" || list[1].ID != "tex_b" {
		t.Fatalf("list = %+v, want ordered by name", list)
	}
	if len(list[0].Fields) != 0 || list[0].Sample != 0.5 || !list[0].IPTruncate || list[0].Host != "eu" {
		t.Errorf("a = %+v", list[0])
	}
	if len(list[1].Fields) != 2 || list[1].Enabled || list[1].IPTruncate {
		t.Errorf("b = %+v", list[1])
	}

	a.Name, a.Enabled = "Analytics 2", false
	if err := db.UpsertTrafficExportSink(ctx, a); err != nil {
		t.Fatal(err)
	}
	got, err := db.GetTrafficExportSink(ctx, "tex_a")
	if err != nil || got.Name != "Analytics 2" || got.Enabled || got.CreatedAt.IsZero() {
		t.Fatalf("get = %+v, %v", got, err)
	}

	if err := db.DeleteTrafficExportSink(ctx, "tex_a"); err != nil {
		t.Fatal(err)
	}
	if _, err := db.GetTrafficExportSink(ctx, "tex_a"); !errors.Is(err, ErrTrafficExportSinkNotFound) {
		t.Errorf("get after delete = %v", err)
	}
	if err := db.DeleteTrafficExportSink(ctx, "tex_a"); !errors.Is(err, ErrTrafficExportSinkNotFound) {
		t.Errorf("second delete = %v", err)
	}
}

func TestTrafficExportSinkValidation(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()
	if err := db.UpsertTrafficExportSink(ctx, TrafficExportSink{ID: "tex_a", Name: "One", Kind: TrafficSinkOTLP, Sample: 1}); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		s    TrafficExportSink
	}{
		{"duplicate name", TrafficExportSink{ID: "tex_b", Name: "One", Kind: TrafficSinkOTLP, Sample: 1}},
		{"blank name", TrafficExportSink{ID: "tex_c", Name: "  ", Kind: TrafficSinkOTLP, Sample: 1}},
		{"missing id", TrafficExportSink{Name: "Two", Kind: TrafficSinkOTLP, Sample: 1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := db.UpsertTrafficExportSink(ctx, tt.s); !errors.Is(err, ErrTrafficExportSinkInvalid) {
				t.Errorf("err = %v, want ErrTrafficExportSinkInvalid", err)
			}
		})
	}
	if err := db.UpsertTrafficExportSink(ctx, TrafficExportSink{ID: "tex_d", Name: "Four", Kind: "carrier", Sample: 1}); err == nil {
		t.Error("an unknown kind must be rejected by the table")
	}
}
