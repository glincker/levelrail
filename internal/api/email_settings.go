package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
)

// EmailSecretsStore is the surface the email settings handlers need from
// internal/secrets.Manager. *secrets.Manager satisfies this structurally.
type EmailSecretsStore interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
}

// emailSettingsResource is the wire shape for GET and PUT
// /api/v1/settings/email. No credential value ever appears in a
// response, only the *_set booleans; on a PUT, an empty credential field
// means "leave whatever is stored alone."
type emailSettingsResource struct {
	Backend string `json:"backend"`

	FromName     string `json:"from_name,omitempty"`
	FromAddress  string `json:"from_address,omitempty"`
	ReplyTo      string `json:"reply_to,omitempty"`
	EnvelopeFrom string `json:"envelope_from,omitempty"`

	SendsPerMinute int `json:"sends_per_minute,omitempty"`
	SendsPerDay    int `json:"sends_per_day,omitempty"`

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

	// SESRegions is the registry the region dropdown renders; read-only.
	SESRegions []string `json:"ses_regions,omitempty"`
}

// emailSecretField ties one write-only request field and its *_set flag to the envKey it is stored under.
type emailSecretField struct {
	envKey string
	value  func(*emailSettingsResource) string
	mark   func(*emailSettingsResource, bool)
}

var emailSecretFields = []emailSecretField{
	{store.EmailSecretSMTPPassword, func(r *emailSettingsResource) string { return r.SMTPPassword }, func(r *emailSettingsResource, v bool) { r.SMTPPasswordSet = v }},
	{store.EmailSecretSESSecretKey, func(r *emailSettingsResource) string { return r.SESSecretAccessKey }, func(r *emailSettingsResource, v bool) { r.SESSecretAccessKeySet = v }},
	{store.EmailSecretSESSMTPPassword, func(r *emailSettingsResource) string { return r.SESSMTPPassword }, func(r *emailSettingsResource, v bool) { r.SESSMTPPasswordSet = v }},
	{store.EmailSecretResendAPIKey, func(r *emailSettingsResource) string { return r.ResendAPIKey }, func(r *emailSettingsResource, v bool) { r.ResendAPIKeySet = v }},
	{store.EmailSecretPostmarkToken, func(r *emailSettingsResource) string { return r.PostmarkServerToken }, func(r *emailSettingsResource, v bool) { r.PostmarkServerTokenSet = v }},
	{store.EmailSecretSendGridAPIKey, func(r *emailSettingsResource) string { return r.SendGridAPIKey }, func(r *emailSettingsResource, v bool) { r.SendGridAPIKeySet = v }},
	{store.EmailSecretMailgunAPIKey, func(r *emailSettingsResource) string { return r.MailgunAPIKey }, func(r *emailSettingsResource, v bool) { r.MailgunAPIKeySet = v }},
}

func toEmailSettingsResource(s store.EmailSettings) emailSettingsResource {
	return emailSettingsResource{
		Backend:         s.Backend,
		FromName:        s.FromName,
		FromAddress:     s.FromAddress,
		ReplyTo:         s.ReplyTo,
		EnvelopeFrom:    s.EnvelopeFrom,
		SendsPerMinute:  s.SendsPerMinute,
		SendsPerDay:     s.SendsPerDay,
		TLSMode:         s.TLSMode,
		TLSSkipVerify:   s.TLSSkipVerify,
		SMTPHost:        s.SMTPHost,
		SMTPPort:        s.SMTPPort,
		SMTPUsername:    s.SMTPUsername,
		SMTPFrom:        s.SMTPFrom,
		SESRegion:       s.SESRegion,
		SESAccessKeyID:  s.SESAccessKeyID,
		SESFrom:         s.SESFrom,
		SESSMTPPort:     s.SESSMTPPort,
		SESSMTPUsername: s.SESSMTPUsername,
		ResendFrom:      s.ResendFrom,
		MailgunDomain:   s.MailgunDomain,
		MailgunRegion:   s.MailgunRegion,
		SESRegions:      email.SESRegions,
	}
}

