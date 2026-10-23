package api

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/mailgroups"
	"github.com/GLINCKER/levelrail/internal/mailtemplates"
	"github.com/GLINCKER/levelrail/internal/store"
)

const maxScheduleAhead = 90 * 24 * time.Hour

type mailBroadcastRequest struct {
	Subject      string   `json:"subject"`
	Body         string   `json:"body"`
	TemplateKey  string   `json:"template_key"`
	Audience     string   `json:"audience"`
	Recipients   []string `json:"recipients"`
	ScheduleAt   string   `json:"schedule_at"`
	DryRun       bool     `json:"dry_run"`
	ExpectedSend int      `json:"expected_recipients"`
}

type mailBroadcastPlan struct {
	mailAudiencePreview
	CapRemaining       int    `json:"cap_remaining"`
	ProviderConfigured bool   `json:"provider_configured"`
	Warning            string `json:"warning,omitempty"`

	send, held []mailgroups.Recipient
}

// planBroadcast validates a request and works out who would get it.
func (rt *Router) planBroadcast(ctx context.Context, req mailBroadcastRequest) (mailBroadcastPlan, store.MailBroadcast, error) {
	var plan mailBroadcastPlan
	b := store.MailBroadcast{Subject: strings.TrimSpace(req.Subject), Body: req.Body, TemplateKey: req.TemplateKey}
	ev, ok := mailtemplates.LookupEvent(mailtemplates.EventAnnouncement)
	if !ok {
		return plan, b, errors.New("announcement template missing")
	}
	if err := mailtemplates.ValidateTemplate(ev, mailtemplates.Template{Subject: b.Subject, Body: b.Body}); err != nil {
		return plan, b, err
	}
	if req.TemplateKey != "" {
		if tev, ok := mailtemplates.LookupEvent(req.TemplateKey); !ok || tev.Category != mailtemplates.CategoryAnnouncement {
			return plan, b, errors.New("template must be an announcement template")
		}
	}
	rules := mailgroups.Rules{}
	if req.Audience != "" {
		g, err := rt.mail.db.GetMailGroup(ctx, req.Audience)
		if err != nil {
			return plan, b, errors.New("no such audience group")
		}
		b.GroupID = g.ID
		rules = decodeGroupRules(*g)
	}
	for _, a := range req.Recipients {
		if err := mailgroups.ValidAddress(a); err != nil {
			return plan, b, err
		}
		rules.ExtraAddresses = append(rules.ExtraAddresses, a)
	}
	if req.Audience == "" && len(req.Recipients) == 0 {
		return plan, b, errors.New("choose an audience group or at least one recipient")
	}
	dir, err := rt.mailDirectory(ctx)
	if err != nil {
		return plan, b, err
	}
	suppressed, err := rt.mailSuppressedSet(ctx)
	if err != nil {
		return plan, b, err
	}
	plan.send, plan.held = rt.audience(rules, dir, suppressed)
	plan.mailAudiencePreview = previewOf(plan.send, plan.held)
	used, err := rt.mail.db.CountMailBroadcastRecipientsSince(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		return plan, b, err
	}
	plan.CapRemaining = max(0, envInt(envMailDailyCap, defaultMailDailyCap)-used)
	plan.ProviderConfigured = rt.mailProviderConfigured(ctx)
	if req.ScheduleAt != "" {
		at, perr := time.Parse(time.RFC3339, req.ScheduleAt)
		if perr != nil || at.After(time.Now().Add(maxScheduleAhead)) {
			return plan, b, errors.New("schedule_at must be an RFC 3339 time within 90 days")
		}
		if at.After(time.Now().Add(30 * time.Second)) {
			b.ScheduledAt = at.UTC().Format(time.RFC3339Nano)
		}
	}
	return plan, b, nil
}

