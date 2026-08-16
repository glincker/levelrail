package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
)

// EmailSettings' internal/secrets envKeys: the two credential fields
// that never appear in a settings-table column or an API response, the
// same "credential goes through internal/secrets, never a plaintext
// column" reasoning store.BackupTargetSecretsKey's own doc comment
// establishes for a backup target's credentials.
const (
	emailSecretsSMTPPasswordKey    = "smtp_password"         //nolint:gosec // an internal/secrets envKey name, not a credential value
	emailSecretsSESSecretAccessKey = "ses_secret_access_key" //nolint:gosec // an internal/secrets envKey name, not a credential value
)

// EmailSecretsStore is the surface the email settings handlers need from
// internal/secrets.Manager: set a credential value, and check whether
// one is already set, without ever reading its plaintext back through
// this interface. *secrets.Manager satisfies this structurally, the same
// "narrow, consumer-defined boundary" shape BackupSecretsSetter already
// establishes.
type EmailSecretsStore interface {
	SetValue(ctx context.Context, serviceName, envKey, plaintext string) error
	Exists(ctx context.Context, serviceName, envKey string) (bool, error)
}

// emailSettingsResource is the wire shape for GET and PUT
// /api/v1/settings/email. No credential value ever appears here, request
// or response: SMTPPasswordSet/SESSecretAccessKeySet on the response
// side report only whether one has been set, the same "settings form
// needs to show connection is configured without ever redisplaying the
// secret" precedent backup_targets.go's own doc comment establishes for
// createBackupTargetRequest. On the request side, SMTPPassword/
// SESSecretAccessKey are write-only: present and non-empty means "set
// or replace this credential," empty or omitted means "leave whatever is
// already stored alone" (see handleUpdateEmailSettings), so an operator
// tweaking the SMTP host doesn't have to re-type a password on every
// save.
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

// handleGetEmailSettings handles GET /api/v1/settings/email. Never
// returns a credential's value, only whether one is set: if rt.emailSecrets
// is nil (no master key configured on this control plane), both "is set"
// flags simply report false, the same "optional signal, absence is not
// an error" shape WithDockerPinger's own absence already has, rather
// than failing the whole request.
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
// backend needs to function at all. It deliberately does not require the
// credential fields (SMTPPassword/SESSecretAccessKey) even on the very
// first save: internal/email.NewSender surfaces a clear error at send
// time if a backend is selected with no credential ever stored, the same
// "always succeeds at the store layer, the first real send is what
// actually proves it works" boundary validateCreateBackupTargetRequest's
// own doc comment describes for a bucket's credentials.
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

// handleUpdateEmailSettings handles PUT /api/v1/settings/email.
// AbilityRoot (router.go's own registration comment): this is real
// infrastructure configuration, the same blast-radius tier PUT
// /api/v1/settings/ingress already occupies.
//
// Credentials are written to internal/secrets before the settings row,
// the same ordering handleCreateBackupTarget's own doc comment explains
// and for the identical reason: if the store write then fails, the
// result is an orphaned, harmless secret rather than a settings row that
// looks configured but has no working credential behind it.
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
