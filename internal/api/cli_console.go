package api

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"slices"
	"time"

	"github.com/coder/websocket"

	"github.com/GLINCKER/levelrail/internal/authengine"
	"github.com/GLINCKER/levelrail/internal/cliconsole"
	"github.com/GLINCKER/levelrail/internal/experimental"
	"github.com/GLINCKER/levelrail/internal/store"
)

// The console reuses the existing step-up scope shape: action manage on the
// pseudo app "cli-console", which the dashboard requests before opening it.
const (
	cliConsoleStepUpApp           = "cli-console"
	cliConsoleResource            = "system"
	auditActionCLIConsoleRun      = "cli_console.run"
	auditActionCLIConsoleRejected = "cli_console.rejected"
	errCodeCLIConsoleUnavailable  = "cli_console_unavailable"
	errCodeCrossOrigin            = "cross_origin"
	errCodeStepUpSessionOnly      = "step_up_session_only"
	cliConsoleReadLimit           = 64 << 10
)

type cliConsoleState struct {
	cfg   cliconsole.Config
	opens *apiRateLimiter
}

func newCLIConsoleState() *cliConsoleState {
	cfg := cliconsole.LoadConfig()
	return &cliConsoleState{cfg: cfg, opens: newAPIRateLimiter(cfg.OpensPerMinute)}
}

type cliConsoleCaller struct {
	userID       string
	sessionToken string
}

type cliConsoleCommandsResponse struct {
	Commands []cliconsole.Command `json:"commands"`
}

func cliConsoleEnabled() bool {
	return !cliconsole.KilledByEnv() && experimental.Enabled(experimental.CLIConsole)
}

// cliConsoleGate runs the gates after requireAbility(AbilityRoot): feature,
// origin, then a fresh step-up grant bound to this dashboard session. A
// WebSocket must carry an Origin; a plain same-origin GET may omit it.
func (rt *Router) cliConsoleGate(w http.ResponseWriter, r *http.Request, requireOrigin bool) (cliConsoleCaller, bool) {
	if !cliConsoleEnabled() {
		writeJSON(w, http.StatusConflict, experimentalError{
			Error: experimental.DisabledMessage(experimental.CLIConsole), Code: ExperimentalDisabledCode, Feature: string(experimental.CLIConsole),
		})
		return cliConsoleCaller{}, false
	}
	origin := r.Header.Get("Origin")
	if (requireOrigin && (origin == "" || !sameHost(origin, r.Host))) || crossOriginRequest(r) {
		writeCodedError(w, http.StatusForbidden, errCodeCrossOrigin, "the CLI console only opens from this dashboard's own origin")
		return cliConsoleCaller{}, false
	}
	if _, pinned := rt.currentPinnedSession(r); pinned {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpSessionOnly, "the CLI console needs a dashboard sign-in, not a session link or API token")
		return cliConsoleCaller{}, false
	}
	c, err := r.Cookie(sessionCookieName)
	userID, found := rt.currentSessionUserID(r)
	if err != nil || !found {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpSessionOnly, "the CLI console needs a dashboard sign-in, not a session link or API token")
		return cliConsoleCaller{}, false
	}
	if _, ok := rt.envProtect.grants.check(c.Value, userID, stepUpActionManage, cliConsoleStepUpApp, time.Now()); !ok {
		writeCodedError(w, http.StatusForbidden, errCodeStepUpRequired, "confirm your password to open the CLI console")
		return cliConsoleCaller{}, false
	}
	return cliConsoleCaller{userID: userID, sessionToken: c.Value}, true
}

// handleCLIConsoleCommands serves GET /api/v1/system/cli-console/commands.
func (rt *Router) handleCLIConsoleCommands(w http.ResponseWriter, r *http.Request) {
	if _, ok := rt.cliConsoleGate(w, r, false); !ok {
		return
	}
	writeJSON(w, http.StatusOK, cliConsoleCommandsResponse{Commands: cliconsole.Commands()})
}

