package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EmailProviderStore is the provider surface the email provider handlers need; *store.DB satisfies it.
type EmailProviderStore interface {
	ListEmailProviders(ctx context.Context) ([]store.EmailProvider, error)
	GetEmailProvider(ctx context.Context, id string) (store.EmailProvider, error)
	UpsertEmailProvider(ctx context.Context, p store.EmailProvider) error
	DeleteEmailProvider(ctx context.Context, id string) error
	PromoteEmailProvider(ctx context.Context, id string) error
}

const emailProviderNameMax = 80

// emailProviderResource is the wire shape of one provider. Credential fields are write-only: a response carries
// only the *_set booleans, and an empty credential on a write leaves the stored value alone.
type emailProviderResource struct {
	ID      string `json:"id,omitempty"`
	Role    string `json:"role,omitempty"`
	Name    string `json:"name,omitempty"`
	Enabled *bool  `json:"enabled,omitempty"`
	Backend string `json:"backend"`

	TLSMode       string `json:"tls_mode,omitempty"`
	TLSSkipVerify bool   `json:"tls_skip_verify,omitempty"`

	SMTPHost        string `json:"smtp_host,omitempty"`
	SMTPPort        int    `json:"smtp_port,omitempty"`
	SMTPUsername    string `json:"smtp_username,omitempty"`
	SMTPFrom        string `json:"smtp_from,omitempty"`
	SMTPPassword    string `json:"smtp_password,omitempty"`
	SMTPPasswordSet bool   `json:"smtp_password_set,omitempty"`

	SESRegion             string `json:"ses_region,omitempty"`
	SESAccessKeyID        string `json:"ses_access_key_id,omitempty"`
	SESFrom               string `json:"ses_from,omitempty"`
	SESSecretAccessKey    string `json:"ses_secret_access_key,omitempty"`
	SESSecretAccessKeySet bool   `json:"ses_secret_access_key_set,omitempty"`

	SESSMTPPort        int    `json:"ses_smtp_port,omitempty"`
	SESSMTPUsername    string `json:"ses_smtp_username,omitempty"`
	SESSMTPPassword    string `json:"ses_smtp_password,omitempty"`
	SESSMTPPasswordSet bool   `json:"ses_smtp_password_set,omitempty"`

	ResendFrom      string `json:"resend_from,omitempty"`
	ResendAPIKey    string `json:"resend_api_key,omitempty"`
	ResendAPIKeySet bool   `json:"resend_api_key_set,omitempty"`

	PostmarkServerToken    string `json:"postmark_server_token,omitempty"`
	PostmarkServerTokenSet bool   `json:"postmark_server_token_set,omitempty"`
	SendGridAPIKey         string `json:"sendgrid_api_key,omitempty"`
	SendGridAPIKeySet      bool   `json:"sendgrid_api_key_set,omitempty"`
	MailgunAPIKey          string `json:"mailgun_api_key,omitempty"`
	MailgunAPIKeySet       bool   `json:"mailgun_api_key_set,omitempty"`
	MailgunDomain          string `json:"mailgun_domain,omitempty"`
	MailgunRegion          string `json:"mailgun_region,omitempty"`

	LastOKAt      *time.Time `json:"last_ok_at,omitempty"`
	LastErrorKind string     `json:"last_error_kind,omitempty"`
	CreatedAt     *time.Time `json:"created_at,omitempty"`
	UpdatedAt     *time.Time `json:"updated_at,omitempty"`
}

type emailProviderListResponse struct {
	Providers []emailProviderResource `json:"providers"`
}

var errEmailProviderBackendRequired = errors.New("backend is required")

// secretView copies the write-only credential fields onto the shared settings shape, so emailSecretFields drives both.
func (p *emailProviderResource) secretView() emailSettingsResource {
	return emailSettingsResource{
		SMTPPassword: p.SMTPPassword, SESSecretAccessKey: p.SESSecretAccessKey, SESSMTPPassword: p.SESSMTPPassword,
		ResendAPIKey: p.ResendAPIKey, PostmarkServerToken: p.PostmarkServerToken, SendGridAPIKey: p.SendGridAPIKey,
		MailgunAPIKey: p.MailgunAPIKey,
	}
}

