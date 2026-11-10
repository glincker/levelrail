package api

import (
	"context"
	"testing"
)

func TestDatabaseViewerLimitsEnvSeedsAndDefaults(t *testing.T) {
	rt := &Router{}
	if got := rt.databaseViewerLimits(context.Background()); got.MaxRows != 1000 || got.SQLBytes != 65536 {
		t.Fatalf("defaults = %+v", got)
	}
	t.Setenv("APP_DB_QUERY_MAX_ROWS", "7")
	if got := rt.databaseViewerLimits(context.Background()); got.MaxRows != 7 {
		t.Fatalf("env seed MaxRows = %d, want 7", got.MaxRows)
	}
}