// handleCreateMailBroadcast handles POST /api/v1/mail/broadcasts: a dry run, or the real queued send.
func (rt *Router) handleCreateMailBroadcast(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	var req mailBroadcastRequest
	if !decodeReq(w, r, &req) {
		return
	}
	ctx := r.Context()
	plan, b, err := rt.planBroadcast(ctx, req)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if req.DryRun {
		writeJSON(w, http.StatusOK, plan)
		return
	}
	if !plan.ProviderConfigured {
		writeCodedError(w, http.StatusConflict, "email_not_configured", "no email provider is configured; set one up in Settings, Email, then try again")
		return
	}
	if len(plan.send) == 0 {
		writeError(w, http.StatusBadRequest, "no deliverable recipients")
		return
	}
	if req.ExpectedSend > 0 && req.ExpectedSend != len(plan.send) {
		writeCodedError(w, http.StatusConflict, "recipient_count_changed", "the audience changed since you confirmed it; review the count and confirm again")
		return
	}
	if len(plan.send) > plan.CapRemaining {
		writeCodedError(w, http.StatusTooManyRequests, "daily_cap_reached", "this would exceed the daily broadcast limit ("+strconv.Itoa(plan.CapRemaining)+" recipients left today)")
		return
	}
	id, err := randomOpaqueID("mb_")
	if err != nil {
		rt.internalError(w, "api: broadcast id failed", err)
		return
	}
	b.ID, b.Status, b.CreatedBy = id, store.MailBroadcastSending, rt.actorForRequest(r)
	b.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	if b.ScheduledAt != "" {
		b.Status = store.MailBroadcastScheduled
	}
	b.Total, b.Suppressed = len(plan.send)+len(plan.held), len(plan.held)
	recipients := make([]store.MailRecipient, 0, b.Total)
	for _, rc := range plan.send {
		recipients = append(recipients, store.MailRecipient{Email: rc.Email, UserID: rc.UserID, Status: store.MailRecipientPending})
	}
	for _, rc := range plan.held {
		recipients = append(recipients, store.MailRecipient{Email: rc.Email, UserID: rc.UserID, Status: store.MailRecipientSuppressed})
	}
	if err := rt.mail.db.CreateMailBroadcast(ctx, b, recipients); err != nil {
		rt.internalError(w, "api: create broadcast failed", err)
		return
	}
	if b.Status == store.MailBroadcastSending {
		rt.mail.runner.Start(ctx, b.ID)
	}
	writeJSON(w, http.StatusCreated, b)
}

// handleListMailBroadcasts handles GET /api/v1/mail/broadcasts, newest first.
func (rt *Router) handleListMailBroadcasts(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	list, err := rt.mail.db.ListMailBroadcasts(r.Context(), mailPageLimit)
	if err != nil {
		rt.internalError(w, "api: list broadcasts failed", err)
		return
	}
	if list == nil {
		list = []store.MailBroadcast{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"broadcasts": list})
}

// handleGetMailBroadcast handles GET /api/v1/mail/broadcasts/{id}, with per-recipient status (addresses masked).
func (rt *Router) handleGetMailBroadcast(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	b, err := rt.mail.db.GetMailBroadcast(r.Context(), r.PathValue("id"))
	if errors.Is(err, store.ErrMailNotFound) {
		writeError(w, http.StatusNotFound, "no such broadcast")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get broadcast failed", err)
		return
	}
	rcpts, err := rt.mail.db.ListMailRecipients(r.Context(), b.ID, r.URL.Query().Get("status"), mailPageLimit)
	if err != nil {
		rt.internalError(w, "api: list broadcast recipients failed", err)
		return
	}
	for i := range rcpts {
		rcpts[i].Email = mailgroups.Mask(rcpts[i].Email)
	}
	if rcpts == nil {
		rcpts = []store.MailRecipient{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"broadcast": b, "recipients": rcpts})
}

// handleCancelMailBroadcast handles POST /api/v1/mail/broadcasts/{id}/cancel.
func (rt *Router) handleCancelMailBroadcast(w http.ResponseWriter, r *http.Request) {
	if !rt.requireMail(w) {
		return
	}
	id := r.PathValue("id")
	if err := rt.mail.runner.Cancel(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrMailNotFound) {
			writeError(w, http.StatusNotFound, "no such broadcast")
			return
		}
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	b, err := rt.mail.db.GetMailBroadcast(r.Context(), id)
	if err != nil {
		rt.internalError(w, "api: get broadcast failed", err)
		return
	}
	writeJSON(w, http.StatusOK, b)
}
