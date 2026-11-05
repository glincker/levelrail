package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/certflow"
)

type certBudgetResource struct {
	// Issued is how many certificates the CA issued in the window, as far as this process has seen.
	Issued     int       `json:"issued"`
	Warn       int       `json:"warn"`
	WindowDays int       `json:"window_days"`
	Over       bool      `json:"over"`
	CheckedAt  time.Time `json:"checked_at"`
}

// certBudget refreshes the ledger from the stored certificates, then counts.
func (rt *Router) certBudget(r *http.Request) (certBudgetResource, error) {
	now := time.Now()
	if _, err := rt.certTruthRows(r.Context(), now); err != nil {
		return certBudgetResource{}, fmt.Errorf("read certificates: %w", err)
	}
	issued := rt.certLedger.IssuedSince(now, certflow.BudgetWindow)
	warn := rt.certWeeklyIssueWarn(r.Context())
	return certBudgetResource{
		Issued: issued, Warn: warn, WindowDays: int(certflow.BudgetWindow / (24 * time.Hour)),
		Over: issued > warn, CheckedAt: now.UTC(),
	}, nil
}

// handleGetCertBudget handles GET /api/v1/certificates/budget.
func (rt *Router) handleGetCertBudget(w http.ResponseWriter, r *http.Request) {
	b, err := rt.certBudget(r)
	if err != nil {
		rt.internalError(w, "api: certificate budget failed", err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}
