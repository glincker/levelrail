package api

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// tokensHygieneCounts is what the tokens_hygiene check measures.
type tokensHygieneCounts struct {
	liveEmergency, expiredPastRetention, expiringSoon int
}

// countTokensHygiene counts live emergency tokens, expired tokens older than the
// retention window (0 means kept forever, so none) and live tokens expiring inside window.
func countTokensHygiene(recs []store.APIToken, now time.Time, retentionDays int, window time.Duration) tokensHygieneCounts {
	var c tokensHygieneCounts
	cutoff := now.Add(-time.Duration(retentionDays) * hygieneDay)
	for _, t := range recs {
		switch {
		case t.RevokedAt != nil:
		case tokenLive(t, now):
			if t.Name == emergencyTokenName {
				c.liveEmergency++
			}
			if t.ExpiresAt != nil && t.ExpiresAt.Before(now.Add(window)) {
				c.expiringSoon++
			}
		case retentionDays > 0 && t.ExpiresAt != nil && t.ExpiresAt.Before(cutoff):
			c.expiredPastRetention++
		}
	}
	return c
}

// doctorCheckTokensHygiene warns about too many live emergency tokens, expired
// tokens kept past retention and tokens about to expire. It never changes anything.
func (rt *Router) doctorCheckTokensHygiene(ctx context.Context) doctorCheckResource {
	const code, name = "tokens_hygiene", "API token list"
	recs, err := rt.liveTokenRecords(ctx, "")
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read tokens: %s", err)}
	}
	maxLive := rt.tokenSettingInt(ctx, runtimesettings.KeyTokensEmergencyMaxLive)
	retention := rt.tokenSettingInt(ctx, runtimesettings.KeyExpiredTokenDays)
	c := countTokensHygiene(recs, time.Now(), retention, rt.tokenExpiryWindow(ctx))
	var problems, fixes []string
	cli := rt.cliName()
	if maxLive > 0 && c.liveEmergency > maxLive {
		problems = append(problems, fmt.Sprintf("%d live emergency tokens, more than the %d allowed by tokens.emergency_max_live", c.liveEmergency, maxLive))
		fixes = append(fixes, cli+" tokens revoke --kind emergency --dry-run")
	}
	if c.expiredPastRetention > 0 {
		problems = append(problems, fmt.Sprintf("%d expired tokens are older than the %d day retention window", c.expiredPastRetention, retention))
		fixes = append(fixes, cli+" tokens prune --older-than "+fmt.Sprintf("%dd", retention)+" --dry-run")
	}
	if c.expiringSoon > 0 {
		problems = append(problems, fmt.Sprintf("%d tokens expire soon", c.expiringSoon))
		fixes = append(fixes, cli+" tokens ls --status expiring")
	}
	if len(problems) == 0 {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "no excess emergency tokens, no stale expired tokens and none expiring soon"}
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: strings.Join(problems, "; ") + ". Nothing is revoked or deleted automatically",
		Fix:     strings.Join(fixes, ", then "),
	}
}
