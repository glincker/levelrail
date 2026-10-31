package store

import (
	"context"
	"errors"
	"testing"
)

func TestCreateMCPOAuthGrantNeedsClient(t *testing.T) {
	db := openTestDB(t)
	g := MCPOAuthGrant{ID: "g1", ClientID: "gone", UserID: "u1", Resource: "https://x/mcp", RequestID: "r1", ExpiresAt: 99}
	err := db.CreateMCPOAuthGrant(context.Background(), g, "tok1", "hash1", 99, 1)
	if !errors.Is(err, ErrMCPOAuthNotFound) {
		t.Fatalf("grant for missing client: %v", err)
	}
}
