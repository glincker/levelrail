package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// The two credential envKeys email settings are stored under in
// internal/secrets, never as plaintext settings-table columns.
const (
	emailSecretsSMTPPasswordKey    = "smtp_password"         //nolint:gosec // an envKey name, not a credential value
	emailSecretsSESSecretAccessKey = "ses_secret_access_key" //nolint:gosec // an envKey name, not a credential value
)

// EmailSecretsStore is the surface the email settings handlers need from
// internal/secrets.Manager. *secrets.Manager satisfies this structurally.
type EmailSecretsStore interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
}

// emailSettingsResource is the wire shape for GET and PUT
// /api/v1/settings/email. No credential value ever appears in a
// response: SMTPPasswordSet/SESSecretAccessKeySet report only whether
// one has been set. On a request, SMTPPassword/SESSecretAccessKey are
// write-only and optional: empty means "leave whatever is stored alone."
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
}

func toEmailSettingsResource(s store.EmailSettings, smtpPasswordSet, sesSecretSet bool) emailSettingsResource {
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

	var smtpPasswordSet, sesSecretSet bool
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
	}

	writeJSON(w, http.StatusOK, toEmailSettingsResource(settings, smtpPasswordSet, sesSecretSet))
}

var (
	errEmailBackendInvalid  = errors.New("backend must be one of \"\", \"smtp\", \"ses\"")
	errSMTPHostRequired     = errors.New("smtp_host is required when backend is smtp")
	errSMTPFromRequired     = errors.New("smtp_from is required when backend is smtp")
	errSMTPPortRequired     = errors.New("smtp_port is required when backend is smtp")
	errSESRegionRequired    = errors.New("ses_region is required when backend is ses")
	errSESFromRequired      = errors.New("ses_from is required when backend is ses")
	errSESAccessKeyRequired = errors.New("ses_access_key_id is required when backend is ses")
)

// validateEmailSettingsRequest enforces the structural fields a chosen
// backend needs. It does not require a credential even on first save:
// internal/email.NewSender surfaces a clear error at send time instead.
func validateEmailSettingsRequest(req emailSettingsResource) error {
	switch req.Backend {
	case "", store.EmailBackendSMTP, store.EmailBackendSES:
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
	return nil
}

// handleUpdateEmailSettings handles PUT /api/v1/settings/email,
// AbilityRoot-gated (router.go). Credentials are written to
// internal/secrets before the settings row, so a failed store write
// leaves only an orphaned, harmless secret rather than a settings row
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

	settings := store.EmailSettings{
		Backend:        req.Backend,
		SMTPHost:       req.SMTPHost,
		SMTPPort:       req.SMTPPort,
		SMTPUsername:   req.SMTPUsername,
		SMTPFrom:       req.SMTPFrom,
		SESRegion:      req.SESRegion,
		SESAccessKeyID: req.SESAccessKeyID,
		SESFrom:        req.SESFrom,
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
	writeJSON(w, http.StatusOK, toEmailSettingsResource(settings, smtpPasswordSet, sesSecretSet))
}
