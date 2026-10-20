package api

import (
	"net/http"
	"time"
)

type devicePendingSummaryItem struct {
	ClientName  string    `json:"client_name"`
	RequesterIP string    `json:"requester_ip"`
	UserAgent   string    `json:"user_agent"`
	CreatedAt   time.Time `json:"created_at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

type devicePendingSummaryResponse struct {
	Pending []devicePendingSummaryItem `json:"pending"`
}

// Read-only view of waiting CLI logins. It never carries a user or device code, and approve and deny stay session-only.
func (rt *Router) handleDevicePendingSummary(w http.ResponseWriter, r *http.Request) {
	pending, err := rt.authLib.device.PendingDevices(r.Context())
	if err != nil {
		rt.internalError(w, "api: device pending summary failed", err)
		return
	}
	out := devicePendingSummaryResponse{Pending: make([]devicePendingSummaryItem, 0, len(pending))}
	for _, p := range pending {
		out.Pending = append(out.Pending, devicePendingSummaryItem{
			ClientName: p.ClientName, RequesterIP: p.RequesterIP, UserAgent: p.UserAgent,
			CreatedAt: p.CreatedAt, ExpiresAt: p.ExpiresAt,
		})
	}
	writeJSON(w, http.StatusOK, out)
}
