package store

import (
	"context"
	"errors"
	"testing"
)

func newTestAppStream(t *testing.T, db *DB, serviceName string) AppStream {
	t.Helper()
	ctx := context.Background()
	if err := db.SaveDesiredService(ctx, DesiredService{Name: serviceName, Image: "postgres:16", Port: 5432}); err != nil {
		t.Fatalf("SaveDesiredService(%q) error = %v", serviceName, err)
	}
	return AppStream{
		ID:            "stream_test1",
		ServiceName:   serviceName,
		ContainerPort: 5432,
		HostPort:      15432,
		Protocol:      AppStreamProtocolTCP,
		CreatedAt:     "2026-10-01T00:00:00Z",
	}
}

func TestSaveAndGetAppStream(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestAppStream(t, db, "pg")
	if err := db.SaveAppStream(ctx, want); err != nil {
		t.Fatalf("SaveAppStream() error = %v", err)
	}

	got, err := db.GetAppStream(ctx, want.ID)
	if err != nil {
		t.Fatalf("GetAppStream() error = %v", err)
	}
	if got != want {
		t.Errorf("GetAppStream() = %+v, want %+v", got, want)
	}
}

func TestGetAppStream_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	_, err := db.GetAppStream(ctx, "missing")
	if !errors.Is(err, ErrAppStreamNotFound) {
		t.Fatalf("GetAppStream() error = %v, want ErrAppStreamNotFound", err)
	}
}

func TestListAppStreamsForService_OldestFirst(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "pg", Image: "postgres:16", Port: 5432}); err != nil {
		t.Fatalf("SaveDesiredService(pg) error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "redis", Image: "redis:7", Port: 6379}); err != nil {
		t.Fatalf("SaveDesiredService(redis) error = %v", err)
	}

	second := AppStream{ID: "stream_b", ServiceName: "redis", ContainerPort: 6379, HostPort: 16379, Protocol: AppStreamProtocolTCP, CreatedAt: "2026-10-02T00:00:00Z"}
	first := AppStream{ID: "stream_a", ServiceName: "pg", ContainerPort: 5432, HostPort: 15432, Protocol: AppStreamProtocolTCP, CreatedAt: "2026-10-01T00:00:00Z"}
	if err := db.SaveAppStream(ctx, second); err != nil {
		t.Fatalf("SaveAppStream(second) error = %v", err)
	}
	if err := db.SaveAppStream(ctx, first); err != nil {
		t.Fatalf("SaveAppStream(first) error = %v", err)
	}

	got, err := db.ListAppStreamsForService(ctx, "pg")
	if err != nil {
		t.Fatalf("ListAppStreamsForService() error = %v", err)
	}
	if len(got) != 1 || got[0].ID != "stream_a" {
		t.Fatalf("ListAppStreamsForService(pg) = %+v, want [stream_a]", got)
	}

	all, err := db.ListAllAppStreams(ctx)
	if err != nil {
		t.Fatalf("ListAllAppStreams() error = %v", err)
	}
	if len(all) != 2 || all[0].ID != "stream_a" || all[1].ID != "stream_b" {
		t.Fatalf("ListAllAppStreams() = %+v, want [stream_a, stream_b]", all)
	}
}

func TestDeleteAppStream(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	want := newTestAppStream(t, db, "pg")
	if err := db.SaveAppStream(ctx, want); err != nil {
		t.Fatalf("SaveAppStream() error = %v", err)
	}
	if err := db.DeleteAppStream(ctx, want.ID); err != nil {
		t.Fatalf("DeleteAppStream() error = %v", err)
	}
	if _, err := db.GetAppStream(ctx, want.ID); !errors.Is(err, ErrAppStreamNotFound) {
		t.Fatalf("GetAppStream() after delete error = %v, want ErrAppStreamNotFound", err)
	}
}

func TestDeleteAppStream_NotFound(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	err := db.DeleteAppStream(ctx, "missing")
	if !errors.Is(err, ErrAppStreamNotFound) {
		t.Fatalf("DeleteAppStream() error = %v, want ErrAppStreamNotFound", err)
	}
}

// TestSaveAppStream_DuplicateHostPortRejected confirms migrations/0279's
// UNIQUE(host_port) constraint actually rejects a second stream (on any
// service) claiming a host port already in use, defense-in-depth below
// internal/api's own pre-check (app_streams.go's handleCreateAppStream).
func TestSaveAppStream_DuplicateHostPortRejected(t *testing.T) {
	db := openTestDB(t)
	ctx := context.Background()

	if err := db.SaveDesiredService(ctx, DesiredService{Name: "pg", Image: "postgres:16", Port: 5432}); err != nil {
		t.Fatalf("SaveDesiredService(pg) error = %v", err)
	}
	if err := db.SaveDesiredService(ctx, DesiredService{Name: "redis", Image: "redis:7", Port: 6379}); err != nil {
		t.Fatalf("SaveDesiredService(redis) error = %v", err)
	}

	first := AppStream{ID: "stream_a", ServiceName: "pg", ContainerPort: 5432, HostPort: 15432, Protocol: AppStreamProtocolTCP, CreatedAt: "2026-10-01T00:00:00Z"}
	if err := db.SaveAppStream(ctx, first); err != nil {
		t.Fatalf("SaveAppStream(first) error = %v", err)
	}

	second := AppStream{ID: "stream_b", ServiceName: "redis", ContainerPort: 6379, HostPort: 15432, Protocol: AppStreamProtocolTCP, CreatedAt: "2026-10-02T00:00:00Z"}
	if err := db.SaveAppStream(ctx, second); err == nil {
		t.Fatalf("SaveAppStream(second) with duplicate host port: want an error, got nil")
	}
}
