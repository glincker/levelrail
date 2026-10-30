package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/GLINCKER/levelrail/internal/mail"
	"github.com/GLINCKER/levelrail/internal/mail/mox"
	"github.com/GLINCKER/levelrail/internal/store"
)

// mailAdminSecretKey is the per-service secret the mail-mox template stores
// the generated admin password under.
const mailAdminSecretKey = "MAIL_ADMIN_PASSWORD"

// mailShownOnce is printed next to every generated mailbox password.
const mailShownOnce = "This password is shown once. Levelrail does not store it; copy it now and hand it to the mailbox user."

// IAM resource prefixes from ADR 059 section 2.6.
const (
	mailResService = "mail:service:"
	mailResDomain  = "mail:domain:"
	mailResMailbox = "mail:mailbox:"
	// mailPasswordStepUpApp scopes the step-up grant for resetting a password.
	mailPasswordStepUpApp = "mail-password"
	mailDeleteStepUpApp   = "mail-delete"
	mailQueueStepUpApp    = "mail-queue-release"
	mailDomainStepUpApp   = "mail-domain"
)

// MailboxStore is the ownership metadata behind mailboxes; *store.DB satisfies it.
type MailboxStore interface {
	ClaimMailDomain(ctx context.Context, d store.MailDomain) error
	GetMailDomain(ctx context.Context, domain string) (store.MailDomain, error)
	ListMailDomains(ctx context.Context) ([]store.MailDomain, error)
	CreateMailbox(ctx context.Context, m store.Mailbox) error
	GetMailbox(ctx context.Context, address string) (store.Mailbox, error)
	ListMailboxes(ctx context.Context, domain string) ([]store.Mailbox, error)
	ListMailboxesLinkedTo(ctx context.Context, teamID, userID string) ([]store.Mailbox, error)
	SetMailboxSuspended(ctx context.Context, address string, suspended bool) error
	DeleteMailbox(ctx context.Context, address string) error
}

// mailEngineFactory builds an engine for one installed mail app; tests swap it.
type mailEngineFactory func(ctx context.Context, service string) (mail.Engine, error)

type mailSecretCreds struct {
	src     ComposeSecretStore
	service string
}

func (c mailSecretCreds) AdminPassword(ctx context.Context) (string, error) {
	return c.src.Resolve(ctx, c.service, mailAdminSecretKey)
}

// defaultMailEngine reaches the admin API on the container's app network
// address and reads the admin password from secret storage per call.
func (rt *Router) defaultMailEngine(ctx context.Context, service string) (mail.Engine, error) {
	if rt.composeSecrets == nil {
		return nil, errors.New("mail: secret storage is not configured")
	}
	ip := ""
	if rt.containers != nil {
		list, err := rt.containers.ListByPrefix(ctx, service)
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			if c.Labels["levelrail.mail"] != "engine" || !c.Running {
				continue
			}
			for _, n := range c.Networks {
				if n.IPAddress != "" {
					ip = n.IPAddress
					break
				}
			}
		}
	}
	if ip == "" {
		return nil, errors.New("mail: the mail container is not running")
	}
	return mox.New(mox.Config{BaseURL: mox.AdminBaseURL(ip), Credentials: mailSecretCreds{src: rt.composeSecrets, service: service}})
}

// mailEngineFor resolves the named mail app and its engine, writing the error itself.
func (rt *Router) mailEngineFor(w http.ResponseWriter, r *http.Request, app string) (mail.Engine, store.DesiredService, bool) {
	svc, found, err := rt.pickMailApp(r, app)
	if err != nil {
		rt.internalError(w, "api: mail: list services failed", err)
		return nil, svc, false
	}
	if !found {
		writeError(w, http.StatusNotFound, "no mail app is installed, install the mail-mox template first")
		return nil, svc, false
	}
	build := rt.mail.engine
	if build == nil {
		build = rt.defaultMailEngine
	}
	e, err := build(r.Context(), svc.Name)
	if err != nil {
		rt.logger.Warn("api: mail: engine unavailable", slog.String("app", svc.Name), slog.String("error", err.Error()))
		writeError(w, http.StatusServiceUnavailable, "the mail server admin API is not reachable; check levelrail-cli mail status")
		return nil, svc, false
	}
	return e, svc, true
}

