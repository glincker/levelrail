package api

import (
	"net/http"
	"testing"
)

func TestStackForgetNeedsAdmin(t *testing.T) {
	rt, db, _ := stackFixture(t)
	writer := seedMatrixToken(t, db, "writer", []string{AbilityRead, AbilityWrite})
	if code := callWithToken(t, rt, writer, http.MethodDelete, "/api/v1/stacks/shop-prod?mode=forget", ""); code != http.StatusForbidden {
		t.Fatalf("forget with a write token = %d, want 403", code)
	}
	if _, err := db.GetStackByName(t.Context(), "shop-prod"); err != nil {
		t.Fatalf("stack gone: %v", err)
	}
}
