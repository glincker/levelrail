package api

import (
	"context"
	"fmt"
	"time"

	"github.com/GLINCKER/levelrail/internal/certflow"
)

func (rt *Router) doctorCheckCertBudget(ctx context.Context) doctorCheckResource {
	const code, name = "certificate_budget", "Certificate issuance budget"
	now := time.Now()
	if _, err := rt.certTruthRows(ctx, now); err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: fmt.Sprintf("could not read certificates: %s", err)}
	}
	issued := rt.certLedger.IssuedSince(now, certflow.BudgetWindow)
	warn := rt.certWeeklyIssueWarn(ctx)
	if issued <= warn {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: fmt.Sprintf("%d certificates issued in the last 7 days (warning at %d)", issued, warn)}
	}
	return doctorCheckResource{
		Code: code, Name: name, Status: doctorStatusWarn,
		Message: fmt.Sprintf("%d certificates issued in the last 7 days, above the warning of %d. Opt apps out of automatic hostnames to save the shared budget", issued, warn),
		Fix:     rt.cliName() + " apps auto-host <app> --off",
	}
}