func (p *emailProviderResource) applyFlags(v emailSettingsResource) {
	p.SMTPPasswordSet, p.SESSecretAccessKeySet, p.SESSMTPPasswordSet = v.SMTPPasswordSet, v.SESSecretAccessKeySet, v.SESSMTPPasswordSet
	p.ResendAPIKeySet, p.PostmarkServerTokenSet, p.SendGridAPIKeySet = v.ResendAPIKeySet, v.PostmarkServerTokenSet, v.SendGridAPIKeySet
	p.MailgunAPIKeySet = v.MailgunAPIKeySet
}

func toEmailProviderResource(p store.EmailProvider) emailProviderResource {
	enabled := p.Enabled
	return emailProviderResource{
		ID: p.ID, Role: p.Role, Name: p.Name, Enabled: &enabled, Backend: p.Backend,
		TLSMode: p.TLSMode, TLSSkipVerify: p.TLSSkipVerify,
		SMTPHost: p.SMTPHost, SMTPPort: p.SMTPPort, SMTPUsername: p.SMTPUsername, SMTPFrom: p.SMTPFrom,
		SESRegion: p.SESRegion, SESAccessKeyID: p.SESAccessKeyID, SESFrom: p.SESFrom,
		SESSMTPPort: p.SESSMTPPort, SESSMTPUsername: p.SESSMTPUsername,
		ResendFrom: p.ResendFrom, MailgunDomain: p.MailgunDomain, MailgunRegion: p.MailgunRegion,
		LastOKAt: timePtr(p.LastOKAt), LastErrorKind: p.LastErrorKind,
		CreatedAt: timePtr(p.CreatedAt), UpdatedAt: timePtr(p.UpdatedAt),
	}
}

// providerWithFlags fills the *_set booleans from the provider's own secrets namespace.
func (rt *Router) providerWithFlags(ctx context.Context, p store.EmailProvider) (emailProviderResource, error) {
	res := toEmailProviderResource(p)
	if rt.emailSecrets == nil {
		return res, nil
	}
	var flags emailSettingsResource
	svc := store.EmailProviderSecretsKey(p.Role, p.ID)
	for _, f := range emailSecretFields {
		ok, err := rt.emailSecrets.Exists(ctx, svc, f.envKey)
		if err != nil {
			return res, fmt.Errorf("check %s: %w", f.envKey, err)
		}
		f.mark(&flags, ok)
	}
	res.applyFlags(flags)
	return res, nil
}

// settingsView is the shared-settings shape of a provider request, used to reuse the backend validation.
func (p *emailProviderResource) settingsView(sharedFrom string) emailSettingsResource {
	return emailSettingsResource{
		Backend: p.Backend, FromAddress: sharedFrom, TLSMode: p.TLSMode, TLSSkipVerify: p.TLSSkipVerify,
		SMTPHost: p.SMTPHost, SMTPPort: p.SMTPPort, SMTPUsername: p.SMTPUsername, SMTPFrom: p.SMTPFrom,
		SESRegion: p.SESRegion, SESAccessKeyID: p.SESAccessKeyID, SESFrom: p.SESFrom,
		SESSMTPPort: p.SESSMTPPort, SESSMTPUsername: p.SESSMTPUsername,
		ResendFrom: p.ResendFrom, MailgunDomain: p.MailgunDomain, MailgunRegion: p.MailgunRegion,
	}
}

func (p *emailProviderResource) validate(sharedFrom string) error {
	if p.Backend == "" {
		return errEmailProviderBackendRequired
	}
	if len(p.Name) > emailProviderNameMax || strings.ContainsAny(p.Name, "\r\n") {
		return fmt.Errorf("name must be at most %d characters and a single line", emailProviderNameMax)
	}
	return validateEmailSettingsRequest(p.settingsView(sharedFrom))
}

// toStore maps a request onto a row; existing carries the fields a request cannot set (role, timestamps, health).
func (p *emailProviderResource) toStore(existing store.EmailProvider) store.EmailProvider {
	out := existing
	out.Backend = p.Backend
	out.Name = p.Name
	if out.Name == "" && existing.Name != "" && existing.Backend == p.Backend {
		out.Name = existing.Name
	}
	if out.Name == "" {
		out.Name = p.Backend
	}
	if p.Enabled != nil {
		out.Enabled = *p.Enabled
	}
	out.SMTPHost, out.SMTPPort, out.SMTPUsername, out.SMTPFrom = p.SMTPHost, p.SMTPPort, p.SMTPUsername, p.SMTPFrom
	out.SESRegion, out.SESAccessKeyID, out.SESFrom, out.ResendFrom = p.SESRegion, p.SESAccessKeyID, p.SESFrom, p.ResendFrom
	out.TLSMode, out.TLSSkipVerify = p.TLSMode, p.TLSSkipVerify
	out.SESSMTPPort, out.SESSMTPUsername = p.SESSMTPPort, p.SESSMTPUsername
	if out.SESSMTPPort == 0 {
		out.SESSMTPPort = 587
	}
	out.MailgunDomain, out.MailgunRegion = p.MailgunDomain, p.MailgunRegion
	if out.MailgunRegion == "" {
		out.MailgunRegion = email.MailgunRegionUS
	}
	return out
}

