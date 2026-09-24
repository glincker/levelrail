package alerting

import (
	"context"
	"fmt"
	"time"
)

// Certificate renewal states reported by GET /api/v1/certificates.
const (
	CertRenewalOK      = "ok"
	CertRenewalStalled = "stalled"
)

// ListAllCertExpiryObservations returns every observation row across all
// rules, so a reader with no rule in hand can see episodes any
// kind=cert_expiry rule has recorded.
func (db *DB) ListAllCertExpiryObservations(ctx context.Context) ([]CertExpiryObservation, error) {
	rows, err := db.QueryContext(ctx, `SELECT DISTINCT rule_id FROM cert_expiry_observations`)
	if err != nil {
		return nil, fmt.Errorf("alerting: list cert expiry observation rules: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return nil, fmt.Errorf("alerting: scan cert expiry observation rule id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return nil, fmt.Errorf("alerting: iterate cert expiry observation rule ids: %w", err)
	}
	_ = rows.Close()

	var out []CertExpiryObservation
	for _, id := range ids {
		obs, err := db.ListCertExpiryObservations(ctx, id)
		if err != nil {
			return nil, err
		}
		out = append(out, obs...)
	}
	return out, nil
}

// CertRenewalStates maps each certificate's domain to CertRenewalOK or
// CertRenewalStalled. An expired certificate is always stalled (a renewal
// would have replaced it). An expiring_soon one is stalled once an
// observation shows the same NotAfter unchanged for at least threshold,
// the same test EvaluateCertExpiry applies.
func CertRenewalStates(infos []CertInfo, obs []CertExpiryObservation, threshold time.Duration, now time.Time) map[string]string {
	if threshold <= 0 {
		threshold = DefaultCertRenewalStalledThreshold
	}
	out := make(map[string]string, len(infos))
	for _, info := range infos {
		out[info.Domain] = CertRenewalOK
		switch info.Status {
		case "expired":
			out[info.Domain] = CertRenewalStalled
		case "expiring_soon":
			for _, o := range obs {
				if o.Domain == info.Domain && o.Status != "healthy" &&
					o.EpisodeNotAfter.Equal(info.NotAfter) && now.Sub(o.EpisodeStartedAt) >= threshold {
					out[info.Domain] = CertRenewalStalled
					break
				}
			}
		}
	}
	return out
}
