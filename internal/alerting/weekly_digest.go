package alerting

import (
	"context"
	"fmt"
	"html"
	"log/slog"
	"sort"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

// WeeklyFacts is one week of platform activity, gathered from data the control plane already keeps.
type WeeklyFacts struct {
	Since, Until     time.Time
	Apps             int
	Deploys          int
	DeploysFailed    int
	Restarts         []NamedCount
	CertsExpiring    []NamedCount // Count is days left
	BackupsOK        int
	BackupsFailed    int
	DiskWorstPercent float64
	DiskWorstNode    string
}

// NamedCount is a label with a number, used for restarts per app and days left per certificate.
type NamedCount struct {
	Name  string
	Count int
}

// WeeklySources are the existing readers the digest builds on; any nil source leaves its section out.
type WeeklySources struct {
	Services ServiceLister
	Attempts interface {
		ListDeployAttempts(ctx context.Context, serviceName string) ([]store.DeployAttempt, error)
	}
	Databases DatabaseLister
	Backups   DatabaseBackupSource
	Certs     CertSource
	Nodes     NodeSource
	Metrics   MetricsSource
	Tracker   *RestartTracker
	// CertWarnDays lists certificates expiring within this many days.
	CertWarnDays int
	Logger       *slog.Logger
}

// CollectWeeklyFacts reads one week ending at now. A failing section is logged and skipped so one bad source never blocks the mail.
func CollectWeeklyFacts(ctx context.Context, src WeeklySources, now time.Time) WeeklyFacts {
	logger := src.Logger
	if logger == nil {
		logger = slog.Default()
	}
	f := WeeklyFacts{Since: now.Add(-7 * 24 * time.Hour), Until: now}
	if src.Services != nil {
		svcs, err := src.Services.ListDesiredServices(ctx)
		if err != nil {
			logger.Warn("weekly digest: list apps failed", slog.String("error", err.Error()))
		}
		f.Apps = len(svcs)
		for _, s := range svcs {
			if src.Attempts != nil {
				at, err := src.Attempts.ListDeployAttempts(ctx, s.Name)
				if err != nil {
					continue
				}
				for _, a := range at {
					if a.StartedAt.Before(f.Since) {
						break
					}
					f.Deploys++
					if a.Status == store.DeployAttemptStatusFailed {
						f.DeploysFailed++
					}
				}
			}
			if src.Tracker != nil {
				if n := src.Tracker.CountSince(s.Name, f.Since); n > 0 {
					f.Restarts = append(f.Restarts, NamedCount{Name: s.Name, Count: n})
				}
			}
		}
	}
	sort.Slice(f.Restarts, func(i, j int) bool { return f.Restarts[i].Count > f.Restarts[j].Count })
	if src.Databases != nil && src.Backups != nil {
		dbs, err := src.Databases.ListDesiredDatabases(ctx)
		if err != nil {
			logger.Warn("weekly digest: list databases failed", slog.String("error", err.Error()))
		}
		for _, d := range dbs {
			hist, err := src.Backups.ListBackupHistory(ctx, d.Name, 50, nil)
			if err != nil {
				continue
			}
			for _, h := range hist {
				if at, err := time.Parse(time.RFC3339, h.StartedAt); err == nil && at.Before(f.Since) {
					continue
				}
				switch h.Status {
				case store.BackupStatusSucceeded:
					f.BackupsOK++
				case store.BackupStatusFailed:
					f.BackupsFailed++
				}
			}
		}
	}
	if src.Certs != nil {
		warn := src.CertWarnDays
		if warn <= 0 {
			warn = 30
		}
		certs, err := ListCertificates(ctx, src.Certs, time.Duration(warn)*24*time.Hour, now, logger)
		if err != nil {
			logger.Warn("weekly digest: list certificates failed", slog.String("error", err.Error()))
		}
		for _, c := range certs {
			if c.Internal {
				continue
			}
			if days := int(c.NotAfter.Sub(now).Hours() / 24); days <= warn {
				f.CertsExpiring = append(f.CertsExpiring, NamedCount{Name: c.Domain, Count: days})
			}
		}
		sort.Slice(f.CertsExpiring, func(i, j int) bool { return f.CertsExpiring[i].Count < f.CertsExpiring[j].Count })
	}
	if src.Nodes != nil && src.Metrics != nil {
		nodes, err := src.Nodes.ListNodes(ctx)
		if err != nil {
			logger.Warn("weekly digest: list nodes failed", slog.String("error", err.Error()))
		}
		for _, n := range nodes {
			pct, have, _ := evaluateNodeDiskSpaceForNode(ctx, src.Metrics, "weekly-digest", n, 101, now.Add(-diskSpaceLookback), now, logger)
			if have && pct > f.DiskWorstPercent {
				f.DiskWorstPercent, f.DiskWorstNode = pct, n.ID
			}
		}
	}
	return f
}

// RenderWeeklyDigest returns the subject, plain text and HTML bodies. The plain text is what the mail carries today and what the CLI preview prints.
func RenderWeeklyDigest(f WeeklyFacts) (subject, text, htmlBody string) {
	subject = fmt.Sprintf("Weekly summary for %s", f.Until.UTC().Format("2006-01-02"))
	lines := []string{
		fmt.Sprintf("Apps: %d", f.Apps),
		fmt.Sprintf("Deploys: %d (%d failed)", f.Deploys, f.DeploysFailed),
		fmt.Sprintf("Backups: %d succeeded, %d failed", f.BackupsOK, f.BackupsFailed),
	}
	if f.DiskWorstNode != "" {
		lines = append(lines, fmt.Sprintf("Disk: highest use %.0f%% on %s", f.DiskWorstPercent, f.DiskWorstNode))
	} else {
		lines = append(lines, "Disk: no readings")
	}
	if len(f.Restarts) == 0 {
		lines = append(lines, "Restarts: none")
	} else {
		parts := make([]string, 0, len(f.Restarts))
		for _, r := range f.Restarts {
			parts = append(parts, fmt.Sprintf("%s %d", r.Name, r.Count))
		}
		lines = append(lines, "Restarts: "+strings.Join(parts, ", "))
	}
	if len(f.CertsExpiring) == 0 {
		lines = append(lines, "Certificates: none expiring soon")
	} else {
		parts := make([]string, 0, len(f.CertsExpiring))
		for _, c := range f.CertsExpiring {
			parts = append(parts, fmt.Sprintf("%s in %d days", c.Name, c.Count))
		}
		lines = append(lines, "Certificates expiring: "+strings.Join(parts, ", "))
	}
	head := fmt.Sprintf("Week of %s to %s", f.Since.UTC().Format("2006-01-02"), f.Until.UTC().Format("2006-01-02"))
	text = head + "\n\n" + strings.Join(lines, "\n") + "\n"
	var b strings.Builder
	b.WriteString("<h2>" + html.EscapeString(head) + "</h2><ul>")
	for _, l := range lines {
		b.WriteString("<li>" + html.EscapeString(l) + "</li>")
	}
	b.WriteString("</ul>")
	return subject, text, b.String()
}

// DigestMailer sends the digest; the second result is false when no email provider is configured.
type DigestMailer interface {
	Configured(ctx context.Context) bool
	Send(ctx context.Context, to, subject, body string) error
}

// WeeklyDigest decides when the weekly mail is due and sends it only when email is configured.
type WeeklyDigest struct {
	Sources   WeeklySources
	Mailer    DigestMailer
	Schedule  func(ctx context.Context) (weekday, hour int, on bool)
	Recipient func(ctx context.Context) string
	// Last and SetLast persist the last send day so a restart inside the due hour does not resend.
	Last    func(ctx context.Context) string
	SetLast func(ctx context.Context, day string)
	Logger  *slog.Logger
}

// Due reports whether now falls in the scheduled hour and the week's mail was not already sent today.
func (w *WeeklyDigest) Due(ctx context.Context, now time.Time) bool {
	wd, hour, on := w.Schedule(ctx)
	now = now.UTC()
	return on && int(now.Weekday()) == wd && now.Hour() == hour && w.Last(ctx) != now.Format("2006-01-02")
}

// Tick sends the digest when due. Without a configured mailer or recipient nothing is sent and nothing fails.
func (w *WeeklyDigest) Tick(ctx context.Context, now time.Time) {
	if !w.Due(ctx, now) || w.Mailer == nil || !w.Mailer.Configured(ctx) {
		return
	}
	to := w.Recipient(ctx)
	if to == "" {
		return
	}
	_, text, _ := RenderWeeklyDigest(CollectWeeklyFacts(ctx, w.Sources, now))
	subject, _, _ := RenderWeeklyDigest(WeeklyFacts{Until: now})
	if err := w.Mailer.Send(ctx, to, subject, text); err != nil {
		if w.Logger != nil {
			w.Logger.Warn("weekly digest send failed", slog.String("error", err.Error()))
		}
		return
	}
	w.SetLast(ctx, now.UTC().Format("2006-01-02"))
}
