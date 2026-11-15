package api

import (
	"context"
	"log/slog"
	"net/http"
	"net/mail"
	"os"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/store"
)

const (
	envApprovalWithoutSession = "APP_AUTH_APPROVAL_WITHOUT_SESSION"

	approvalWithoutSessionAllow = "allow"
	approvalWithoutSessionHold  = "hold"

	approvalPolicyStepUpApp = "security-policy"

	errCodeApprovalHoldNoChannel = "approval_hold_no_channel"

	approvalChannelApproveToken = "approve_token"
	approvalChannelHostCommand  = "host_command"
)

func validApprovalWithoutSession(s string) bool {
	return s == approvalWithoutSessionAllow || s == approvalWithoutSessionHold
}

// envApprovalWithoutSessionDefault is the seed for a fresh install; it never overrides a saved value.
func envApprovalWithoutSessionDefault() (string, bool) {
	v := strings.TrimSpace(os.Getenv(envApprovalWithoutSession))
	if validApprovalWithoutSession(v) {
		return v, true
	}
	return approvalWithoutSessionAllow, false
}

// approvalModeMemo keeps the last policy value read, so a store error never
// flips the mode an install chose.
type approvalModeMemo struct {
	mu    sync.Mutex
	value string
}

var approvalModes sync.Map // *Router -> *approvalModeMemo

func (rt *Router) approvalModeMemo() *approvalModeMemo {
	m, _ := approvalModes.LoadOrStore(rt, &approvalModeMemo{})
	memo, _ := m.(*approvalModeMemo)
	return memo
}

func (m *approvalModeMemo) remember(v string) {
	m.mu.Lock()
	m.value = v
	m.mu.Unlock()
}

func (m *approvalModeMemo) last() (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.value, m.value != ""
}

// holdWithoutLiveSession reports whether a sign-in with no other live session
// is held: the platform mode is hold, or the account asked for approval itself.
func (rt *Router) holdWithoutLiveSession(ctx context.Context, userID string) bool {
	mode := ""
	p, _, err := rt.effectiveSecurityPolicy(ctx)
	if err == nil {
		mode = p.ApprovalWithoutSession
		rt.approvalModeMemo().remember(mode)
	} else {
		last, ok := rt.approvalModeMemo().last()
		rt.logger.Warn("api: approval policy unreadable, using last good value", slog.Bool("had_value", ok), slog.String("error", err.Error()))
		if ok {
			mode = last
		} else if seed, fromEnv := envApprovalWithoutSessionDefault(); fromEnv {
			mode = seed
		}
	}
	if mode == approvalWithoutSessionHold {
		return true
	}
	if rt.security == nil {
		return false
	}
	us, uerr := rt.security.GetUserSecuritySettings(ctx, userID)
	return uerr == nil && us.RequireNewDeviceApproval
}

// approvalChannels lists the out-of-band approval paths for a held sign-in
// that userID could use. An approval is bound to the account that signed in,
// so only that account's own signin:approve token counts; "" means any admin's
// (the doctor view). The host command always works, so it is listed last and
// never counts alone.
func (rt *Router) approvalChannels(ctx context.Context, userID string) []string {
	var out []string
	if rt.adminApproveTokenLive(ctx, userID) {
		out = append(out, approvalChannelApproveToken)
	}
	return append(out, approvalChannelHostCommand)
}

func (rt *Router) adminIDs(ctx context.Context) map[string]string {
	out := map[string]string{}
	if rt.auth == nil {
		return out
	}
	users, err := rt.auth.ListUsers(ctx)
	if err != nil {
		return out
	}
	for _, u := range users {
		if hasAbility(u.Abilities, AbilityRoot) {
			out[u.ID] = u.Email
		}
	}
	return out
}

func (rt *Router) adminApproveTokenLive(ctx context.Context, userID string) bool {
	toks, err := rt.libraryListTokens(ctx, "", true)
	if err != nil {
		return false
	}
	admins := rt.adminIDs(ctx)
	now := time.Now()
	for _, t := range toks {
		if t.RevokedAt != nil || (t.ExpiresAt != nil && !now.Before(*t.ExpiresAt)) {
			continue
		}
		if userID != "" && t.OwnerUserID != userID {
			continue
		}
		if _, ok := admins[t.OwnerUserID]; ok && slices.Contains(t.Abilities, AbilitySignInApprove) {
			return true
		}
	}
	return false
}

