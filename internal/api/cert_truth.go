package api

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/alerting"
	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/certflow"
	"github.com/GLINCKER/levelrail/internal/fixit"
)

// certTruthRow is one stored certificate with everything the doctor, the
// watchdog and the list need to judge it the same way.
type certTruthRow struct {
	Info     alerting.CertInfo
	Kind     certflow.Kind
	Source   string
	Owner    string
	Renewal  string
	Failure  *acmeFailureResource
	Routed   bool
	DaysLeft int
}

// RenewalFailed is true when the CA refused the last attempt for a certificate
// the platform renews itself.
func (c certTruthRow) RenewalFailed() bool { return c.Failure != nil && c.Kind.AutoRenewed() }

// NearExpiry is true for an auto-renewed certificate inside the failure window.
func (c certTruthRow) NearExpiry() bool {
	return c.Kind.AutoRenewed() && c.DaysLeft < certflow.FailDays()
}

// NeedsRenewal is true for an auto-renewed certificate that expired, stalled,
// failed its last renewal or is inside the failure window.
func (c certTruthRow) NeedsRenewal() bool {
	if !c.Routed || !c.Kind.AutoRenewed() {
		return false
	}
	return c.Info.Status == "expired" || c.Renewal == "stalled" || c.RenewalFailed() || c.NearExpiry()
}

func (rt *Router) certTruthRows(ctx context.Context, now time.Time) ([]certTruthRow, error) {
	infos, err := alerting.ListCertificates(ctx, rt.certs, rt.certWarningWindow(), now, rt.logger)
	if err != nil {
		return nil, fmt.Errorf("list certificates: %w", err)
	}
	var obs []alerting.CertExpiryObservation
	if rt.certObservations != nil {
		obs, _ = rt.certObservations.ListAllCertExpiryObservations(ctx)
	}
	renewal := alerting.CertRenewalStates(infos, obs, rt.certRenewalStalledThreshold, now)
	owners, _ := rt.domainOwners(ctx)
	custom, _ := rt.customTLSCertDomains(ctx)
	snap := rt.tlsSnapshot(ctx)
	out := make([]certTruthRow, 0, len(infos))
	for _, in := range infos {
		if snap.Suppressed(in.Domain) {
			continue
		}
		source := "acme"
		if custom[strings.ToLower(in.Domain)] {
			source = "custom"
		}
		owner := string(snap.Owner(in.Domain))
		kind := certflow.Classify(in.Issuer, source, owner)
		out = append(out, certTruthRow{
			Info: in, Kind: kind, Source: source, Owner: owner, Renewal: renewal[in.Domain],
			Failure:  acmeFailureIn(snap, in.Domain),
			Routed:   len(owners[strings.ToLower(in.Domain)]) > 0 || strings.EqualFold(in.Domain, rt.publicHost),
			DaysLeft: certflow.DaysLeft(in.NotAfter, now),
		})
		rt.observeCertHistory(out[len(out)-1], now)
	}
	return out, nil
}

// observeCertHistory feeds the rate-limit ledger from what the CA reported.
func (rt *Router) observeCertHistory(c certTruthRow, now time.Time) {
	d := strings.ToLower(c.Info.Domain)
	if c.Failure != nil && c.Kind != certflow.KindStaging {
		rt.certLedger.RecordFailure(d, c.Failure.At, now)
	}
	if c.Kind == certflow.KindLetsEncrypt && !c.Info.NotBefore.IsZero() {
		rt.certLedger.RecordIssued(d, c.Info.NotBefore, now)
	}
}

// certBadgeFor is the per-domain truth badge for one stored certificate.
func (rt *Router) certBadgeFor(c certTruthRow) certflow.Badge {
	b := certflow.Badge{Kind: c.Kind, DaysLeft: c.DaysLeft}
	var at time.Time
	var msg string
	if c.Failure != nil {
		at, msg = c.Failure.At, c.Failure.Error
	}
	b.LastRenewal = certflow.BuildRenewal(c.Kind, c.Info.NotBefore, at, msg, c.RenewalFailed())
	if rt.certWatchdog != nil {
		if s, ok := rt.certWatchdog.State(c.Info.Domain); ok && !s.NextAt.IsZero() {
			next := s.NextAt
			b.NextRetryAt = &next
		}
	}
	return b
}

// watchdog returns the process-wide renewal watchdog, built on first use.
func (rt *Router) watchdog() *certflow.Watchdog {
	rt.certWatchdogOnce.Do(func() {
		rt.certWatchdog = certflow.NewWatchdog(certflow.WatchdogDeps{
			Scan:  rt.watchdogScan,
			Renew: rt.watchdogRenew,
			Log:   rt.logger,
		})
	})
	return rt.certWatchdog
}

