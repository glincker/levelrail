package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/store"
)

// emailTestTimeout bounds the synchronous test-send below, the same
// reasoning notificationChannelTestTimeout documents for notification
// channels: an unresponsive-but-connectable backend can't hang the
// request goroutine indefinitely.
const emailTestTimeout = 10 * time.Second

// The credential envKeys email settings are stored under in
// internal/secrets, never as plaintext settings-table columns.
const (
	emailSecretsSMTPPasswordKey    = "smtp_password"         //nolint:gosec // an envKey name, not a credential value
	emailSecretsSESSecretAccessKey = "ses_secret_access_key" //nolint:gosec // an envKey name, not a credential value
	emailSecretsResendAPIKey       = "resend_api_key"        //nolint:gosec // an envKey name, not a credential value
)

// EmailSecretsStore is the surface the email settings handlers need from
// internal/secrets.Manager. *secrets.Manager satisfies this structurally.
type EmailSecretsStore interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
}

// emailSettingsResource is the wire shape for GET and PUT
// /api/v1/settings/email. No credential value ever appears in a
// response, only SMTPPasswordSet/SESSecretAccessKeySet booleans; on a
// PUT, an empty credential field means "leave whatever is stored alone."
type emailSettingsResource struct {
	Backend               string `json:"backend"`
	SMTPHost              string `json:"smtp_host,omitempty"`
	SMTPPort              int    `json:"smtp_port,omitempty"`
	SMTPUsername          string `json:"smtp_username,omitempty"`
	SMTPFrom              string `json:"smtp_from,omitempty"`
	SMTPPassword          string `json:"smtp_password,omitempty"`
	SMTPPasswordSet       bool   `json:"smtp_password_set,omitempty"`
	SESRegion             string `json:"ses_region,omitempty"`
	SESAccessKeyID        string `json:"ses_access_key_id,omitempty"`
	SESFrom               string `json:"ses_from,omitempty"`
	SESSecretAccessKey    string `json:"ses_secret_access_key,omitempty"`
	SESSecretAccessKeySet bool   `json:"ses_secret_access_key_set,omitempty"`
	ResendFrom            string `json:"resend_from,omitempty"`
	ResendAPIKey          string `json:"resend_api_key,omitempty"`
	ResendAPIKeySet       bool   `json:"resend_api_key_set,omitempty"`
}

func toEmailSettingsResource(s store.EmailSettings, smtpPasswordSet, sesSecretSet, resendKeySet bool) emailSettingsResource {
	return emailSettingsResource{
		Backend:               s.Backend,
		SMTPHost:              s.SMTPHost,
		SMTPPort:              s.SMTPPort,
		SMTPUsername:          s.SMTPUsername,
		SMTPFrom:              s.SMTPFrom,
		SMTPPasswordSet:       smtpPasswordSet,
		SESRegion:             s.SESRegion,
		SESAccessKeyID:        s.SESAccessKeyID,
		SESFrom:               s.SESFrom,
		SESSecretAccessKeySet: sesSecretSet,
		ResendFrom:            s.ResendFrom,
		ResendAPIKeySet:       resendKeySet,
	}
}

