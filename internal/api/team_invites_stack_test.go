package api

import (
	"net/http"
	"testing"

	"github.com/GLINCKER/levelrail/internal/store"
)

func TestTeamInviteAcceptRefusesStackToken(t *testing.T) {
	rt, _, cookie, stackTok, _ := stackTokenFixture(t, []string{store.StackVerbPlan}, []string{AbilityRead})
	if _, ok := rt.acceptingUserID(bearerRequest(http.MethodPost, "/api/v1/team-invites/accept", `{}`, stackTok)); ok {
		t.Fatal("a stack-scoped token was accepted as its owner's identity")
	}
	var plain createTokenResponse
	if code := iacCall(t, rt, authedRequest(t, cookie, http.MethodPost, "/api/v1/auth/tokens", `{"name":"cli","abilities":["read"]}`), &plain); code != http.StatusCreated {
		t.Fatalf("mint = %d", code)
	}
	if _, ok := rt.acceptingUserID(bearerRequest(http.MethodPost, "/api/v1/team-invites/accept", `{}`, plain.Token)); !ok {
		t.Fatal("an ordinary owned token should still identify its owner")
	}
}
