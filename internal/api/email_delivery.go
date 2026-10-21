package api

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
)

// emailTestTimeout bounds the synchronous test-send: an
// unresponsive-but-connectable backend can't hang the request goroutine.
const emailTestTimeout = 20 * time.Second

// EmailDeliveryStore is the delivery log surface the email handlers read; *store.DB satisfies it.
type EmailDeliveryStore interface {
	ListEmailDeliveries(ctx context.Context, f store.EmailDeliveryFilter) ([]store.EmailDelivery, error)
	GetEmailDelivery(ctx context.Context, id string) (store.EmailDelivery, error)
	EmailDeliveryStats(ctx context.Context, since time.Time) (store.EmailDeliveryStats, error)
}

type testEmailRequest struct {
	To string `json:"to"`
}

type testEmailResponse struct {
	To        string `json:"to"`
	Provider  string `json:"provider"`
	MessageID string `json:"message_id,omitempty"`
	Attempts  int    `json:"attempts"`
}

type detailedSender interface {
	SendDetailed(ctx context.Context, to, subject, body string) (email.Result, error)
}

// handleTestEmail handles POST /api/v1/settings/email/test: sends one
// real email through rt.emailSender, the exact same DynamicSender
// alert notifications and password resets already use, so a successful
// test proves the real send path works. With no "to", it goes to the
// signed-in user's own address.
func (rt *Router) handleTestEmail(w http.ResponseWriter, r *http.Request) {
	if rt.emailSender == nil {
		writeError(w, http.StatusNotImplemented, "email is not configured on this control plane")
		return
	}

	var req testEmailRequest
	if !decodeReq(w, r, &req) {
		return
	}
	to := strings.TrimSpace(req.To)
	if to == "" {
		to = rt.signedInUserEmail(r)
		if to == "" {
			writeError(w, http.StatusBadRequest, "to is required: no signed-in user address to default to")
			return
		}
	}
	if err := email.ValidateAddress(to); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(email.WithoutRetry(email.WithKind(r.Context(), "test")), emailTestTimeout)
	defer cancel()
	subject := "Test email from " + rt.brand.Name
	body := "This is a test email from your " + rt.brand.Name + " control plane. If you received this, your email settings are working."

	var res email.Result
	var err error
	if ds, ok := rt.emailSender.(detailedSender); ok {
		res, err = ds.SendDetailed(ctx, to, subject, body)
	} else {
		err = rt.emailSender.Send(ctx, to, subject, body)
	}
	if err != nil {
		kind := email.Classify(err)
		rt.logger.Warn("api: test email failed", slog.String("kind", string(kind)))
		writeCodedError(w, http.StatusBadGateway, string(kind), err.Error())
		return
	}
	writeJSON(w, http.StatusOK, testEmailResponse{To: to, Provider: res.Provider, MessageID: res.MessageID, Attempts: max(res.Attempts, 1)})
}

func (rt *Router) signedInUserEmail(r *http.Request) string {
	userID, ok := rt.currentSessionUserID(r)
	if !ok || rt.auth == nil {
		return ""
	}
	u, err := rt.auth.GetUserByID(r.Context(), userID)
	if err != nil || u == nil {
		return ""
	}
	return u.Email
}

type emailStatusResponse struct {
	Configured    bool       `json:"configured"`
	Provider      string     `json:"provider"`
	From          string     `json:"from,omitempty"`
	LastSuccessAt *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt *time.Time `json:"last_failure_at,omitempty"`
	LastErrorKind string     `json:"last_error_kind,omitempty"`
	Sent24h       int        `json:"sent_24h"`
	Failed24h     int        `json:"failed_24h"`
}

