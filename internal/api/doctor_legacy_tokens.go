package api

import (
	"context"
	"fmt"
)

// doctorCheckLegacyTokens reports the upgrade import of API tokens minted
// before the auth library. Pending tokens still import on first use.
func (rt *Router) doctorCheckLegacyTokens(ctx context.Context) doctorCheckResource {
	const code, name = "legacy_tokens", "Pre-upgrade API tokens"
	if rt.authLib.tokens == nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "auth library not configured"}
	}
	sum, err := rt.authLib.tokens.LegacyImportStatus(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read token import status: %s", err)}
	}
	msg := fmt.Sprintf("%d legacy token(s) migrated, %d skipped (expired, revoked or orphaned)", sum.Migrated, sum.Skipped)
	if sum.Pending > 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, Message: fmt.Sprintf("%s, %d not yet migrated (they migrate on next start or first use)", msg, sum.Pending)}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: msg}
}
