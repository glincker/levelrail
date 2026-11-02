package api

import (
	"context"
	"errors"
	"testing"

	"github.com/GLINCKER/levelrail/internal/fixit"
)

func TestPoolerCandidatesWithoutDatabasesStore(t *testing.T) {
	rt := &Router{}
	got, err := rt.poolerCandidates(context.Background())
	if err != nil || len(got) != 0 {
		t.Fatalf("no store must yield no candidates: %v %v", got, err)
	}
}

func TestEnablePoolerForFixUnsupportedStore(t *testing.T) {
	rt := &Router{}
	if err := rt.enablePoolerForFix(context.Background(), "orders"); !errors.Is(err, fixit.ErrCannotFixHere) {
		t.Fatalf("a store without pooler support must report cannot fix here, got %v", err)
	}
}
