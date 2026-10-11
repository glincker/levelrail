package api

import (
	"context"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/attention"
	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envFailedLoginWindow           = "APP_SECURITY_FAILED_LOGIN_WINDOW"
	envFailedLoginAccountThreshold = "APP_SECURITY_FAILED_LOGIN_ACCOUNT_THRESHOLD"
	envFailedLoginIPThreshold      = "APP_SECURITY_FAILED_LOGIN_IP_THRESHOLD"
	defaultFailedLoginWindow       = 15 * time.Minute
	defaultFailedLoginAccount      = 10
	defaultFailedLoginIP           = 20
	failedLoginMaxKeys             = 10_000
	failedLoginEventKeep           = 24 * time.Hour
	failedLoginMaxEvents           = 50

	anomalyKindAccount = "account"
	anomalyKindIP      = "ip"
)

// loginAnomaly is one threshold crossing: too many failed passwords for one
// account, or from one address, inside the window.
type loginAnomaly struct {
	Kind    string    `json:"kind"`
	Subject string    `json:"subject"`
	Count   int       `json:"count"`
	At      time.Time `json:"at"`
}

// failedLoginCounter counts failed password attempts per account and per
// address in a sliding window. In memory only: a restart resets the counts.
type failedLoginCounter struct {
	mu        sync.Mutex
	window    time.Duration
	acctLimit int
	ipLimit   int
	byKey     map[string][]time.Time
	flagged   map[string]time.Time
	events    []loginAnomaly
}

func newFailedLoginCounter() *failedLoginCounter {
	return &failedLoginCounter{
		window:    envDuration(envFailedLoginWindow, defaultFailedLoginWindow),
		acctLimit: envInt(envFailedLoginAccountThreshold, defaultFailedLoginAccount),
		ipLimit:   envInt(envFailedLoginIPThreshold, defaultFailedLoginIP),
		byKey:     map[string][]time.Time{},
		flagged:   map[string]time.Time{},
	}
}

// record counts one failure and returns any threshold it just crossed. A key
// raises at most one anomaly per window, so a flood is one alert, not many.
func (c *failedLoginCounter) record(account, ip string, now time.Time) []loginAnomaly {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(now)
	var out []loginAnomaly
	for _, k := range []struct {
		kind, subject string
		limit         int
	}{{anomalyKindAccount, strings.ToLower(strings.TrimSpace(account)), c.acctLimit}, {anomalyKindIP, ip, c.ipLimit}} {
		if k.subject == "" || k.limit <= 0 {
			continue
		}
		key := k.kind + "|" + k.subject
		if _, known := c.byKey[key]; !known && len(c.byKey) >= failedLoginMaxKeys {
			continue
		}
		c.byKey[key] = append(c.byKey[key], now)
		n := len(c.byKey[key])
		if last, ok := c.flagged[key]; n < k.limit || (ok && now.Sub(last) < c.window) {
			continue
		}
		c.flagged[key] = now
		a := loginAnomaly{Kind: k.kind, Subject: k.subject, Count: n, At: now}
		c.events = append(c.events, a)
		out = append(out, a)
	}
	if len(c.events) > failedLoginMaxEvents {
		c.events = c.events[len(c.events)-failedLoginMaxEvents:]
	}
	return out
}

func (c *failedLoginCounter) pruneLocked(now time.Time) {
	cutoff := now.Add(-c.window)
	for k, ts := range c.byKey {
		i := sort.Search(len(ts), func(i int) bool { return ts[i].After(cutoff) })
		if i == len(ts) {
			delete(c.byKey, k)
			continue
		}
		c.byKey[k] = ts[i:]
	}
	for k, at := range c.flagged {
		if now.Sub(at) >= c.window {
			delete(c.flagged, k)
		}
	}
	keep := c.events[:0]
	for _, e := range c.events {
		if now.Sub(e.At) < failedLoginEventKeep {
			keep = append(keep, e)
		}
	}
	c.events = keep
}

// recent returns the anomalies of the last day, newest first.
func (c *failedLoginCounter) recent(now time.Time) []loginAnomaly {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pruneLocked(now)
	out := make([]loginAnomaly, 0, len(c.events))
	for i := len(c.events) - 1; i >= 0; i-- {
		out = append(out, c.events[i])
	}
	return out
}

// recordFailedPassword feeds one failed password sign-in to the counter and
// audits and announces any threshold it crosses.
func (rt *Router) recordFailedPassword(r *http.Request, username string) {
	for _, a := range rt.sec.failures.record(username, clientIP(r), time.Now()) {
		subject := safeSignInText(a.Subject)
		if a.Kind == anomalyKindIP {
			subject = safeSignInIP(a.Subject)
		}
		rt.auditSignIn(r.Context(), r, signInActor{kind: auditActorSystem, name: signInAuditActor}, store.AuditActionFailedLoginsThreshold,
			"/api/v1/security/anomalies#"+a.Kind+"#"+strconv.Itoa(a.Count), http.StatusOK)
		rt.sendSecurityNotice("Security: " + strconv.Itoa(a.Count) + " failed password sign-ins for " + a.Kind + " " + subject +
			" in the last " + rt.sec.failures.window.String() + ". Review the security center.")
	}
}

// loginAnomalyItems surfaces recent failed-sign-in bursts: every one for an
// admin, only those naming the caller's own account otherwise.
func (rt *Router) loginAnomalyItems(r *http.Request, abilities []string, now time.Time) []attention.Item {
	admin := hasAbility(abilities, AbilityRoot)
	own := ""
	if userID, ok := rt.currentSessionUserID(r); ok {
		if u, err := rt.auth.GetUserByID(r.Context(), userID); err == nil {
			own = strings.ToLower(u.Email)
		}
	}
	var items []attention.Item
	for _, a := range rt.sec.failures.recent(now) {
		if !admin && (a.Kind != anomalyKindAccount || a.Subject != own || own == "") {
			continue
		}
		subject := safeSignInText(a.Subject)
		if a.Kind == anomalyKindIP {
			subject = safeSignInIP(a.Subject)
		}
		it := feedItem(attention.Warning, attention.KindLoginAnomaly, subject,
			strconv.Itoa(a.Count)+" failed password sign-ins for "+a.Kind+" "+subject,
			map[string]string{"kind": a.Kind, "subject": subject, "count": strconv.Itoa(a.Count), "at": a.At.UTC().Format(time.RFC3339)})
		it.ID += ":" + a.Kind
		items = append(items, it)
	}
	return items
}

// sendSecurityNotice posts text to channels opted in to sign-in notices,
// detached and bounded so a sign-in never waits on a webhook.
func (rt *Router) sendSecurityNotice(text string) {
	if rt.deviceNotifier == nil || rt.notificationChannels == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), deviceNoticeTimeout)
		defer cancel()
		channels, err := rt.notificationChannels.ListNotificationChannels(ctx)
		if err != nil {
			rt.logger.Error("api: list notification channels for security notice failed", slog.String("error", err.Error()))
			return
		}
		for _, c := range channels {
			if !c.Enabled || !c.NotifyDeviceLogin {
				continue
			}
			if err := rt.deviceNotifier.SendNotice(ctx, c.Kind, c.NotifyURL, text); err != nil {
				rt.logger.Error("api: send security notice failed", slog.String("channel_id", c.ID), slog.String("error", err.Error()))
			}
		}
	}()
}