// RunCertWatchdog retries stuck or failing certificate renewals with backoff until ctx ends.
func (rt *Router) RunCertWatchdog(ctx context.Context) {
	rt.watchdog().Run(ctx)
}

func (rt *Router) watchdogScan(ctx context.Context) ([]certflow.Candidate, error) {
	now := time.Now()
	rows, err := rt.certTruthRows(ctx, now)
	if err != nil {
		return nil, err
	}
	var out []certflow.Candidate
	for _, c := range rows {
		if !c.NeedsRenewal() {
			continue
		}
		snap := rt.certLedger.Snapshot(strings.ToLower(c.Info.Domain))
		var errText string
		var errAt time.Time
		if c.Failure != nil {
			errText, errAt = c.Failure.Error, c.Failure.At
		}
		blocked := snap.Check(certflow.LimitsFromEnv(), now, errText, errAt).Blocked
		out = append(out, certflow.Candidate{
			Domain: c.Info.Domain, Reason: certReason(c), Expired: c.Info.Status == "expired" || c.Renewal == "stalled", Blocked: blocked,
		})
	}
	return out, nil
}

func certReason(c certTruthRow) string {
	switch {
	case c.Info.Status == "expired":
		return "expired"
	case c.Renewal == "stalled":
		return "stalled"
	case c.RenewalFailed():
		return "renewal_failed"
	}
	return "expiring"
}

// watchdogRenew reuses the one renewal path: clear the stored certificate when
// it is already unusable, then nudge the reconciler so ingress re-issues.
func (rt *Router) watchdogRenew(ctx context.Context, c certflow.Candidate) error {
	if c.Expired {
		if _, err := alerting.DeleteCertificateForDomain(ctx, rt.certs, rt.certs, c.Domain); err != nil {
			return fmt.Errorf("clear certificate for %s: %w", c.Domain, err)
		}
	}
	rt.nudgeReconciler()
	return nil
}

const certsRenewFixID = "certs.renew"

// certRenewFix is the typed 'Renew now' fix behind the certificate doctor check.
func (rt *Router) certRenewFix() fixit.Fix {
	failing := func(ctx context.Context) ([]certTruthRow, error) {
		rows, err := rt.certTruthRows(ctx, time.Now())
		if err != nil {
			return nil, err
		}
		var out []certTruthRow
		for _, c := range rows {
			if c.NeedsRenewal() {
				out = append(out, c)
			}
		}
		return out, nil
	}
	return fixit.Fix{
		ID: certsRenewFixID, CheckID: "certificates", Subject: "certificates", Kind: fixit.KindAuto,
		Title: "Renew certificates now",
		Why:   "A certificate that is close to expiry or failed its last renewal turns into a browser warning for every visitor.",
		Risk:  fixit.RiskSafe, Reversible: false, Needs: "root",
		Detect: func(ctx context.Context) (fixit.Applicability, error) {
			rows, err := failing(ctx)
			if err != nil {
				return fixit.Applicability{}, err
			}
			if len(rows) == 0 {
				return fixit.Applicability{Detail: "no certificate needs renewal", Changes: []fixit.Change{}}, nil
			}
			names := make([]string, 0, len(rows))
			changes := make([]fixit.Change, 0, len(rows))
			for _, c := range rows {
				names = append(names, c.Info.Domain)
				changes = append(changes, fixit.Change{Label: "Request a new certificate for " + c.Info.Domain, Detail: certReason(c)})
			}
			return fixit.Applicability{
				Failing: true, Detail: strings.Join(names, ", "), Signature: strings.Join(names, ","), Changes: changes,
			}, nil
		},
		Run: func(ctx context.Context, _ fixit.Input) error {
			rows, err := failing(ctx)
			if err != nil {
				return err
			}
			for _, c := range rows {
				if _, err := alerting.DeleteCertificateForDomain(ctx, rt.certs, rt.certs, c.Info.Domain); err != nil {
					return fmt.Errorf("clear certificate for %s: %w", c.Info.Domain, err)
				}
				rt.logger.Info("api: certificate renewal requested by fix", slog.String("domain", c.Info.Domain))
			}
			rt.nudgeReconciler()
			return nil
		},
	}
}

// certDoctorFacts feeds attention.CertFacts the failure signals this package adds.
func certDoctorFacts(c certTruthRow) attention.CertFacts {
	return attention.CertFacts{
		Issuer: c.Info.Issuer, Source: c.Source, TLSOwner: c.Owner, Status: c.Info.Status, Renewal: c.Renewal,
		NotBefore: c.Info.NotBefore, NotAfter: c.Info.NotAfter, Routed: c.Routed,
		RenewalFailed: c.RenewalFailed(), NearExpiry: c.NearExpiry(),
	}
}