func (rt *Router) mailboxStore(w http.ResponseWriter) (MailboxStore, bool) {
	if rt.mail.boxes == nil {
		writeError(w, http.StatusNotImplemented, "mailbox management needs the control plane database")
		return nil, false
	}
	return rt.mail.boxes, true
}

// mailDecide is the mail authorization rule: a resource outside the caller's
// teams, or behind an explicit IAM Deny, answers 404 so its existence does
// not leak; a team role too low for the ability answers 403.
func mailDecide(scope *teamScope, abilities []string, policies []store.Policy, teamID, ability string, resources ...string) int {
	role := store.TeamRoleOwner
	if scope != nil {
		role = scope.roleOfTeam(teamID)
	}
	if role == "" {
		return http.StatusNotFound
	}
	if len(resources) > 0 {
		if matched, allowed := evaluatePolicies(policies, ability, resources[0], resources[1:]...); matched && !allowed {
			return http.StatusNotFound
		}
	}
	if !hasAbility(capAbilities(abilities, teamID, role), ability) {
		return http.StatusForbidden
	}
	return 0
}

// mailAuthorize applies mailDecide to the current caller and writes the denial.
func (rt *Router) mailAuthorize(w http.ResponseWriter, r *http.Request, teamID, ability string, resources ...string) (string, bool) {
	pType, pID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: mail: resolve caller", err)
		return "", false
	}
	var policies []store.Policy
	if rt.policies != nil {
		if policies, err = rt.policies.ListPoliciesForPrincipal(r.Context(), pType, pID); err != nil {
			rt.internalError(w, "api: mail: list policies", err)
			return "", false
		}
	}
	scope, _ := callerTeamScope(r.Context())
	switch mailDecide(scope, abilities, policies, teamID, ability, resources...) {
	case http.StatusNotFound:
		writeError(w, http.StatusNotFound, "mail resource not found")
		return "", false
	case http.StatusForbidden:
		writeError(w, http.StatusForbidden, msgTeamRoleTooLow)
		return "", false
	}
	return pID, true
}

func mailboxResources(m store.Mailbox) []string {
	return []string{mailResMailbox + m.Address, mailResDomain + m.Domain, mailResService + m.Service}
}

// mailEngineError maps the typed engine errors onto HTTP answers.
func (rt *Router) mailEngineError(w http.ResponseWriter, what string, err error) {
	switch {
	case errors.Is(err, mail.ErrHasMail):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, mail.ErrInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, mail.ErrExists):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, mail.ErrNotFound):
		writeError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, mail.ErrUnsupported):
		writeError(w, http.StatusNotImplemented, err.Error())
	case errors.Is(err, mail.ErrUnauthorized):
		writeError(w, http.StatusBadGateway, "the mail server rejected the stored admin password")
	default:
		rt.logger.Warn("api: mail engine call failed", slog.String("op", what), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "the mail server could not "+what)
	}
}

type mailDomainResource struct {
	store.MailDomain
}

