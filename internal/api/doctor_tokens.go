package api

import (
	"context"
	"fmt"
	"time"
)

// doctorCheckTokenHygiene warns about idle tokens and about tokens that never
// expire yet can change things. It never revokes anything.
func (rt *Router) doctorCheckTokenHygiene(ctx context.Context) doctorCheckResource {
	const code, name = "token_hygiene", "API token hygiene"
	recs, err := rt.liveTokenRecords(ctx, "")
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read tokens: %s", err)}
	}
	now := time.Now()
	unused := len(unusedTokens(recs, rt.tokenUnusedAlertDays(ctx), now))
	noExpiry, ownerless := 0, 0
	strict := rt.strictOrgIsolation(ctx)
	for _, t := range recs {
		if strict && tokenLive(t, now) && t.OwnerUserID == "" && !hasAbility(t.Abilities, AbilityRoot) && !rt.tokenHasTeamScope(ctx, t.ID) {
			ownerless++
		}
		// Ownerless tokens are platform-internal and not the operator's to rotate.
		if tokenLive(t, now) && t.ExpiresAt == nil && t.OwnerUserID != "" && holdsSensitiveAbility(t.Abilities) {
			noExpiry++
		}
	}
	if ownerless > 0 {
		return doctorCheckResource{
			Code: code, Name: name, Status: doctorStatusWarn,
			Message: fmt.Sprintf("%d tokens have no owner and no organization limit, so strict organization isolation limits them to the default organization. %d unused tokens, %d tokens without expiry that can change or reveal data", ownerless, unused, noExpiry),
			Fix:     rt.cliName() + " tokens list, then revoke or re-mint them as a user's token, or with an organization limit when they belong to another organization",
		}
	}
	if unused == 0 && noExpiry == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "no unused tokens and no never-expiring tokens with write or sensitive access"}
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("%d unused tokens, %d tokens without expiry that can change or reveal data. Nothing is revoked automatically", unused, noExpiry),
		Fix:     rt.cliName() + " tokens unused, then " + rt.cliName() + " tokens revoke <id>",
	}
}

// tokenHasTeamScope is true when a token carries a stored org limit or the
// system marker; a failed lookup counts as scoped so the warning stays quiet.
func (rt *Router) tokenHasTeamScope(ctx context.Context, tokenID string) bool {
	db := rt.teamDB()
	if db == nil {
		return false
	}
	s, err := db.GetAPITokenScope(ctx, tokenID)
	return err != nil || s != nil
}
