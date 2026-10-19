package api

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	defaultMyActivityLimit = 25
	maxMyActivityLimit     = 100
)

// Categories a feed row can fall into, also the ?kind= filter values.
const (
	activityKindSignIn   = "sign_in"
	activityKindSecurity = "security"
	activityKindSessions = "sessions"
	activityKindTokens   = "tokens"
	activityKindTokenUse = "token_use"
	activityKindAccount  = "account"
	activityKindFailed   = "failed"
	activityKindOther    = "other"
)

var activityKindOfEvent = map[string]string{
	accountEventSignIn:           activityKindSignIn,
	accountEventPasswordChanged:  activityKindSecurity,
	accountEvent2FAEnabled:       activityKindSecurity,
	accountEvent2FADisabled:      activityKindSecurity,
	accountEventRecoveryCodes:    activityKindSecurity,
	accountEventPasskeyAdded:     activityKindSecurity,
	accountEventPasskeyRemoved:   activityKindSecurity,
	accountEventPasskeyRenamed:   activityKindSecurity,
	accountEventSessionsRevoked:  activityKindSessions,
	accountEventSessionRevoked:   activityKindSessions,
	accountEventDeviceRemoved:    activityKindSessions,
	accountEventTokenCreated:     activityKindTokens,
	accountEventProfileUpdated:   activityKindAccount,
	accountEventPrefsUpdated:     activityKindAccount,
	accountEventExportRequested:  activityKindAccount,
	accountEventExportDownloaded: activityKindAccount,
	accountEventProviderUnlinked: activityKindSecurity,
	accountEventEmailRequested:   activityKindAccount,
	accountEventEmailChanged:     activityKindAccount,
	accountEventDeleted:          activityKindAccount,
	auditActionStepUpVerified:    activityKindSecurity,
	auditActionStepUpFailed:      activityKindSecurity,
	auditActionStepUpLocked:      activityKindSecurity,
	auditActionStepUpNeedCode:    activityKindSecurity,
}

type myActivityEntry struct {
	ID         string    `json:"id"`
	At         time.Time `json:"at"`
	Kind       string    `json:"kind"`
	Event      string    `json:"event"`
	ActorType  string    `json:"actor_type"`
	TokenName  string    `json:"token_name,omitempty"`
	Method     string    `json:"method"`
	Path       string    `json:"path"`
	StatusCode int       `json:"status_code"`
	RemoteAddr string    `json:"remote_addr"`
	ClientKind string    `json:"client_kind"`
}

type myActivityResponse struct {
	Entries    []myActivityEntry `json:"entries"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

// activityEventAndKind names a row: its stored action when it has one, else
// the route it came from, else a plain API request.
func activityEventAndKind(e store.AuditEntry) (event, kind string) {
	event = e.Action
	if event == "" {
		if a, ok := matchAccountEventRoute(e.Method, e.Path); ok {
			event = a
		} else if e.Method == http.MethodDelete && strings.HasPrefix(e.Path, "/api/v1/auth/tokens/") {
			return "account.token_revoked", activityKindTokens
		}
	}
	if k, ok := activityKindOfEvent[event]; ok {
		return event, k
	}
	if e.ActorType == "token" {
		return event, activityKindTokenUse
	}
	return event, activityKindOther
}

// securityActivityActionPrefixes is the default feed: sign-ins, credential,
// session, token, provider, email, export and delete events, no raw requests.
var securityActivityActionPrefixes = []string{
	accountEventSignIn, accountEventPasswordChanged, "account.2fa_", "account.recovery_", "account.passkey_",
	"account.session", "account.trusted_device", accountEventTokenCreated, accountEventProviderUnlinked,
	"account.email_", "account.export_", accountEventDeleted, "step_up.",
}

func applyActivityKind(f *store.AuditEntryFilter, kind string) bool {
	switch kind {
	case "":
		f.ActionPrefixes = securityActivityActionPrefixes
		f.PathLike = []string{"/api/v1/auth/tokens/%"}
		f.ActionOrPath = true
	case "all":
	case activityKindSignIn:
		f.Action = accountEventSignIn
	case activityKindSecurity:
		f.ActionPrefixes = []string{accountEventPasswordChanged, "account.2fa_", "account.recovery_", "account.passkey_", accountEventProviderUnlinked, "step_up."}
	case activityKindSessions:
		f.ActionPrefixes = []string{"account.session", "account.trusted_device"}
	case activityKindTokens:
		f.PathLike = []string{"/api/v1/auth/tokens%"}
	case activityKindTokenUse:
		f.ActorType = "token"
	case activityKindAccount:
		f.ActionPrefixes = []string{"account.profile_", "account.preferences_", "account.export_", "account.email_", accountEventDeleted}
	case activityKindFailed:
		f.FailedOnly = true
	default:
		return false
	}
	return true
}

// handleMyActivity handles GET /api/v1/auth/my-activity: only rows the caller
// itself caused, newest first. Without ?kind it lists security events only;
// ?kind=all adds raw API requests.
func (rt *Router) handleMyActivity(w http.ResponseWriter, r *http.Request) {
	userID, _, ok := rt.accountCaller(w, r, AbilityReadSensitive)
	if !ok {
		return
	}
	q := r.URL.Query()
	limit := defaultMyActivityLimit
	if raw := q.Get("limit"); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 || n > maxMyActivityLimit {
			writeError(w, http.StatusBadRequest, "limit must be 1 to "+strconv.Itoa(maxMyActivityLimit))
			return
		}
		limit = n
	}
	var before *time.Time
	if raw := q.Get("cursor"); raw != "" {
		t, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "cursor must be a value returned by this endpoint")
			return
		}
		before = &t
	}
	filter := store.AuditEntryFilter{OwnerUserID: userID}
	if !applyActivityKind(&filter, q.Get("kind")) {
		writeError(w, http.StatusBadRequest, "unknown kind")
		return
	}
	rows, err := rt.auditLog.ListAuditEntries(r.Context(), limit+1, before, filter)
	if err != nil {
		rt.internalError(w, "api: my activity failed", err)
		return
	}
	out := myActivityResponse{Entries: []myActivityEntry{}}
	if len(rows) > limit {
		rows = rows[:limit]
		out.NextCursor = rows[len(rows)-1].CreatedAt
	}
	for _, e := range rows {
		out.Entries = append(out.Entries, toMyActivityEntry(e))
	}
	writeJSON(w, http.StatusOK, out)
}

func toMyActivityEntry(e store.AuditEntry) myActivityEntry {
	event, kind := activityEventAndKind(e)
	at, _ := time.Parse(time.RFC3339Nano, e.CreatedAt)
	entry := myActivityEntry{
		ID: e.ID, At: at, Kind: kind, Event: event, ActorType: e.ActorType,
		Method: e.Method, Path: e.Path, StatusCode: e.StatusCode,
		RemoteAddr: e.RemoteAddr, ClientKind: e.ClientKind,
	}
	if e.ActorType == "token" {
		entry.TokenName = e.ActorName
	}
	return entry
}