// handleEmailStatus handles GET /api/v1/settings/email/status: whether sending is set up plus the last success and failure.
func (rt *Router) handleEmailStatus(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.emailSettings.GetEmailSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: email status: get settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	out := emailStatusResponse{Provider: settings.Backend, From: settings.EffectiveFrom()}
	if c, ok := rt.emailSender.(interface{ Configured(context.Context) bool }); ok {
		out.Configured = c.Configured(r.Context())
	}
	if out.Provider == "" && out.Configured {
		out.Provider = "env"
	}
	if rt.emailDeliveries != nil {
		st, err := rt.emailDeliveries.EmailDeliveryStats(r.Context(), time.Now().Add(-24*time.Hour))
		if err != nil {
			rt.logger.Error("api: email status: stats failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		out.Sent24h, out.Failed24h = st.Sent, st.Failed
		if st.LastSuccess != nil {
			out.LastSuccessAt = &st.LastSuccess.CreatedAt
		}
		if st.LastFailure != nil {
			out.LastFailureAt = &st.LastFailure.CreatedAt
			out.LastErrorKind = st.LastFailure.ErrorKind
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleEmailDomainCheck handles GET /api/v1/settings/email/domain-check: SPF, DKIM and DMARC for the From domain, resolved by the control plane.
func (rt *Router) handleEmailDomainCheck(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.emailSettings.GetEmailSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: email domain check: get settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	domain := email.DomainOf(settings.EffectiveFrom())
	if d := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain"))); d != "" {
		domain = d
	}
	if domain == "" || strings.ContainsAny(domain, " /\\@") {
		writeError(w, http.StatusBadRequest, "no sender domain: set a from address first")
		return
	}
	var dns email.DNSLookup = email.SystemDNS{}
	if rt.emailDNS != nil {
		dns = rt.emailDNS
	}
	writeJSON(w, http.StatusOK, email.CheckDomain(r.Context(), dns, domain, email.Backend(settings.Backend)))
}

type emailDeliveriesResponse struct {
	Deliveries []store.EmailDelivery `json:"deliveries"`
	// NextBefore is the cursor for the next page; empty when there is none.
	NextBefore string `json:"next_before,omitempty"`
}

// handleListEmailDeliveries handles GET /api/v1/settings/email/deliveries with status, provider, kind, q, since, before and limit filters.
func (rt *Router) handleListEmailDeliveries(w http.ResponseWriter, r *http.Request) {
	if rt.emailDeliveries == nil {
		writeJSON(w, http.StatusOK, emailDeliveriesResponse{Deliveries: []store.EmailDelivery{}})
		return
	}
	q := r.URL.Query()
	f := store.EmailDeliveryFilter{Status: q.Get("status"), Provider: q.Get("provider"), Kind: q.Get("kind"), Query: strings.TrimSpace(q.Get("q"))}
	if f.Status != "" && f.Status != store.EmailDeliverySent && f.Status != store.EmailDeliveryFailed && f.Status != store.EmailDeliveryBounced {
		writeError(w, http.StatusBadRequest, "status must be sent, failed or bounced")
		return
	}
	for name, dst := range map[string]*time.Time{"since": &f.Since, "before": &f.Before} {
		if v := q.Get(name); v != "" {
			t, err := time.Parse(time.RFC3339Nano, v)
			if err != nil {
				writeError(w, http.StatusBadRequest, name+" must be an RFC 3339 time")
				return
			}
			*dst = t
		}
	}
	limit := 50
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 200 {
			writeError(w, http.StatusBadRequest, "limit must be between 1 and 200")
			return
		}
		limit = n
	}
	f.Limit = limit
	rows, err := rt.emailDeliveries.ListEmailDeliveries(r.Context(), f)
	if err != nil {
		rt.logger.Error("api: list email deliveries failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	if rows == nil {
		rows = []store.EmailDelivery{}
	}
	out := emailDeliveriesResponse{Deliveries: rows}
	if len(rows) == limit {
		out.NextBefore = rows[len(rows)-1].CreatedAt.UTC().Format("2006-01-02T15:04:05.000000000Z")
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetEmailDelivery handles GET /api/v1/settings/email/deliveries/{id}.
func (rt *Router) handleGetEmailDelivery(w http.ResponseWriter, r *http.Request) {
	if rt.emailDeliveries == nil {
		writeError(w, http.StatusNotFound, "delivery not found")
		return
	}
	d, err := rt.emailDeliveries.GetEmailDelivery(r.Context(), r.PathValue("id"))
	if errors.Is(err, sql.ErrNoRows) {
		writeError(w, http.StatusNotFound, "delivery not found")
		return
	}
	if err != nil {
		rt.logger.Error("api: get email delivery failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, d)
}
