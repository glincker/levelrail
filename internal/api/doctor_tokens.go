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
	noExpiry := 0
	for _, t := range recs {
		// Ownerless tokens are platform-internal and not the operator's to rotate.
		if tokenLive(t, now) && t.ExpiresAt == nil && t.OwnerUserID != "" && holdsSensitiveAbility(t.Abilities) {
			noExpiry++
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
