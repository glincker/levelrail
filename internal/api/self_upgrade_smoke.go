package api

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// Smoke tour item ids and results, in the order the Updates page shows them.
const (
	smokeApps     = "apps"
	smokeDomains  = "domains"
	smokeCerts    = "certificates"
	smokeBackups  = "backups"
	smokePass     = "pass"
	smokeWarn     = "warn"
	smokeFail     = "fail"
	smokeSkipped  = "skipped"
	smokeRunLimit = 30 * time.Second
)

type selfUpgradeSmokeItem struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

type selfUpgradeSmokeResource struct {
	// OK is false only when an item failed; warnings and skipped items do not fail the tour.
	OK        bool                   `json:"ok"`
	CheckedAt time.Time              `json:"checked_at"`
	Items     []selfUpgradeSmokeItem `json:"items"`
}

// handleSelfUpgradeSmoke handles GET /api/v1/updates/self-upgrade/smoke: the
// fixed post-upgrade health tour (apps up, domains resolve, certs valid,
// backups ok), one pass or fail row per item. It reuses the doctor checks
// so the tour and Doctor never disagree.
func (rt *Router) handleSelfUpgradeSmoke(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), smokeRunLimit)
	defer cancel()
	items := []selfUpgradeSmokeItem{
		rt.smokeApps(ctx, r),
		smokeFromDoctor(smokeDomains, rt.doctorCheckDomainDNS(ctx)),
		smokeFromDoctor(smokeCerts, rt.doctorCheckCertExpiry(ctx)),
		smokeFromDoctor(smokeBackups, rt.doctorCheckControlPlaneBackup(), rt.doctorCheckBackupFreshness(ctx)),
	}
	out := selfUpgradeSmokeResource{OK: true, CheckedAt: time.Now().UTC(), Items: items}
	for _, it := range items {
		if it.Status == smokeFail {
			out.OK = false
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// smokeApps fails when any app the caller can see is in the failing bucket.
func (rt *Router) smokeApps(ctx context.Context, r *http.Request) selfUpgradeSmokeItem {
	fs, ok := rt.apps.(appFilterStore)
	if !ok {
		return selfUpgradeSmokeItem{ID: smokeApps, Status: smokeSkipped, Message: "app status is not available"}
	}
	canRead, _, err := rt.callerAppVisibility(r)
	if err != nil {
		return selfUpgradeSmokeItem{ID: smokeApps, Status: smokeSkipped, Message: "could not resolve app access: " + err.Error()}
	}
	f := parseAppListFilter(r)
	f.Limit, f.Offset = 0, 0
	svcs, err := listAppHeads(ctx, fs, f)
	if err != nil {
		return selfUpgradeSmokeItem{ID: smokeApps, Status: smokeSkipped, Message: "could not list apps: " + err.Error()}
	}
	visible := svcs[:0:0]
	for _, s := range svcs {
		if canRead(s.Name) {
			visible = append(visible, s)
		}
	}
	buckets, err := rt.appStatusBuckets(ctx, visible)
	if err != nil {
		return selfUpgradeSmokeItem{ID: smokeApps, Status: smokeSkipped, Message: "could not read app status: " + err.Error()}
	}
	failing, running := 0, 0
	for _, s := range visible {
		switch buckets[s.Name] {
		case appBucketFailing:
			failing++
		case appBucketRunning:
			running++
		}
	}
	if failing > 0 {
		return selfUpgradeSmokeItem{ID: smokeApps, Status: smokeFail, Message: fmt.Sprintf("%d of %d app(s) are failing", failing, len(visible))}
	}
	return selfUpgradeSmokeItem{ID: smokeApps, Status: smokePass, Message: fmt.Sprintf("%d of %d app(s) running", running, len(visible))}
}

// smokeFromDoctor folds one or more doctor checks into a single tour row,
// keeping the worst result and its message.
func smokeFromDoctor(id string, checks ...doctorCheckResource) selfUpgradeSmokeItem {
	worst := selfUpgradeSmokeItem{ID: id, Status: smokeSkipped}
	rank := map[string]int{smokeSkipped: 0, smokePass: 1, smokeWarn: 2, smokeFail: 3}
	for _, c := range checks {
		cur := selfUpgradeSmokeItem{ID: id, Status: smokeStatusOf(c.Status), Message: c.Message}
		if rank[cur.Status] > rank[worst.Status] {
			worst = cur
		}
	}
	return worst
}

func smokeStatusOf(doctorStatus string) string {
	switch doctorStatus {
	case doctorStatusOK:
		return smokePass
	case doctorStatusWarn:
		return smokeWarn
	case doctorStatusFail:
		return smokeFail
	default:
		return smokeSkipped
	}
}