// writeProviderError maps store errors to 4xx and logs anything else as a 500.
func (rt *Router) writeProviderError(w http.ResponseWriter, msg string, err error) {
	switch {
	case errors.Is(err, store.ErrEmailProviderNotFound):
		writeError(w, http.StatusNotFound, "email provider not found")
	case errors.Is(err, store.ErrEmailProviderInvalid):
		writeError(w, http.StatusConflict, strings.TrimPrefix(err.Error(), store.ErrEmailProviderInvalid.Error()+": "))
	default:
		rt.internalError(w, msg, err)
	}
}

// ensureNoBackupProvider refuses clearing the primary while a backup still depends on it.
func (rt *Router) ensureNoBackupProvider(ctx context.Context) error {
	rows, err := rt.emailProviders.ListEmailProviders(ctx)
	if err != nil {
		return err
	}
	for _, r := range rows {
		if r.Role == store.EmailRoleBackup {
			return fmt.Errorf("%w: remove the backup provider before clearing the primary", store.ErrEmailProviderInvalid)
		}
	}
	return nil
}

// syncPrimaryProvider keeps the primary provider row in step with a legacy settings write: the loader reads providers
// first, so the row is the source of truth for transport and email_settings is mirrored from it.
func (rt *Router) syncPrimaryProvider(ctx context.Context, s store.EmailSettings) error {
	rows, err := rt.emailProviders.ListEmailProviders(ctx)
	if err != nil {
		return err
	}
	var primary *store.EmailProvider
	for i := range rows {
		if rows[i].Role == store.EmailRolePrimary {
			primary = &rows[i]
		}
	}
	if s.Backend == "" {
		if primary == nil {
			return nil
		}
		return rt.emailProviders.DeleteEmailProvider(ctx, primary.ID)
	}
	next := store.EmailProvider{ID: store.EmailPrimaryProviderID, Role: store.EmailRolePrimary, Enabled: true, Name: s.Backend}
	if primary != nil {
		next = *primary
		if next.Backend != s.Backend {
			next.Name = s.Backend
		}
	}
	next.Backend = s.Backend
	next.SMTPHost, next.SMTPPort, next.SMTPUsername, next.SMTPFrom = s.SMTPHost, s.SMTPPort, s.SMTPUsername, s.SMTPFrom
	next.SESRegion, next.SESAccessKeyID, next.SESFrom, next.ResendFrom = s.SESRegion, s.SESAccessKeyID, s.SESFrom, s.ResendFrom
	next.TLSMode, next.TLSSkipVerify = s.TLSMode, s.TLSSkipVerify
	next.SESSMTPPort, next.SESSMTPUsername = s.SESSMTPPort, s.SESSMTPUsername
	next.MailgunDomain, next.MailgunRegion = s.MailgunDomain, s.MailgunRegion
	return rt.emailProviders.UpsertEmailProvider(ctx, next)
}

// auditEmailSecretChange records which credential fields changed for a provider, never their values.
func (rt *Router) auditEmailSecretChange(r *http.Request, providerID string, fields []string) {
	if len(fields) == 0 || rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: email secret audit id failed", slog.String("error", err.Error()))
		return
	}
	detail, err := json.Marshal(map[string]any{"provider_id": providerID, "fields": fields})
	if err != nil {
		return
	}
	actorType, actorID, actorName, _ := rt.currentActor(r)
	err = rt.auditLog.SaveAuditEntry(r.Context(), store.AuditEntry{
		ID: id, ActorType: actorType, ActorID: actorID, ActorName: actorName, Ability: AbilityRoot,
		Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: store.AuditActionEmailSecretsChanged, Detail: string(detail),
	})
	if err != nil {
		rt.logger.Warn("api: email secret audit failed", slog.String("error", err.Error()))
	}
}