// withSecretFlags fills each *_set flag from internal/secrets; with no secrets store every flag stays false.
func (rt *Router) withSecretFlags(ctx context.Context, res emailSettingsResource) (emailSettingsResource, error) {
	if rt.emailSecrets == nil {
		return res, nil
	}
	for _, f := range emailSecretFields {
		ok, err := rt.emailSecrets.Exists(ctx, store.EmailSettingsSecretsKey(), f.envKey)
		if err != nil {
			return res, fmt.Errorf("check %s: %w", f.envKey, err)
		}
		f.mark(&res, ok)
	}
	return res, nil
}

// handleGetEmailSettings handles GET /api/v1/settings/email. If
// rt.emailSecrets is nil (no master key configured), the "is set" flags
// simply report false rather than failing the request.
func (rt *Router) handleGetEmailSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.emailSettings.GetEmailSettings(r.Context())
	if err != nil {
		rt.internalError(w, "api: get email settings failed", err)
		return
	}
	res, err := rt.withSecretFlags(r.Context(), toEmailSettingsResource(settings))
	if err != nil {
		rt.internalError(w, "api: get email settings: secrets check failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

var (
	errEmailBackendInvalid  = errors.New("backend must be one of \"\", \"smtp\", \"ses\", \"ses_smtp\", \"resend\", \"postmark\", \"sendgrid\", \"mailgun\"")
	errSMTPHostRequired     = errors.New("smtp_host is required when backend is smtp")
	errSMTPFromRequired     = errors.New("from_address is required when backend is smtp")
	errSMTPPortRequired     = errors.New("smtp_port is required when backend is smtp")
	errSESRegionInvalid     = errors.New("ses_region must be an AWS SES region such as us-east-1, not a hostname")
	errSESFromRequired      = errors.New("from_address is required when backend is ses")
	errSESAccessKeyRequired = errors.New("ses_access_key_id is required when backend is ses")
	errSESSMTPUserRequired  = errors.New("ses_smtp_username is required when backend is ses_smtp")
	errSESSMTPPortInvalid   = errors.New("ses_smtp_port must be 25, 465, 587, 2465 or 2587")
	errMailgunDomain        = errors.New("mailgun_domain is required when backend is mailgun")
	errMailgunRegion        = errors.New("mailgun_region must be \"us\" or \"eu\"")
	errFromRequired         = errors.New("from_address is required")
	errTLSModeInvalid       = errors.New("tls_mode must be one of \"\", \"starttls\", \"tls\", \"none\"")
	errLimitNegative        = errors.New("sends_per_minute and sends_per_day must not be negative")
)

// emailFrom is the sender address the request would use for its chosen backend.
func (req emailSettingsResource) emailFrom() string {
	s := store.EmailSettings{Backend: req.Backend, FromAddress: req.FromAddress, SMTPFrom: req.SMTPFrom, SESFrom: req.SESFrom, ResendFrom: req.ResendFrom}
	return s.EffectiveFrom()
}

// validateEmailSettingsRequest enforces the structural fields a chosen
// backend needs. It does not require a credential even on first save:
// internal/email.NewSender surfaces a clear error at send time instead.
func validateEmailSettingsRequest(req emailSettingsResource) error {
	if req.Backend != "" && !email.ValidBackend(email.Backend(req.Backend)) {
		return errEmailBackendInvalid
	}
	if !slices.Contains([]string{"", email.TLSStartTLS, email.TLSImplicit, email.TLSNone}, req.TLSMode) {
		return errTLSModeInvalid
	}
	if req.SendsPerMinute < 0 || req.SendsPerDay < 0 {
		return errLimitNegative
	}
	for _, a := range []string{req.FromAddress, req.ReplyTo, req.EnvelopeFrom} {
		if a != "" {
			if err := email.ValidateAddress(a); err != nil {
				return err
			}
		}
	}
	if strings.ContainsAny(req.FromName, "\r\n") {
		return errors.New("from_name must not contain a newline")
	}
	if req.Backend == "" {
		return nil
	}
	if req.emailFrom() == "" {
		switch req.Backend {
		case store.EmailBackendSMTP:
			return errSMTPFromRequired
		case store.EmailBackendSES, store.EmailBackendSESSMTP:
			return errSESFromRequired
		default:
			return errFromRequired
		}
	}
	switch req.Backend {
	case store.EmailBackendSMTP:
		if req.SMTPHost == "" {
			return errSMTPHostRequired
		}
		if req.SMTPPort == 0 {
			return errSMTPPortRequired
		}
	case store.EmailBackendSES:
		if !email.ValidSESRegion(req.SESRegion) {
			return errSESRegionInvalid
		}
		if req.SESAccessKeyID == "" {
			return errSESAccessKeyRequired
		}
	case store.EmailBackendSESSMTP:
		if !email.ValidSESRegion(req.SESRegion) {
			return errSESRegionInvalid
		}
		if req.SESSMTPUsername == "" {
			return errSESSMTPUserRequired
		}
		if req.SESSMTPPort != 0 && !slices.Contains([]int{25, 465, 587, 2465, 2587}, req.SESSMTPPort) {
			return errSESSMTPPortInvalid
		}
	case store.EmailBackendMailgun:
		if req.MailgunDomain == "" {
			return errMailgunDomain
		}
		if req.MailgunRegion != "" && req.MailgunRegion != email.MailgunRegionUS && req.MailgunRegion != email.MailgunRegionEU {
			return errMailgunRegion
		}
	}
	return nil
}

// handleUpdateEmailSettings handles PUT /api/v1/settings/email.
// Credentials are written to internal/secrets before the settings row,
// so a failed store write leaves an orphaned secret, not a settings row
// that looks configured but isn't.
func (rt *Router) handleUpdateEmailSettings(w http.ResponseWriter, r *http.Request) {
	if rt.emailSecrets == nil {
		writeError(w, http.StatusNotImplemented, "email settings are not configured on this control plane (no master key set)")
		return
	}

	var req emailSettingsResource
	if !decodeReq(w, r, &req) {
		return
	}
	if err := validateEmailSettingsRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	key := store.EmailSettingsSecretsKey()
	for _, f := range emailSecretFields {
		v := f.value(&req)
		if v == "" {
			continue
		}
		if err := rt.emailSecrets.SetValue(r.Context(), key, f.envKey, v); err != nil {
			rt.internalError(w, "api: update email settings: set secret failed", err, slog.String("key", f.envKey))
			return
		}
	}

	mailgunRegion := req.MailgunRegion
	if mailgunRegion == "" {
		mailgunRegion = email.MailgunRegionUS
	}
	sesSMTPPort := req.SESSMTPPort
	if sesSMTPPort == 0 {
		sesSMTPPort = 587
	}
	settings := store.EmailSettings{
		Backend:         req.Backend,
		SMTPHost:        req.SMTPHost,
		SMTPPort:        req.SMTPPort,
		SMTPUsername:    req.SMTPUsername,
		SMTPFrom:        req.SMTPFrom,
		SESRegion:       req.SESRegion,
		SESAccessKeyID:  req.SESAccessKeyID,
		SESFrom:         req.SESFrom,
		ResendFrom:      req.ResendFrom,
		FromName:        req.FromName,
		FromAddress:     req.FromAddress,
		ReplyTo:         req.ReplyTo,
		EnvelopeFrom:    req.EnvelopeFrom,
		SendsPerMinute:  req.SendsPerMinute,
		SendsPerDay:     req.SendsPerDay,
		TLSMode:         req.TLSMode,
		TLSSkipVerify:   req.TLSSkipVerify,
		SESSMTPPort:     sesSMTPPort,
		SESSMTPUsername: req.SESSMTPUsername,
		MailgunDomain:   req.MailgunDomain,
		MailgunRegion:   mailgunRegion,
	}
	if err := rt.emailSettings.UpdateEmailSettings(r.Context(), settings); err != nil {
		rt.internalError(w, "api: update email settings failed", err)
		return
	}

	res, err := rt.withSecretFlags(r.Context(), toEmailSettingsResource(settings))
	if err != nil {
		rt.internalError(w, "api: update email settings: secrets check failed", err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}