// handleCLIConsoleSession serves GET /api/v1/system/cli-console/session. Every
// refusal happens before the upgrade, so it is a normal JSON error.
func (rt *Router) handleCLIConsoleSession(w http.ResponseWriter, r *http.Request) {
	caller, ok := rt.cliConsoleGate(w, r, true)
	if !ok {
		return
	}
	st := rt.cliConsole
	if allowed, retry := allowAll(limiterCheck{st.opens, "user:" + caller.userID}, limiterCheck{st.opens, "ip:" + clientIP(r)}); !allowed {
		writeRateLimited(w, retry)
		return
	}
	bin, err := cliconsole.LocateBinary(rt.brand.BinaryName)
	if err != nil {
		rt.logger.Warn("api: cli console: binary unavailable", slog.String("error", err.Error()))
		writeCodedError(w, http.StatusConflict, errCodeCLIConsoleUnavailable, err.Error())
		return
	}
	apiURL := rt.cliConsoleAPIURL()
	if apiURL == "" {
		writeCodedError(w, http.StatusConflict, errCodeCLIConsoleUnavailable, "the control plane's own HTTP address is not known")
		return
	}
	abilities, err := rt.callerAbilities(r)
	if err != nil {
		rt.internalError(w, "api: cli console: resolve abilities failed", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), st.cfg.MaxSession)
	defer cancel()
	expires := time.Now().Add(st.cfg.MaxSession)
	raw, tokenIDs, err := rt.mintCLIConsoleToken(ctx, caller.userID, consoleTokenAbilities(abilities), expires)
	if err != nil {
		rt.internalError(w, "api: cli console: mint session token failed", err, slog.String("user_id", caller.userID))
		return
	}
	defer rt.revokeCLIConsoleToken(tokenIDs)

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{CompressionMode: websocket.CompressionDisabled})
	if err != nil {
		rt.logger.Warn("api: cli console: websocket upgrade failed", slog.String("error", err.Error()))
		return
	}
	conn.SetReadLimit(cliConsoleReadLimit)
	defer func() { _ = conn.CloseNow() }()

	stopWatch := rt.watchAuthorization(r, AbilityRoot, func(*http.Request) string { return cliConsoleResource }, func() {
		rt.logger.Warn("api: cli console: caller lost access, ending session", slog.String("user_id", caller.userID))
		cancel()
	})
	defer stopWatch()

	sess := &cliConsoleSession{
		rt: rt, conn: conn, req: r, caller: caller, token: raw, cfg: st.cfg,
		runner: &cliconsole.Runner{
			Binary: bin, APIURL: apiURL, Experimental: experimentalEnvValue(),
			Timeout: st.cfg.CommandTimeout, MaxOutputBytes: st.cfg.MaxOutputBytes, MaxLineBytes: st.cfg.MaxLineBytes,
			RunAsUser: st.cfg.RunAsUser,
		},
	}
	sess.serve(ctx, cancel)
}

// consoleTokenAbilities never mints root or a sensitive ability: the console
// is for reads and routine writes, so a leaked session token cannot reveal secrets.
func consoleTokenAbilities(abilities []string) []string {
	if slices.Contains(abilities, AbilityRoot) {
		return []string{AbilityRead, AbilityWrite, AbilityDeploy}
	}
	out := []string{}
	for _, a := range abilities {
		switch a {
		case AbilityRead, AbilityWrite, AbilityDeploy:
			out = append(out, a)
		}
	}
	return out
}

func experimentalEnvValue() string {
	out := ""
	for _, f := range experimental.EnabledList() {
		if out != "" {
			out += ","
		}
		out += string(f)
	}
	return out
}