// handleListEmailProviders handles GET /api/v1/settings/email/providers.
func (rt *Router) handleListEmailProviders(w http.ResponseWriter, r *http.Request) {
	rows, err := rt.emailProviders.ListEmailProviders(r.Context())
	if err != nil {
		rt.internalError(w, "api: list email providers failed", err)
		return
	}
	out := emailProviderListResponse{Providers: make([]emailProviderResource, 0, len(rows))}
	for _, p := range rows {
		res, err := rt.providerWithFlags(r.Context(), p)
		if err != nil {
			rt.internalError(w, "api: list email providers: secrets check failed", err)
			return
		}
		out.Providers = append(out.Providers, res)
	}
	writeJSON(w, http.StatusOK, out)
}

// handleGetEmailProvider handles GET /api/v1/settings/email/providers/{id}.
func (rt *Router) handleGetEmailProvider(w http.ResponseWriter, r *http.Request) {
	p, err := rt.emailProviders.GetEmailProvider(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeProviderError(w, "api: get email provider failed", err)
		return
	}
	res, err := rt.providerWithFlags(r.Context(), p)
	if err != nil {
		rt.internalError(w, "api: get email provider: secrets check failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// writeProviderSecrets stores each non-empty credential under the provider's namespace and returns the envKeys written.
func (rt *Router) writeProviderSecrets(ctx context.Context, role, id string, req emailProviderResource) ([]string, error) {
	view := req.secretView()
	svc := store.EmailProviderSecretsKey(role, id)
	var written []string
	for _, f := range emailSecretFields {
		v := f.value(&view)
		if v == "" {
			continue
		}
		if err := rt.emailSecrets.SetValue(ctx, svc, f.envKey, v); err != nil {
			return written, fmt.Errorf("set %s: %w", f.envKey, err)
		}
		written = append(written, f.envKey)
	}
	return written, nil
}

// handleCreateEmailProvider handles POST /api/v1/settings/email/providers. The first provider becomes the primary,
// the second the backup; a third is refused.
func (rt *Router) handleCreateEmailProvider(w http.ResponseWriter, r *http.Request) {
	if rt.emailSecrets == nil {
		writeError(w, http.StatusNotImplemented, "email settings are not configured on this control plane (no master key set)")
		return
	}
	var req emailProviderResource
	if !decodeReq(w, r, &req) {
		return
	}
	ctx := r.Context()
	settings, err := rt.emailSettings.GetEmailSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: create email provider: get settings failed", err)
		return
	}
	if err := req.validate(settings.FromAddress); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rows, err := rt.emailProviders.ListEmailProviders(ctx)
	if err != nil {
		rt.internalError(w, "api: create email provider: list failed", err)
		return
	}
	role := store.EmailRolePrimary
	for _, p := range rows {
		if p.Role == store.EmailRolePrimary {
			role = store.EmailRoleBackup
		} else {
			writeError(w, http.StatusConflict, "a primary and a backup email provider already exist; remove one first")
			return
		}
	}
	if role == store.EmailRolePrimary && settings.Backend != "" {
		// A backend configured through the legacy settings has no row yet: adopt it as the primary, this one is the backup.
		if err := rt.syncPrimaryProvider(ctx, settings); err != nil {
			rt.writeProviderError(w, "api: create email provider: adopt settings failed", err)
			return
		}
		role = store.EmailRoleBackup
	}
	id, err := randomOpaqueID("prov_")
	if err != nil {
		rt.internalError(w, "api: create email provider: id failed", err)
		return
	}
	p := req.toStore(store.EmailProvider{ID: id, Role: role, Enabled: true})
	written, err := rt.writeProviderSecrets(ctx, role, id, req)
	if err != nil {
		rt.internalError(w, "api: create email provider: set secret failed", err)
		return
	}
	if err := rt.emailProviders.UpsertEmailProvider(ctx, p); err != nil {
		if role == store.EmailRoleBackup {
			_ = rt.emailSecrets.DeleteAll(ctx, store.EmailProviderSecretsKey(role, id))
		}
		rt.writeProviderError(w, "api: create email provider failed", err)
		return
	}
	rt.auditEmailSecretChange(r, id, written)
	saved, err := rt.emailProviders.GetEmailProvider(ctx, id)
	if err != nil {
		rt.internalError(w, "api: create email provider: reload failed", err)
		return
	}
	res, err := rt.providerWithFlags(ctx, saved)
	if err != nil {
		rt.internalError(w, "api: create email provider: secrets check failed", err)
		return
	}
	writeJSON(w, http.StatusCreated, res)
}

// handleUpdateEmailProvider handles PUT /api/v1/settings/email/providers/{id}. Secrets are written before the row,
// so a failed row write leaves an orphaned secret, not a row that looks configured but is not.
func (rt *Router) handleUpdateEmailProvider(w http.ResponseWriter, r *http.Request) {
	if rt.emailSecrets == nil {
		writeError(w, http.StatusNotImplemented, "email settings are not configured on this control plane (no master key set)")
		return
	}
	ctx := r.Context()
	id := r.PathValue("id")
	existing, err := rt.emailProviders.GetEmailProvider(ctx, id)
	if err != nil {
		rt.writeProviderError(w, "api: update email provider: get failed", err)
		return
	}
	var req emailProviderResource
	if !decodeReq(w, r, &req) {
		return
	}
	settings, err := rt.emailSettings.GetEmailSettings(ctx)
	if err != nil {
		rt.internalError(w, "api: update email provider: get settings failed", err)
		return
	}
	if err := req.validate(settings.FromAddress); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	written, err := rt.writeProviderSecrets(ctx, existing.Role, id, req)
	rt.auditEmailSecretChange(r, id, written)
	if err != nil {
		rt.internalError(w, "api: update email provider: set secret failed", err)
		return
	}
	if err := rt.emailProviders.UpsertEmailProvider(ctx, req.toStore(existing)); err != nil {
		rt.writeProviderError(w, "api: update email provider failed", err)
		return
	}
	saved, err := rt.emailProviders.GetEmailProvider(ctx, id)
	if err != nil {
		rt.internalError(w, "api: update email provider: reload failed", err)
		return
	}
	res, err := rt.providerWithFlags(ctx, saved)
	if err != nil {
		rt.internalError(w, "api: update email provider: secrets check failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// handleDeleteEmailProvider handles DELETE /api/v1/settings/email/providers/{id}. The row goes first so a refused
// delete never costs the credentials.
func (rt *Router) handleDeleteEmailProvider(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")
	existing, err := rt.emailProviders.GetEmailProvider(ctx, id)
	if err != nil {
		rt.writeProviderError(w, "api: delete email provider: get failed", err)
		return
	}
	if err := rt.emailProviders.DeleteEmailProvider(ctx, id); err != nil {
		rt.writeProviderError(w, "api: delete email provider failed", err)
		return
	}
	if rt.emailSecrets != nil {
		rt.deleteProviderSecrets(ctx, existing)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (rt *Router) deleteProviderSecrets(ctx context.Context, p store.EmailProvider) {
	svc := store.EmailProviderSecretsKey(p.Role, p.ID)
	if p.Role != store.EmailRolePrimary {
		if err := rt.emailSecrets.DeleteAll(ctx, svc); err != nil {
			rt.logger.Warn("api: delete email provider secrets failed", slog.String("provider", p.ID), slog.String("error", err.Error()))
		}
		return
	}
	for _, f := range emailSecretFields {
		if err := rt.emailSecrets.DeleteValue(ctx, svc, f.envKey); err != nil {
			rt.logger.Warn("api: delete email provider secret failed", slog.String("provider", p.ID), slog.String("key", f.envKey), slog.String("error", err.Error()))
		}
	}
}

// handleTestEmailProvider handles POST /api/v1/settings/email/providers/{id}/test: one send through that provider
// alone, with no failover, enabled or not.
func (rt *Router) handleTestEmailProvider(w http.ResponseWriter, r *http.Request) {
	p, err := rt.emailProviders.GetEmailProvider(r.Context(), r.PathValue("id"))
	if err != nil {
		rt.writeProviderError(w, "api: test email provider: get failed", err)
		return
	}
	rt.sendTestEmail(w, r, p.ID)
}

type secretSlot struct {
	value   string
	present bool
}

func (rt *Router) readSlots(ctx context.Context, svc string) (map[string]secretSlot, error) {
	out := make(map[string]secretSlot, len(emailSecretFields))
	for _, f := range emailSecretFields {
		v, err := rt.emailSecrets.Resolve(ctx, svc, f.envKey)
		switch {
		case errors.Is(err, secrets.ErrValueNotFound):
			out[f.envKey] = secretSlot{}
		case err != nil:
			return nil, fmt.Errorf("read %s: %w", f.envKey, err)
		default:
			out[f.envKey] = secretSlot{value: v, present: true}
		}
	}
	return out, nil
}

func (rt *Router) writeSlots(ctx context.Context, svc string, slots map[string]secretSlot) error {
	for _, f := range emailSecretFields {
		s := slots[f.envKey]
		var err error
		if s.present {
			err = rt.emailSecrets.SetValue(ctx, svc, f.envKey, s.value)
		} else {
			err = rt.emailSecrets.DeleteValue(ctx, svc, f.envKey)
		}
		if err != nil {
			return fmt.Errorf("write %s: %w", f.envKey, err)
		}
	}
	return nil
}

// handlePromoteEmailProvider handles POST /api/v1/settings/email/providers/{id}/promote. The roles swap in one store
// transaction. The primary's credentials live under the shared namespace, so they move with the role: the old
// primary's go to its own namespace and the new primary's to the shared one, undone if the swap fails.
func (rt *Router) handlePromoteEmailProvider(w http.ResponseWriter, r *http.Request) {
	if rt.emailSecrets == nil {
		writeError(w, http.StatusNotImplemented, "email settings are not configured on this control plane (no master key set)")
		return
	}
	ctx := r.Context()
	target, err := rt.emailProviders.GetEmailProvider(ctx, r.PathValue("id"))
	if err != nil {
		rt.writeProviderError(w, "api: promote email provider: get failed", err)
		return
	}
	if target.Role == store.EmailRolePrimary {
		writeError(w, http.StatusConflict, "provider is already the primary")
		return
	}
	rows, err := rt.emailProviders.ListEmailProviders(ctx)
	if err != nil {
		rt.internalError(w, "api: promote email provider: list failed", err)
		return
	}
	var old *store.EmailProvider
	for i := range rows {
		if rows[i].Role == store.EmailRolePrimary {
			old = &rows[i]
		}
	}
	if old == nil {
		writeError(w, http.StatusConflict, "there is no primary to swap with")
		return
	}

	shared := store.EmailSettingsSecretsKey()
	oldNS := store.EmailProviderSecretsKey(store.EmailRoleBackup, old.ID)
	targetNS := store.EmailProviderSecretsKey(target.Role, target.ID)
	primarySlots, err := rt.readSlots(ctx, shared)
	if err != nil {
		rt.internalError(w, "api: promote email provider: read primary secrets failed", err)
		return
	}
	targetSlots, err := rt.readSlots(ctx, targetNS)
	if err != nil {
		rt.internalError(w, "api: promote email provider: read backup secrets failed", err)
		return
	}
	undo := func() {
		if err := rt.writeSlots(ctx, shared, primarySlots); err != nil {
			rt.logger.Error("api: promote email provider: restore primary secrets failed", slog.String("error", err.Error()))
		}
		if err := rt.emailSecrets.DeleteAll(ctx, oldNS); err != nil {
			rt.logger.Error("api: promote email provider: clear staged secrets failed", slog.String("error", err.Error()))
		}
	}
	if err := rt.writeSlots(ctx, oldNS, primarySlots); err != nil {
		undo()
		rt.internalError(w, "api: promote email provider: stage secrets failed", err)
		return
	}
	if err := rt.writeSlots(ctx, shared, targetSlots); err != nil {
		undo()
		rt.internalError(w, "api: promote email provider: move secrets failed", err)
		return
	}
	if err := rt.emailProviders.PromoteEmailProvider(ctx, target.ID); err != nil {
		undo()
		rt.writeProviderError(w, "api: promote email provider failed", err)
		return
	}
	if err := rt.emailSecrets.DeleteAll(ctx, targetNS); err != nil {
		rt.logger.Warn("api: promote email provider: clear old namespace failed", slog.String("provider", target.ID), slog.String("error", err.Error()))
	}
	fields := make([]string, 0, len(emailSecretFields))
	for _, f := range emailSecretFields {
		fields = append(fields, f.envKey)
	}
	rt.auditEmailSecretChange(r, target.ID, fields)

	saved, err := rt.emailProviders.ListEmailProviders(ctx)
	if err != nil {
		rt.internalError(w, "api: promote email provider: reload failed", err)
		return
	}
	out := emailProviderListResponse{Providers: make([]emailProviderResource, 0, len(saved))}
	for _, p := range saved {
		res, err := rt.providerWithFlags(ctx, p)
		if err != nil {
			rt.internalError(w, "api: promote email provider: secrets check failed", err)
			return
		}
		out.Providers = append(out.Providers, res)
	}
	writeJSON(w, http.StatusOK, out)
}