// applyApprovalWithoutSession runs the safeguards for a mode change and
// reports whether the caller may save it; it writes the refusal itself.
func (rt *Router) applyApprovalWithoutSession(w http.ResponseWriter, r *http.Request, req securityPolicyRequest, current string) (string, bool) {
	want := *req.ApprovalWithoutSession
	if want == current {
		return "", true
	}
	owner, ok := rt.stepUpOwner(w, r, req.StepUp, approvalPolicyStepUpApp, "change how sign-ins without a session are handled", "this change")
	if !ok {
		return "", false
	}
	if want == approvalWithoutSessionHold && !req.ConfirmHostAccess {
		if !slices.Contains(rt.approvalChannels(r.Context(), owner), approvalChannelApproveToken) {
			writeCodedError(w, http.StatusConflict, errCodeApprovalHoldNoChannel,
				"hold needs a way to approve a sign-in from outside: create a signin:approve token you own, or confirm you can run commands on the server")
			return "", false
		}
	}
	return owner, true
}

// afterApprovalModeChange keeps the acting device trusted and tells every admin.
func (rt *Router) afterApprovalModeChange(w http.ResponseWriter, r *http.Request, owner, mode string) {
	if _, ok := rt.currentSessionUserID(r); ok {
		rt.trustDevice(w, r, owner)
	}
	rt.approvalModeMemo().remember(mode)
	actorID, actorKind := rt.securityActor(r)
	rt.auditSignIn(r.Context(), r, signInActor{kind: actorKind, id: actorID, name: rt.auditActorName(r.Context(), actorKind, actorID, "")},
		store.AuditActionSecurityPolicyUpdate, "/api/v1/security/policy#approval_without_session="+mode, http.StatusOK)
	if rt.emailSender == nil {
		return
	}
	admins := rt.adminIDs(r.Context())
	ctx := context.WithoutCancel(r.Context())
	go func() {
		ctx, cancel := context.WithTimeout(ctx, libMailTimeout)
		defer cancel()
		subject := rt.brand.Name + ": sign-in approval setting changed"
		body := "An admin set sign-ins without another live session to \"" + mode + "\" on your " + rt.brand.Name +
			" control plane. If this was not expected, review the audit log. To undo it from the server, run the host command security reset-approval-policy."
		for _, addr := range admins {
			if _, err := mail.ParseAddress(addr); err != nil {
				continue
			}
			if err := rt.emailSender.Send(ctx, addr, subject, body); err != nil {
				rt.logger.Warn("api: approval policy change email failed", slog.String("error", err.Error()))
			}
		}
	}()
}

// doctorCheckApprovalWithoutSession shows the mode and which approval paths exist.
func (rt *Router) doctorCheckApprovalWithoutSession(ctx context.Context) doctorCheckResource {
	const code, name = "approval_without_session", "Sign-ins without a live session"
	p, _, err := rt.effectiveSecurityPolicy(ctx)
	if err != nil {
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusUnknown, Message: "could not read the security policy"}
	}
	channels := rt.approvalChannels(ctx, "")
	if p.ApprovalWithoutSession == approvalWithoutSessionHold {
		note := rt.approvalHoldNote(p)
		if len(channels) == 1 {
			return doctorCheckResource{Code: code, Name: name, Status: doctorStatusWarn, DocsPath: "/security-center#approval-without-session",
				Message: "mode hold: only the host command can approve a sign-in that has no other live session; create a signin:approve token. " + note}
		}
		return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "mode hold, approval paths: " + strings.Join(channels, ", ") + ". " + note}
	}
	return doctorCheckResource{Code: code, Name: name, Status: doctorStatusOK, Message: "mode allow: a correct password with no other live session signs in, unless the account asks for approval"}
}

// approvalHoldNote says where hold does not apply, so a saved mode is never read as a blanket guarantee.
func (rt *Router) approvalHoldNote(p securityPolicy) string {
	var parts []string
	if !rt.newDeviceApproval {
		parts = append(parts, "new-device approval is off, so hold has no effect")
	}
	if p.ApprovalScope == approvalScopePasswordOnly {
		parts = append(parts, "approval scope is password_only, so SSO and passkey sign-ins are not held")
	}
	parts = append(parts, "browsers already trusted skip the hold")
	return strings.Join(parts, "; ")
}