// cliConsoleAPIURL is the loopback address of this process's own HTTP API.
func (rt *Router) cliConsoleAPIURL() string {
	host, port, err := net.SplitHostPort(rt.dashboardListenAddr)
	if err != nil || port == "" {
		return ""
	}
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

type cliConsoleTokenIDs struct{ legacyID, engineID string }

// mintCLIConsoleToken issues the per-session token, mirroring the library
// mint path so the token is listed, expiring and revocable like any other.
func (rt *Router) mintCLIConsoleToken(ctx context.Context, userID string, abilities []string, expires time.Time) (string, cliConsoleTokenIDs, error) {
	return rt.mintOwnedToken(ctx, userID, "cli-console", abilities, expires)
}

// mintOwnedToken mints a user-owned token named "<label> <short id>".
func (rt *Router) mintOwnedToken(ctx context.Context, userID, label string, abilities []string, expires time.Time) (string, cliConsoleTokenIDs, error) {
	if err := validateTokenAbilities(abilities); err != nil {
		return "", cliConsoleTokenIDs{}, err
	}
	legacyID, err := randomTokenID()
	if err != nil {
		return "", cliConsoleTokenIDs{}, err
	}
	name := label + " " + legacyID[:min(8, len(legacyID))]
	raw, rec, err := rt.authLib.tokens.MintToken(ctx, authengine.MintInput{OwnerLegacyID: userID, Name: name, Abilities: abilities, ExpiresAt: &expires})
	if err != nil {
		return "", cliConsoleTokenIDs{}, err
	}
	ids := cliConsoleTokenIDs{legacyID: legacyID, engineID: rec.EngineID}
	legacy := store.APIToken{ID: legacyID, Name: rec.Name, TokenHash: rec.HashHex, Abilities: abilities, CreatedAt: rec.CreatedAt, ExpiresAt: &expires, OwnerUserID: userID}
	if err := rt.tokens.SaveAPIToken(ctx, legacy); err != nil {
		rt.revokeCLIConsoleToken(cliConsoleTokenIDs{engineID: rec.EngineID})
		return "", cliConsoleTokenIDs{}, err
	}
	if err := rt.authLib.tokens.LinkToken(ctx, legacyID, rec.EngineID); err != nil {
		rt.revokeCLIConsoleToken(ids)
		return "", cliConsoleTokenIDs{}, err
	}
	return raw, ids, nil
}

// revokeCLIConsoleToken runs on a fresh context: the session's own is already done.
func (rt *Router) revokeCLIConsoleToken(ids cliConsoleTokenIDs) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if ids.engineID != "" {
		if err := rt.authLib.tokens.RevokeToken(ctx, ids.engineID); err != nil && !errors.Is(err, authengine.ErrTokenUnknown) {
			rt.logger.Error("api: cli console: revoke session token failed", slog.String("token_id", ids.legacyID), slog.String("error", err.Error()))
		}
	}
	if ids.legacyID != "" {
		if err := rt.tokens.RevokeAPIToken(ctx, ids.legacyID); err != nil && !errors.Is(err, store.ErrAPITokenNotFound) {
			rt.logger.Error("api: cli console: revoke token mirror failed", slog.String("token_id", ids.legacyID), slog.String("error", err.Error()))
		}
	}
}

// auditCLIConsole records one typed line, run or refused, with secrets masked.
func (rt *Router) auditCLIConsole(r *http.Request, caller cliConsoleCaller, line string, status int, action string) {
	if rt.auditLog == nil {
		return
	}
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: cli console audit id failed", slog.String("error", err.Error()))
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	entry := store.AuditEntry{
		ID: id, ActorType: auditActorSession, ActorID: caller.userID,
		ActorName: rt.auditActorName(ctx, auditActorSession, caller.userID, ""),
		Ability:   AbilityRoot, Method: "RUN", Path: "cli-console: " + cliconsole.SanitizeLine(line, 0), StatusCode: status,
		RemoteAddr: clientIP(r), CreatedAt: store.FormatAuditTime(time.Now()),
		ClientKind: clientKindFromUserAgent(r.UserAgent()), Action: action,
	}
	if err := rt.auditLog.SaveAuditEntry(ctx, entry); err != nil {
		rt.logger.Warn("api: save cli console audit entry failed", slog.String("error", err.Error()))
	}
}