// handleGetEmailSettings handles GET /api/v1/settings/email. If
// rt.emailSecrets is nil (no master key configured), the "is set" flags
// simply report false rather than failing the request.
func (rt *Router) handleGetEmailSettings(w http.ResponseWriter, r *http.Request) {
	settings, err := rt.emailSettings.GetEmailSettings(r.Context())
	if err != nil {
		rt.logger.Error("api: get email settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	var smtpPasswordSet, sesSecretSet, resendKeySet bool
	if rt.emailSecrets != nil {
		key := store.EmailSettingsSecretsKey()
		smtpPasswordSet, err = rt.emailSecrets.Exists(r.Context(), key, emailSecretsSMTPPasswordKey)
		if err != nil {
			rt.logger.Error("api: get email settings: check smtp password failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		sesSecretSet, err = rt.emailSecrets.Exists(r.Context(), key, emailSecretsSESSecretAccessKey)
		if err != nil {
			rt.logger.Error("api: get email settings: check ses secret failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
		resendKeySet, err = rt.emailSecrets.Exists(r.Context(), key, emailSecretsResendAPIKey)
		if err != nil {
			rt.logger.Error("api: get email settings: check resend api key failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	writeJSON(w, http.StatusOK, toEmailSettingsResource(settings, smtpPasswordSet, sesSecretSet, resendKeySet))
}

var (
	errEmailBackendInvalid  = errors.New("backend must be one of \"\", \"smtp\", \"ses\", \"resend\"")
	errSMTPHostRequired     = errors.New("smtp_host is required when backend is smtp")
	errSMTPFromRequired     = errors.New("smtp_from is required when backend is smtp")
	errSMTPPortRequired     = errors.New("smtp_port is required when backend is smtp")
	errSESRegionRequired    = errors.New("ses_region is required when backend is ses")
	errSESFromRequired      = errors.New("ses_from is required when backend is ses")
	errSESAccessKeyRequired = errors.New("ses_access_key_id is required when backend is ses")
	errResendFromRequired   = errors.New("resend_from is required when backend is resend")
)

// validateEmailSettingsRequest enforces the structural fields a chosen
// backend needs. It does not require a credential even on first save:
// internal/email.NewSender surfaces a clear error at send time instead.
func validateEmailSettingsRequest(req emailSettingsResource) error {
	switch req.Backend {
	case "", store.EmailBackendSMTP, store.EmailBackendSES, store.EmailBackendResend:
	default:
		return errEmailBackendInvalid
	}
	if req.Backend == store.EmailBackendSMTP {
		if req.SMTPHost == "" {
			return errSMTPHostRequired
		}
		if req.SMTPFrom == "" {
			return errSMTPFromRequired
		}
		if req.SMTPPort == 0 {
			return errSMTPPortRequired
		}
	}
	if req.Backend == store.EmailBackendSES {
		if req.SESRegion == "" {
			return errSESRegionRequired
		}
		if req.SESFrom == "" {
			return errSESFromRequired
		}
		if req.SESAccessKeyID == "" {
			return errSESAccessKeyRequired
		}
	}
	if req.Backend == store.EmailBackendResend && req.ResendFrom == "" {
		return errResendFromRequired
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
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := validateEmailSettingsRequest(req); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	key := store.EmailSettingsSecretsKey()
	if req.SMTPPassword != "" {
		if err := rt.emailSecrets.SetValue(r.Context(), key, emailSecretsSMTPPasswordKey, req.SMTPPassword); err != nil {
			rt.logger.Error("api: update email settings: set smtp password failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if req.SESSecretAccessKey != "" {
		if err := rt.emailSecrets.SetValue(r.Context(), key, emailSecretsSESSecretAccessKey, req.SESSecretAccessKey); err != nil {
			rt.logger.Error("api: update email settings: set ses secret failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}
	if req.ResendAPIKey != "" {
		if err := rt.emailSecrets.SetValue(r.Context(), key, emailSecretsResendAPIKey, req.ResendAPIKey); err != nil {
			rt.logger.Error("api: update email settings: set resend api key failed", slog.String("error", err.Error()))
			writeError(w, http.StatusInternalServerError, "internal error")
			return
		}
	}

	settings := store.EmailSettings{
		Backend:        req.Backend,
		SMTPHost:       req.SMTPHost,
		SMTPPort:       req.SMTPPort,
		SMTPUsername:   req.SMTPUsername,
		SMTPFrom:       req.SMTPFrom,
		SESRegion:      req.SESRegion,
		SESAccessKeyID: req.SESAccessKeyID,
		SESFrom:        req.SESFrom,
		ResendFrom:     req.ResendFrom,
	}
	if err := rt.emailSettings.UpdateEmailSettings(r.Context(), settings); err != nil {
		rt.logger.Error("api: update email settings failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}

	smtpPasswordSet, err := rt.emailSecrets.Exists(r.Context(), key, emailSecretsSMTPPasswordKey)
	if err != nil {
		rt.logger.Error("api: update email settings: check smtp password failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	sesSecretSet, err := rt.emailSecrets.Exists(r.Context(), key, emailSecretsSESSecretAccessKey)
	if err != nil {
		rt.logger.Error("api: update email settings: check ses secret failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	resendKeySet, err := rt.emailSecrets.Exists(r.Context(), key, emailSecretsResendAPIKey)
	if err != nil {
		rt.logger.Error("api: update email settings: check resend api key failed", slog.String("error", err.Error()))
		writeError(w, http.StatusInternalServerError, "internal error")
		return
	}
	writeJSON(w, http.StatusOK, toEmailSettingsResource(settings, smtpPasswordSet, sesSecretSet, resendKeySet))
}

type testEmailRequest struct {
	To string `json:"to"`
}

// handleTestEmail handles POST /api/v1/settings/email/test: sends one
// real email through rt.emailSender, the exact same DynamicSender
// alert notifications and password resets already use, so a successful
// test proves the real send path works, not a separate one that could
// drift from it.
func (rt *Router) handleTestEmail(w http.ResponseWriter, r *http.Request) {
	if rt.emailSender == nil {
		writeError(w, http.StatusNotImplemented, "email is not configured on this control plane")
		return
	}

	var req testEmailRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := email.ValidateAddress(req.To); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), emailTestTimeout)
	defer cancel()
	subject := "Test email from Levelrail"
	body := "This is a test email from your Levelrail control plane. If you received this, your email settings are working."
	if err := rt.emailSender.Send(ctx, req.To, subject, body); err != nil {
		writeError(w, http.StatusBadGateway, fmt.Sprintf("test email failed: %s", err.Error()))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