type mailDomainClaimRequest struct {
	Domain string       `json:"domain"`
	TeamID string       `json:"team_id"`
	App    string       `json:"app,omitempty"`
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

// handleListMailDomains handles GET /api/v1/mail/domains: the owned domains the caller's teams can see.
func (rt *Router) handleListMailDomains(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.mailboxStore(w)
	if !ok {
		return
	}
	list, err := db.ListMailDomains(r.Context())
	if err != nil {
		rt.internalError(w, "api: list mail domains", err)
		return
	}
	scope, _ := callerTeamScope(r.Context())
	out := []mailDomainResource{}
	for _, d := range list {
		if scope.allowsTeam(d.TeamID) {
			out = append(out, mailDomainResource{d})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// handleClaimMailDomain handles POST /api/v1/mail/domains: one team takes
// ownership of a domain the mail server already serves.
func (rt *Router) handleClaimMailDomain(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.mailboxStore(w)
	if !ok {
		return
	}
	var req mailDomainClaimRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Domain = strings.ToLower(strings.TrimSpace(req.Domain))
	if req.Domain == "" || req.TeamID == "" {
		writeError(w, http.StatusBadRequest, "domain and team_id are required")
		return
	}
	actor, ok := rt.mailAuthorize(w, r, req.TeamID, AbilityRoot, mailResDomain+req.Domain)
	if !ok {
		return
	}
	e, svc, ok := rt.mailEngineFor(w, r, req.App)
	if !ok {
		return
	}
	domains, err := e.Domains(r.Context())
	if err != nil {
		rt.mailEngineError(w, "list domains", err)
		return
	}
	served := false
	for _, d := range domains {
		served = served || strings.EqualFold(d.Name, req.Domain)
	}
	if !served {
		writeError(w, http.StatusBadRequest, "the mail server does not serve "+req.Domain+"; add it to the mail server first")
		return
	}
	if _, ok := rt.stepUpOwner(w, r, req.StepUp, mailDomainStepUpApp+":"+req.Domain, "claim a mail domain", "claiming a mail domain"); !ok {
		return
	}
	d := store.MailDomain{Domain: req.Domain, Service: svc.Name, TeamID: req.TeamID, CreatedBy: actor}
	if err := db.ClaimMailDomain(r.Context(), d); errors.Is(err, store.ErrMailDomainTaken) {
		writeError(w, http.StatusConflict, "this domain already belongs to a team")
		return
	} else if err != nil {
		rt.internalError(w, "api: claim mail domain", err)
		return
	}
	writeJSON(w, http.StatusCreated, mailDomainResource{d})
}

type mailboxResource struct {
	store.Mailbox
}

type mailboxCreateRequest struct {
	Address      string `json:"address"`
	LinkedUserID string `json:"linked_user_id,omitempty"`
}

// mailboxSecretResponse carries a generated password exactly once.
type mailboxSecretResponse struct {
	Mailbox  mailboxResource `json:"mailbox"`
	Password string          `json:"password"`
	Notice   string          `json:"notice"`
	// SendCapPerHour is advisory data (ADR 059 decision 6); nothing enforces it yet.
	SendCapPerHour int `json:"send_cap_per_hour"`
}

// handleListMailboxes handles GET /api/v1/mail/mailboxes?domain=.
func (rt *Router) handleListMailboxes(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.mailboxStore(w)
	if !ok {
		return
	}
	list, err := db.ListMailboxes(r.Context(), strings.TrimSpace(r.URL.Query().Get("domain")))
	if err != nil {
		rt.internalError(w, "api: list mailboxes", err)
		return
	}
	pType, pID, abilities, err := rt.callerPrincipal(r)
	if err != nil {
		rt.internalError(w, "api: mail: resolve caller", err)
		return
	}
	var policies []store.Policy
	if rt.policies != nil {
		if policies, err = rt.policies.ListPoliciesForPrincipal(r.Context(), pType, pID); err != nil {
			rt.internalError(w, "api: mail: list policies", err)
			return
		}
	}
	scope, _ := callerTeamScope(r.Context())
	out := []mailboxResource{}
	for _, m := range list {
		if mailDecide(scope, abilities, policies, m.TeamID, AbilityRead, mailboxResources(m)...) == 0 {
			out = append(out, mailboxResource{m})
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// loadMailbox reads a mailbox from the path and authorizes the ability on it.
func (rt *Router) loadMailbox(w http.ResponseWriter, r *http.Request, ability string) (MailboxStore, store.Mailbox, bool) {
	db, ok := rt.mailboxStore(w)
	if !ok {
		return nil, store.Mailbox{}, false
	}
	m, err := db.GetMailbox(r.Context(), r.PathValue("address"))
	if errors.Is(err, store.ErrMailboxNotFound) {
		writeError(w, http.StatusNotFound, "mail resource not found")
		return nil, m, false
	}
	if err != nil {
		rt.internalError(w, "api: get mailbox", err)
		return nil, m, false
	}
	_, ok = rt.mailAuthorize(w, r, m.TeamID, ability, mailboxResources(m)...)
	return db, m, ok
}

// handleCreateMailbox handles POST /api/v1/mail/mailboxes. The domain's team
// owns the mailbox; the generated password is returned once.
func (rt *Router) handleCreateMailbox(w http.ResponseWriter, r *http.Request) {
	db, ok := rt.mailboxStore(w)
	if !ok {
		return
	}
	var req mailboxCreateRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	local, domain, err := mail.SplitAddress(req.Address)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	d, err := db.GetMailDomain(r.Context(), domain)
	if errors.Is(err, store.ErrMailDomainNotFound) {
		writeError(w, http.StatusNotFound, "mail domain not found; a team admin claims it first with mail domains claim")
		return
	}
	if err != nil {
		rt.internalError(w, "api: get mail domain", err)
		return
	}
	address := mail.AccountNameFor(req.Address)
	actor, ok := rt.mailAuthorize(w, r, d.TeamID, AbilityWriteSensitive, mailResMailbox+address, mailResDomain+domain, mailResService+d.Service)
	if !ok {
		return
	}
	if _, err := db.GetMailbox(r.Context(), address); err == nil {
		writeError(w, http.StatusConflict, "mailbox already exists")
		return
	}
	e, _, ok := rt.mailEngineFor(w, r, d.Service)
	if !ok {
		return
	}
	capPerHour := mail.DefaultSendCapPerHour()
	pw, err := mail.CreateAccount(r.Context(), e, address, capPerHour)
	if err != nil {
		rt.mailEngineError(w, "create the mailbox", err)
		return
	}
	m := store.Mailbox{Address: address, Domain: domain, Localpart: local, Service: d.Service, TeamID: d.TeamID, LinkedUserID: req.LinkedUserID, CreatedBy: actor}
	if err := db.CreateMailbox(r.Context(), m); err != nil {
		// Keep engine and store in step: undo the account the store could not record.
		_ = e.RemoveAccount(r.Context(), address)
		rt.internalError(w, "api: record mailbox", err)
		return
	}
	saved, _ := db.GetMailbox(r.Context(), address)
	writeJSON(w, http.StatusCreated, mailboxSecretResponse{Mailbox: mailboxResource{saved}, Password: pw, Notice: mailShownOnce, SendCapPerHour: capPerHour})
}

type mailStepUpRequest struct {
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

// handleMailboxPassword handles POST /api/v1/mail/mailboxes/{address}/password:
// a fresh generated password, shown once, after step-up.
func (rt *Router) handleMailboxPassword(w http.ResponseWriter, r *http.Request) {
	var req mailStepUpRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, m, ok := rt.loadMailbox(w, r, AbilityWriteSensitive)
	if !ok {
		return
	}
	if _, ok := rt.stepUpOwner(w, r, req.StepUp, mailPasswordStepUpApp+":"+m.Address, "reset a mailbox password", "resetting a mailbox password"); !ok {
		return
	}
	e, _, ok := rt.mailEngineFor(w, r, m.Service)
	if !ok {
		return
	}
	pw, err := mail.SetAccountPassword(r.Context(), e, m.Address)
	if err != nil {
		rt.mailEngineError(w, "set the password", err)
		return
	}
	writeJSON(w, http.StatusOK, mailboxSecretResponse{Mailbox: mailboxResource{m}, Password: pw, Notice: mailShownOnce, SendCapPerHour: mail.DefaultSendCapPerHour()})
}

// handleSuspendMailbox handles POST /api/v1/mail/mailboxes/{address}/suspend and /resume.
func (rt *Router) handleSuspendMailbox(suspend bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		db, m, ok := rt.loadMailbox(w, r, AbilityWriteSensitive)
		if !ok {
			return
		}
		e, _, ok := rt.mailEngineFor(w, r, m.Service)
		if !ok {
			return
		}
		op, err := mail.SuspendAccount, error(nil)
		if !suspend {
			op = mail.ResumeAccount
		}
		if err = op(r.Context(), e, m.Address); err != nil {
			rt.mailEngineError(w, "change the mailbox login state", err)
			return
		}
		if err := db.SetMailboxSuspended(r.Context(), m.Address, suspend); err != nil {
			rt.internalError(w, "api: save mailbox state", err)
			return
		}
		m.Suspended = suspend
		writeJSON(w, http.StatusOK, mailboxResource{m})
	}
}

type mailboxDeleteRequest struct {
	Confirm    string       `json:"confirm"`
	DeleteMail bool         `json:"delete_mail"`
	StepUp     *stepUpProof `json:"step_up,omitempty"`
}

// handleDeleteMailbox handles DELETE /api/v1/mail/mailboxes/{address}: root,
// a typed confirm, step-up, and a refusal while mail exists unless delete_mail.
func (rt *Router) handleDeleteMailbox(w http.ResponseWriter, r *http.Request) {
	var req mailboxDeleteRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	db, m, ok := rt.loadMailbox(w, r, AbilityRoot)
	if !ok {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(req.Confirm), m.Address) {
		writeError(w, http.StatusBadRequest, "confirm must repeat the mailbox address")
		return
	}
	if _, ok := rt.stepUpOwner(w, r, req.StepUp, mailDeleteStepUpApp+":"+m.Address, "delete a mailbox", "deleting a mailbox"); !ok {
		return
	}
	e, _, ok := rt.mailEngineFor(w, r, m.Service)
	if !ok {
		return
	}
	if err := mail.DeleteAccount(r.Context(), e, m.Address, m.Address, req.DeleteMail); err != nil && !errors.Is(err, mail.ErrNotFound) {
		rt.mailEngineError(w, "delete the mailbox", err)
		return
	}
	if err := db.DeleteMailbox(r.Context(), m.Address); err != nil {
		rt.internalError(w, "api: delete mailbox record", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type mailAliasRequest struct {
	Address string   `json:"address"`
	Targets []string `json:"targets"`
}

// mailDomainFor loads an owned domain and authorizes the ability on it.
func (rt *Router) mailDomainFor(w http.ResponseWriter, r *http.Request, domain, ability string, extra ...string) (store.MailDomain, bool) {
	db, ok := rt.mailboxStore(w)
	if !ok {
		return store.MailDomain{}, false
	}
	d, err := db.GetMailDomain(r.Context(), domain)
	if errors.Is(err, store.ErrMailDomainNotFound) {
		writeError(w, http.StatusNotFound, "mail resource not found")
		return d, false
	}
	if err != nil {
		rt.internalError(w, "api: get mail domain", err)
		return d, false
	}
	res := append(extra, mailResDomain+d.Domain, mailResService+d.Service)
	_, ok = rt.mailAuthorize(w, r, d.TeamID, ability, res...)
	return d, ok
}

// handleListMailAliases handles GET /api/v1/mail/aliases?domain=.
func (rt *Router) handleListMailAliases(w http.ResponseWriter, r *http.Request) {
	d, ok := rt.mailDomainFor(w, r, strings.ToLower(strings.TrimSpace(r.URL.Query().Get("domain"))), AbilityRead)
	if !ok {
		return
	}
	e, _, ok := rt.mailEngineFor(w, r, d.Service)
	if !ok {
		return
	}
	list, err := mail.ListAliases(r.Context(), e, d.Domain)
	if err != nil {
		rt.mailEngineError(w, "list aliases", err)
		return
	}
	writeJSON(w, http.StatusOK, list)
}

// handleAddMailAlias handles POST /api/v1/mail/aliases. Targets must be
// mailboxes on domains the same team owns, so an alias cannot pull mail
// into another team's mailbox.
func (rt *Router) handleAddMailAlias(w http.ResponseWriter, r *http.Request) {
	var req mailAliasRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<14); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	_, domain, err := mail.SplitAddress(req.Address)
	if err != nil || len(req.Targets) == 0 || len(req.Targets) > 50 {
		writeError(w, http.StatusBadRequest, "address must be an address and targets must list 1 to 50 mailboxes")
		return
	}
	addr := mail.AccountNameFor(req.Address)
	d, ok := rt.mailDomainFor(w, r, domain, AbilityWriteSensitive, mailResMailbox+addr)
	if !ok {
		return
	}
	for i, t := range req.Targets {
		req.Targets[i] = mail.AccountNameFor(t)
		tm, err := rt.mail.boxes.GetMailbox(r.Context(), req.Targets[i])
		if err != nil || tm.TeamID != d.TeamID {
			writeError(w, http.StatusBadRequest, "every target must be a mailbox your team owns")
			return
		}
	}
	e, _, ok := rt.mailEngineFor(w, r, d.Service)
	if !ok {
		return
	}
	if err := e.AddAlias(r.Context(), mail.Alias{Address: addr, Targets: req.Targets}); err != nil {
		rt.mailEngineError(w, "add the alias", err)
		return
	}
	writeJSON(w, http.StatusCreated, mail.Alias{Address: addr, Targets: req.Targets})
}

// handleRemoveMailAlias handles DELETE /api/v1/mail/aliases/{address}.
func (rt *Router) handleRemoveMailAlias(w http.ResponseWriter, r *http.Request) {
	_, domain, err := mail.SplitAddress(r.PathValue("address"))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	addr := mail.AccountNameFor(r.PathValue("address"))
	d, ok := rt.mailDomainFor(w, r, domain, AbilityWriteSensitive, mailResMailbox+addr)
	if !ok {
		return
	}
	e, _, ok := rt.mailEngineFor(w, r, d.Service)
	if !ok {
		return
	}
	if err := e.RemoveAlias(r.Context(), addr); err != nil {
		rt.mailEngineError(w, "remove the alias", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type mailQueueResponse struct {
	App     string            `json:"app"`
	Queued  int               `json:"queued"`
	Entries []mail.QueueEntry `json:"entries"`
}

type mailQueueHoldRequest struct {
	App    string       `json:"app,omitempty"`
	IDs    []int64      `json:"ids,omitempty"`
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

type mailQueueHoldResponse struct {
	App     string `json:"app"`
	Scope   string `json:"scope"`
	Changed int    `json:"changed"`
}

// handleMailQueue handles GET /api/v1/mail/queue?app=&max=: delivery metadata
// only, never subjects or bodies. The route itself needs read:sensitive.
func (rt *Router) handleMailQueue(w http.ResponseWriter, r *http.Request) {
	e, svc, ok := rt.mailEngineFor(w, r, r.URL.Query().Get("app"))
	if !ok {
		return
	}
	limit, _ := strconv.Atoi(r.URL.Query().Get("max"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	list, err := mail.ListQueue(r.Context(), e, limit)
	if err != nil {
		rt.mailEngineError(w, "list the queue", err)
		return
	}
	writeJSON(w, http.StatusOK, mailQueueResponse{App: svc.Name, Queued: len(list), Entries: list})
}

// handleMailQueueHold handles POST /api/v1/mail/queue/hold and /release. No
// ids means the whole queue; release needs step-up (ADR 059 section 2.6).
func (rt *Router) handleMailQueueHold(hold bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req mailQueueHoldRequest
		if err := decodeJSONBodyLimit(r, &req, 1<<16); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
		e, svc, ok := rt.mailEngineFor(w, r, req.App)
		if !ok {
			return
		}
		if !hold {
			if _, ok := rt.stepUpOwner(w, r, req.StepUp, mailQueueStepUpApp+":"+svc.Name, "release held mail", "releasing held mail"); !ok {
				return
			}
		}
		out := mailQueueHoldResponse{App: svc.Name, Scope: "all"}
		var err error
		switch {
		case len(req.IDs) > 0:
			out.Scope = "ids"
			out.Changed, err = e.HoldMessages(r.Context(), req.IDs, hold)
		case hold:
			err = e.HoldQueue(r.Context())
		default:
			err = e.ReleaseQueue(r.Context())
		}
		if err != nil {
			rt.mailEngineError(w, "change the queue hold", err)
			return
		}
		writeJSON(w, http.StatusOK, out)
	}
}

// offboardMailboxes suspends the mailboxes linked to a user who left a team
// (ADR 059 decision 2). Best effort: a failure is logged and the record is
// left active so the dashboard still shows it as needing action.
func (rt *Router) offboardMailboxes(ctx context.Context, teamID, userID string) int {
	if rt.mail.boxes == nil || userID == "" {
		return 0
	}
	list, err := rt.mail.boxes.ListMailboxesLinkedTo(ctx, teamID, userID)
	if err != nil || len(list) == 0 {
		return 0
	}
	build := rt.mail.engine
	if build == nil {
		build = rt.defaultMailEngine
	}
	n := 0
	for _, m := range list {
		if m.Suspended {
			continue
		}
		e, eerr := build(ctx, m.Service)
		if eerr == nil {
			eerr = mail.SuspendAccount(ctx, e, m.Address)
		}
		if eerr == nil {
			eerr = rt.mail.boxes.SetMailboxSuspended(ctx, m.Address, true)
		}
		if eerr != nil {
			rt.logger.Warn("api: mail offboarding: suspend failed", slog.String("mailbox", m.Address), slog.String("error", eerr.Error()))
			continue
		}
		n++
	}
	return n
}
