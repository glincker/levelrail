package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/email"
	"github.com/GLINCKER/levelrail/internal/mail"
	"github.com/GLINCKER/levelrail/internal/mailrecords"
	"github.com/GLINCKER/levelrail/internal/secrets"
	"github.com/GLINCKER/levelrail/internal/store"
)

// System email: the instance's own mail server sends invites, resets and alerts as noreply@<domain>.
const (
	sysEmailSecretsNS   = "mail-system-email"
	sysEmailStateKey    = "state"
	sysEmailStepUpApp   = "mail-system-email"
	sysEmailLocalpart   = "noreply"
	envSysEmailPerHour  = "APP_MAIL_SYSTEM_EMAIL_ACTIONS_PER_HOUR"
	defSysEmailPerHour  = 10
	sysEmailProbeWindow = 15 * time.Second
	sysEmailDNSWarning  = "Mail to outside recipients will land in spam or be rejected until SPF, DKIM and DMARC pass for this domain and the server's reverse DNS (PTR) matches its mail host name."
)

// sysEmailRuntime serializes changes and rate limits callers; the zero value is ready.
type sysEmailRuntime struct {
	mu    sync.Mutex
	once  sync.Once
	limit *apiRateLimiter
}

func (s *sysEmailRuntime) limiter() *apiRateLimiter {
	s.once.Do(func() { s.limit = newWindowRateLimiter(envInt(envSysEmailPerHour, defSysEmailPerHour), time.Hour) })
	return s.limit
}

// sysEmailPrevious is the provider configuration enable replaced; its credentials stay sealed with it.
type sysEmailPrevious struct {
	Settings store.EmailSettings `json:"settings"`
	Sealed   map[string]string   `json:"sealed,omitempty"`
}

// sysEmailState is sealed in secret storage, never returned as is.
type sysEmailState struct {
	Enabled   bool              `json:"enabled"`
	Domain    string            `json:"domain"`
	Service   string            `json:"service"`
	Mailbox   string            `json:"mailbox"`
	Addr      string            `json:"addr"`
	TLSMode   string            `json:"tls_mode"`
	Previous  *sysEmailPrevious `json:"previous,omitempty"`
	LastTest  *sysEmailTest     `json:"last_test,omitempty"`
	UpdatedAt time.Time         `json:"updated_at"`
}

type sysEmailTest struct {
	To      string    `json:"to"`
	OK      bool      `json:"ok"`
	Kind    string    `json:"kind,omitempty"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

type sysEmailStep struct {
	ID      string `json:"id"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

type sysEmailDNS struct {
	SPF   string `json:"spf"`
	DKIM  string `json:"dkim"`
	DMARC string `json:"dmarc"`
	Pass  bool   `json:"pass"`
}

// sysEmailView is the wire shape: no credential or pin value, only facts about them.
type sysEmailView struct {
	Enabled bool `json:"enabled"`
	// Active is false when someone changed the email settings away from the mail server since enable.
	Active      bool          `json:"active"`
	Domain      string        `json:"domain,omitempty"`
	App         string        `json:"app,omitempty"`
	FromAddress string        `json:"from_address,omitempty"`
	Submission  string        `json:"submission,omitempty"`
	TLSMode     string        `json:"tls_mode,omitempty"`
	Pinned      bool          `json:"certificate_pinned"`
	HasPrevious bool          `json:"has_previous_provider"`
	LastTest    *sysEmailTest `json:"last_test,omitempty"`
	DNS         *sysEmailDNS  `json:"dns,omitempty"`
	Warning     string        `json:"warning,omitempty"`
	Demo        bool          `json:"demo,omitempty"`
	Domains     []string      `json:"domains"`
}

type sysEmailResult struct {
	State sysEmailView   `json:"state"`
	Steps []sysEmailStep `json:"steps"`
}

type sysEmailEnableRequest struct {
	Domain          string       `json:"domain"`
	To              string       `json:"to,omitempty"`
	Port            int          `json:"port,omitempty"`
	ReplaceExisting bool         `json:"replace_existing"`
	StepUp          *stepUpProof `json:"step_up,omitempty"`
}

type sysEmailActionRequest struct {
	To     string       `json:"to,omitempty"`
	StepUp *stepUpProof `json:"step_up,omitempty"`
}

func (rt *Router) loadSysEmailState(ctx context.Context) (sysEmailState, error) {
	var st sysEmailState
	if d := rt.mail.demo; d != nil {
		d.mu.Lock()
		defer d.mu.Unlock()
		if d.systemEmail != nil {
			st = *d.systemEmail
		}
		return st, nil
	}
	raw, err := rt.emailSecrets.Resolve(ctx, sysEmailSecretsNS, sysEmailStateKey)
	if errors.Is(err, secrets.ErrValueNotFound) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	return st, json.Unmarshal([]byte(raw), &st)
}

func (rt *Router) saveSysEmailState(ctx context.Context, st sysEmailState) error {
	st.UpdatedAt = time.Now().UTC()
	if d := rt.mail.demo; d != nil {
		d.mu.Lock()
		d.systemEmail = &st
		d.mu.Unlock()
		return nil
	}
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return rt.emailSecrets.SetValue(ctx, sysEmailSecretsNS, sysEmailStateKey, string(b))
}

// sysEmailReady checks the stores and the per-caller budget, writing the error itself.
func (rt *Router) sysEmailReady(w http.ResponseWriter, r *http.Request, limited bool) bool {
	if rt.mail.demo == nil && rt.emailSecrets == nil {
		writeError(w, http.StatusNotImplemented, "email settings are not configured on this control plane (no master key set)")
		return false
	}
	if _, ok := rt.mailboxStore(w); !ok {
		return false
	}
	if limited {
		_, pID, _, _ := rt.callerPrincipal(r)
		if ok, retry := rt.mail.sysEmail.limiter().allow("sysemail:" + pID); !ok {
			w.Header().Set("Retry-After", strconv.Itoa(int(retry.Seconds())+1))
			writeError(w, http.StatusTooManyRequests, "too many system email actions; try again later")
			return false
		}
	}
	return true
}

func (rt *Router) sysEmailView(ctx context.Context, st sysEmailState) sysEmailView {
	v := sysEmailView{Enabled: st.Enabled, Domain: st.Domain, App: st.Service, Submission: st.Addr, TLSMode: st.TLSMode,
		HasPrevious: st.Previous != nil, LastTest: st.LastTest, Demo: rt.mail.demo != nil, Domains: []string{}}
	if st.Mailbox != "" {
		v.FromAddress = st.Mailbox
	}
	if st.Enabled && rt.mail.demo != nil {
		v.Active = true
	} else if st.Enabled {
		if s, err := rt.emailSettings.GetEmailSettings(ctx); err == nil {
			v.Active = s.Backend == store.EmailBackendSMTP && strings.EqualFold(s.SMTPUsername, st.Mailbox) && net.JoinHostPort(s.SMTPHost, strconv.Itoa(s.SMTPPort)) == st.Addr
		}
		if pin, err := rt.emailSecrets.Resolve(ctx, store.EmailSettingsSecretsKey(), store.EmailSecretSMTPTLSPin); err == nil {
			v.Pinned = email.PinFor(pin, st.Addr) != ""
		}
	}
	if list, err := rt.mail.boxes.ListMailDomains(ctx); err == nil {
		for _, d := range list {
			v.Domains = append(v.Domains, d.Domain)
		}
	}
	if st.Domain != "" {
		v.DNS = rt.sysEmailDNS(ctx, st.Domain)
		if v.DNS == nil || !v.DNS.Pass {
			v.Warning = sysEmailDNSWarning
		}
	}
	return v
}

// sysEmailDNS reuses the mail DNS check; it reads public DNS only.
func (rt *Router) sysEmailDNS(ctx context.Context, domain string) *sysEmailDNS {
	exp := mailrecords.Expect{}
	domain = rt.mailDemoCheckArgs(domain, &exp)
	ctx, cancel := context.WithTimeout(ctx, 5*domainCheckLookupTimeout)
	defer cancel()
	rep, err := mailrecords.Check(ctx, rt.mailResolver(), domain, exp)
	if err != nil {
		return nil
	}
	out := &sysEmailDNS{SPF: mailrecords.StatusNotTested, DKIM: mailrecords.StatusNotTested, DMARC: mailrecords.StatusNotTested}
	for _, it := range rep.Items {
		switch it.ID {
		case "spf":
			out.SPF = it.Status
		case "dkim":
			out.DKIM = it.Status
		case "dmarc":
			out.DMARC = it.Status
		}
	}
	out.Pass = out.SPF == mailrecords.StatusPass && out.DKIM == mailrecords.StatusPass && out.DMARC == mailrecords.StatusPass
	return out
}

// handleGetSystemEmail handles GET /api/v1/mail/system-email.
func (rt *Router) handleGetSystemEmail(w http.ResponseWriter, r *http.Request) {
	if !rt.sysEmailReady(w, r, false) {
		return
	}
	st, err := rt.loadSysEmailState(r.Context())
	if err != nil {
		rt.internalError(w, "api: system email: load state", err)
		return
	}
	writeJSON(w, http.StatusOK, rt.sysEmailView(r.Context(), st))
}

// ensureNoreplyMailbox creates noreply@domain through the mailbox code path, or resets and resumes the existing
// one. The password goes straight to the caller's sealing step and is never returned or logged.
func (rt *Router) ensureNoreplyMailbox(r *http.Request, e mail.Engine, d store.MailDomain, actor string) (string, string, error) {
	db := rt.mail.boxes
	address := mail.AccountNameFor(sysEmailLocalpart + "@" + d.Domain)
	m, err := db.GetMailbox(r.Context(), address)
	switch {
	case err == nil:
		pw, err := mail.SetAccountPassword(r.Context(), e, address)
		if err != nil {
			return address, "", err
		}
		if m.Suspended {
			if err := mail.ResumeAccount(r.Context(), e, address); err != nil {
				return address, "", err
			}
			if err := db.SetMailboxSuspended(r.Context(), address, false); err != nil {
				return address, "", err
			}
		}
		return address, pw, nil
	case !errors.Is(err, store.ErrMailboxNotFound):
		return address, "", err
	}
	pw, err := mail.CreateAccount(r.Context(), e, address, mail.DefaultSendCapPerHour())
	if err != nil {
		return address, "", err
	}
	box := store.Mailbox{Address: address, Domain: d.Domain, Localpart: sysEmailLocalpart, Service: d.Service, TeamID: d.TeamID, CreatedBy: actor}
	if err := db.CreateMailbox(r.Context(), box); err != nil {
		_ = e.RemoveAccount(r.Context(), address)
		return address, "", err
	}
	return address, pw, nil
}

// sysEmailSubmission resolves the engine's submission address on the private app network.
func (rt *Router) sysEmailSubmission(ctx context.Context, service string, port int) (string, error) {
	if rt.mail.demo != nil {
		return net.JoinHostPort("mail.demo.invalid", strconv.Itoa(port)), nil
	}
	svc, err := rt.apps.GetDesiredService(ctx, service)
	if err != nil {
		return "", err
	}
	c, err := rt.mailEngineContainer(ctx, *svc)
	if err != nil {
		return "", err
	}
	ip := containerIP(c)
	if !c.Running || ip == "" {
		return "", errMailEngineMissing
	}
	return net.JoinHostPort(ip, strconv.Itoa(port)), nil
}

// sealedSlots reads the primary credentials so disable can put them back.
func (rt *Router) sealedSlots(ctx context.Context) (map[string]string, error) {
	slots, err := rt.readSlots(ctx, store.EmailSettingsSecretsKey())
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	for k, s := range slots {
		if s.present {
			out[k] = s.value
		}
	}
	pin, err := rt.emailSecrets.Resolve(ctx, store.EmailSettingsSecretsKey(), store.EmailSecretSMTPTLSPin)
	if err == nil {
		out[store.EmailSecretSMTPTLSPin] = pin
	} else if !errors.Is(err, secrets.ErrValueNotFound) {
		return nil, err
	}
	return out, nil
}

func (rt *Router) restoreSealed(ctx context.Context, sealed map[string]string) error {
	slots := map[string]secretSlot{}
	for _, f := range emailSecretFields {
		v, ok := sealed[f.envKey]
		slots[f.envKey] = secretSlot{value: v, present: ok}
	}
	if err := rt.writeSlots(ctx, store.EmailSettingsSecretsKey(), slots); err != nil {
		return err
	}
	if pin, ok := sealed[store.EmailSecretSMTPTLSPin]; ok {
		return rt.emailSecrets.SetValue(ctx, store.EmailSettingsSecretsKey(), store.EmailSecretSMTPTLSPin, pin)
	}
	return rt.emailSecrets.DeleteValue(ctx, store.EmailSettingsSecretsKey(), store.EmailSecretSMTPTLSPin)
}

func (rt *Router) applyEmailSettings(ctx context.Context, s store.EmailSettings) error {
	if err := rt.syncPrimaryProvider(ctx, s); err != nil {
		return err
	}
	return rt.emailSettings.UpdateEmailSettings(ctx, s)
}

// sysEmailTestSend runs the provider test send; in demo mode nothing leaves the process.
func (rt *Router) sysEmailTestSend(r *http.Request, to string) sysEmailTest {
	t := sysEmailTest{To: to, At: time.Now().UTC()}
	if rt.mail.demo != nil {
		t.OK, t.Message = true, "DEMO DATA: the test send was simulated; no mail left this machine."
		return t
	}
	if rt.emailSender == nil {
		t.Kind, t.Message = "not_configured", "email is not configured on this control plane"
		return t
	}
	ctx, cancel := context.WithTimeout(email.WithoutRetry(email.WithKind(r.Context(), "test")), emailTestTimeout)
	defer cancel()
	subject := "Test email from " + rt.brand.Name
	body := "This is a test email from your " + rt.brand.Name + " control plane, sent through your own mail server."
	if err := rt.emailSender.Send(ctx, to, subject, body); err != nil {
		t.Kind, t.Message = string(email.Classify(err)), "the mail server did not accept the test message"
		rt.logger.Warn("api: system email: test send failed", slog.String("kind", t.Kind))
		return t
	}
	t.OK, t.Message = true, "the mail server accepted the message into its queue"
	return t
}

func (rt *Router) sysEmailRecipient(r *http.Request, to string) (string, error) {
	to = strings.TrimSpace(to)
	if to == "" {
		to = rt.signedInUserEmail(r)
	}
	if to == "" {
		return "", errors.New("to is required: no signed-in user address to default to")
	}
	return to, email.ValidateAddress(to)
}

func testSteps(t sysEmailTest) []sysEmailStep {
	return []sysEmailStep{{ID: "test_send_queued", OK: t.OK, Message: t.Message}, {ID: "delivered_to_local_queue", OK: t.OK, Message: t.Message}}
}

// handleEnableSystemEmail handles POST /api/v1/mail/system-email/enable.
func (rt *Router) handleEnableSystemEmail(w http.ResponseWriter, r *http.Request) {
	var req sysEmailEnableRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.sysEmailReady(w, r, true) {
		return
	}
	if req.Port == 0 {
		req.Port = 587
	}
	if req.Port != 587 && req.Port != 465 {
		writeError(w, http.StatusBadRequest, "port must be 587 (STARTTLS) or 465 (implicit TLS)")
		return
	}
	to, err := rt.sysEmailRecipient(r, req.To)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	ctx := r.Context()
	domain := strings.ToLower(strings.TrimSpace(req.Domain))
	d, ok := rt.mailDomainFor(w, r, domain, AbilityWriteSensitive)
	if !ok {
		return
	}
	actor, ok := rt.mailAuthorize(w, r, d.TeamID, AbilityWriteSensitive, mailResMailbox+sysEmailLocalpart+"@"+domain, mailResDomain+domain, mailResService+d.Service)
	if !ok {
		return
	}
	if _, ok := rt.stepUpOwner(w, r, req.StepUp, sysEmailStepUpApp, "change the system email provider", "changing the system email provider"); !ok {
		return
	}
	rt.mail.sysEmail.mu.Lock()
	defer rt.mail.sysEmail.mu.Unlock()
	st, err := rt.loadSysEmailState(ctx)
	if err != nil {
		rt.internalError(w, "api: system email: load state", err)
		return
	}
	var current store.EmailSettings
	if rt.mail.demo == nil {
		if current, err = rt.emailSettings.GetEmailSettings(ctx); err != nil {
			rt.internalError(w, "api: system email: read settings", err)
			return
		}
		if current.Backend != "" && !st.Enabled && !req.ReplaceExisting {
			writeCodedError(w, http.StatusConflict, "provider_exists", "an email provider is already configured; pass replace_existing=true to replace it (disable puts it back)")
			return
		}
	}
	addr, err := rt.sysEmailSubmission(ctx, d.Service, req.Port)
	if err != nil {
		rt.logger.Warn("api: system email: engine lookup failed", slog.String("app", d.Service), slog.String("error", err.Error()))
		writeError(w, http.StatusServiceUnavailable, "the mail server is not running on the app network; check levelrail-cli mail status")
		return
	}
	e, _, ok := rt.mailEngineFor(w, r, d.Service)
	if !ok {
		return
	}
	pin := ""
	if rt.mail.demo == nil {
		pctx, cancel := context.WithTimeout(ctx, sysEmailProbeWindow)
		pin, err = email.FetchSMTPCertSHA256(pctx, addr)
		cancel()
		if err != nil {
			rt.logger.Warn("api: system email: submission probe failed", slog.String("error", err.Error()))
			writeError(w, http.StatusBadGateway, "the mail server submission listener did not answer with TLS")
			return
		}
	}
	address, pw, err := rt.ensureNoreplyMailbox(r, e, d, actor)
	if err != nil {
		rt.mailEngineError(w, "prepare the noreply mailbox", err)
		return
	}
	steps := []sysEmailStep{{ID: "mailbox", OK: true, Message: address + " is ready (send cap " + strconv.Itoa(mail.DefaultSendCapPerHour()) + " per hour)"}}
	tlsMode := email.TLSStartTLS
	if req.Port == 465 {
		tlsMode = email.TLSImplicit
	}
	if !st.Enabled {
		st.Previous = nil
		if rt.mail.demo == nil && current.Backend != "" {
			sealed, err := rt.sealedSlots(ctx)
			if err != nil {
				rt.internalError(w, "api: system email: snapshot provider", err)
				return
			}
			st.Previous = &sysEmailPrevious{Settings: current, Sealed: sealed}
		}
	}
	st.Enabled, st.Domain, st.Service, st.Mailbox, st.Addr, st.TLSMode = true, domain, d.Service, address, addr, tlsMode
	// Saved before the provider changes so disable can always revert.
	if err := rt.saveSysEmailState(ctx, st); err != nil {
		rt.internalError(w, "api: system email: save state", err)
		return
	}
	if rt.mail.demo == nil {
		host, _, _ := net.SplitHostPort(addr)
		next := current
		next.Backend, next.SMTPHost, next.SMTPPort, next.SMTPUsername, next.SMTPFrom = store.EmailBackendSMTP, host, req.Port, address, address
		next.FromAddress, next.EnvelopeFrom, next.TLSMode, next.TLSSkipVerify = address, "", tlsMode, false
		key := store.EmailSettingsSecretsKey()
		if err := rt.emailSecrets.SetValue(ctx, key, store.EmailSecretSMTPPassword, pw); err != nil {
			rt.internalError(w, "api: system email: seal credential", err)
			return
		}
		if err := rt.emailSecrets.SetValue(ctx, key, store.EmailSecretSMTPTLSPin, email.FormatPin(addr, pin)); err != nil {
			rt.internalError(w, "api: system email: seal pin", err)
			return
		}
		if err := rt.applyEmailSettings(ctx, next); err != nil {
			rt.writeProviderError(w, "api: system email: save provider", err)
			return
		}
		rt.auditEmailSecretChange(r, "", []string{store.EmailSecretSMTPPassword, store.EmailSecretSMTPTLSPin})
	}
	steps = append(steps, sysEmailStep{ID: "provider_saved", OK: true, Message: "system email now submits to " + addr + " as " + address})
	t := rt.sysEmailTestSend(r, to)
	st.LastTest = &t
	if err := rt.saveSysEmailState(ctx, st); err != nil {
		rt.internalError(w, "api: system email: save state", err)
		return
	}
	rt.logger.Info("api: system email enabled", slog.String("domain", domain), slog.String("by", actor), slog.Bool("test_ok", t.OK))
	writeJSON(w, http.StatusOK, sysEmailResult{State: rt.sysEmailView(ctx, st), Steps: append(steps, testSteps(t)...)})
}

// handleDisableSystemEmail handles POST /api/v1/mail/system-email/disable: puts the previous provider back and
// suspends (never deletes) the noreply mailbox.
func (rt *Router) handleDisableSystemEmail(w http.ResponseWriter, r *http.Request) {
	var req sysEmailActionRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.sysEmailReady(w, r, false) {
		return
	}
	st, d, ok := rt.sysEmailEnabled(w, r, req.StepUp)
	if !ok {
		return
	}
	defer rt.mail.sysEmail.mu.Unlock()
	ctx := r.Context()
	var steps []sysEmailStep
	if rt.mail.demo == nil {
		prev := store.EmailSettings{}
		sealed := map[string]string{}
		if st.Previous != nil {
			prev, sealed = st.Previous.Settings, st.Previous.Sealed
		}
		if err := rt.restoreSealed(ctx, sealed); err != nil {
			rt.internalError(w, "api: system email: restore credentials", err)
			return
		}
		if prev.Backend == "" {
			if err := rt.ensureNoBackupProvider(ctx); err != nil {
				rt.writeProviderError(w, "api: system email: provider check", err)
				return
			}
		}
		if err := rt.applyEmailSettings(ctx, prev); err != nil {
			rt.writeProviderError(w, "api: system email: restore provider", err)
			return
		}
		rt.auditEmailSecretChange(r, "", []string{store.EmailSecretSMTPPassword, store.EmailSecretSMTPTLSPin})
	}
	steps = append(steps, sysEmailStep{ID: "provider_restored", OK: true, Message: "the previous email provider is back"})
	suspended := sysEmailStep{ID: "mailbox_suspended", OK: true, Message: st.Mailbox + " is suspended; its mail is kept"}
	if err := rt.sysEmailSuspend(r, d.Service, st.Mailbox); err != nil {
		rt.logger.Warn("api: system email: suspend noreply failed", slog.String("error", err.Error()))
		suspended.OK, suspended.Message = false, "the provider was restored but the mailbox could not be suspended; suspend it from the Mailboxes panel"
	}
	st.Enabled, st.Previous = false, nil
	if err := rt.saveSysEmailState(ctx, st); err != nil {
		rt.internalError(w, "api: system email: save state", err)
		return
	}
	writeJSON(w, http.StatusOK, sysEmailResult{State: rt.sysEmailView(ctx, st), Steps: append(steps, suspended)})
}

func (rt *Router) sysEmailSuspend(r *http.Request, service, address string) error {
	build := rt.mail.engine
	if build == nil {
		build = rt.defaultMailEngine
	}
	if rt.mail.demo != nil {
		build = rt.mail.demo.engineFactory
	}
	e, err := build(r.Context(), service)
	if err != nil {
		return err
	}
	if err := mail.SuspendAccount(r.Context(), e, address); err != nil {
		return err
	}
	return rt.mail.boxes.SetMailboxSuspended(r.Context(), address, true)
}

// sysEmailEnabled authorizes and step-ups an action on an enabled setup. On success it returns holding the lock.
func (rt *Router) sysEmailEnabled(w http.ResponseWriter, r *http.Request, proof *stepUpProof) (sysEmailState, store.MailDomain, bool) {
	rt.mail.sysEmail.mu.Lock()
	st, err := rt.loadSysEmailState(r.Context())
	fail := func() (sysEmailState, store.MailDomain, bool) {
		rt.mail.sysEmail.mu.Unlock()
		return st, store.MailDomain{}, false
	}
	if err != nil {
		rt.internalError(w, "api: system email: load state", err)
		return fail()
	}
	if !st.Enabled {
		writeError(w, http.StatusConflict, "system email through the mail server is not enabled")
		return fail()
	}
	d, ok := rt.mailDomainFor(w, r, st.Domain, AbilityWriteSensitive)
	if !ok {
		return fail()
	}
	if _, ok := rt.mailAuthorize(w, r, d.TeamID, AbilityWriteSensitive, mailResMailbox+st.Mailbox, mailResDomain+d.Domain, mailResService+d.Service); !ok {
		return fail()
	}
	if _, ok := rt.stepUpOwner(w, r, proof, sysEmailStepUpApp, "change the system email provider", "changing the system email provider"); !ok {
		return fail()
	}
	return st, d, true
}

// handleRotateSystemEmail handles POST /api/v1/mail/system-email/rotate: a new generated password, sealed, never shown.
func (rt *Router) handleRotateSystemEmail(w http.ResponseWriter, r *http.Request) {
	var req sysEmailActionRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.sysEmailReady(w, r, true) {
		return
	}
	st, d, ok := rt.sysEmailEnabled(w, r, req.StepUp)
	if !ok {
		return
	}
	defer rt.mail.sysEmail.mu.Unlock()
	e, _, ok := rt.mailEngineFor(w, r, d.Service)
	if !ok {
		return
	}
	pw, err := mail.SetAccountPassword(r.Context(), e, st.Mailbox)
	if err != nil {
		rt.mailEngineError(w, "set the noreply password", err)
		return
	}
	if rt.mail.demo == nil {
		if err := rt.emailSecrets.SetValue(r.Context(), store.EmailSettingsSecretsKey(), store.EmailSecretSMTPPassword, pw); err != nil {
			rt.internalError(w, "api: system email: seal credential", err)
			return
		}
		rt.auditEmailSecretChange(r, "", []string{store.EmailSecretSMTPPassword})
	}
	writeJSON(w, http.StatusOK, sysEmailResult{State: rt.sysEmailView(r.Context(), st), Steps: []sysEmailStep{{ID: "rotated", OK: true, Message: "a new password is sealed into the email provider; it is never shown"}}})
}

// handleTestSystemEmail handles POST /api/v1/mail/system-email/test.
func (rt *Router) handleTestSystemEmail(w http.ResponseWriter, r *http.Request) {
	var req sysEmailActionRequest
	if err := decodeJSONBodyLimit(r, &req, 1<<12); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if !rt.sysEmailReady(w, r, true) {
		return
	}
	to, err := rt.sysEmailRecipient(r, req.To)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	rt.mail.sysEmail.mu.Lock()
	defer rt.mail.sysEmail.mu.Unlock()
	st, err := rt.loadSysEmailState(r.Context())
	if err != nil {
		rt.internalError(w, "api: system email: load state", err)
		return
	}
	if !st.Enabled {
		writeError(w, http.StatusConflict, "system email through the mail server is not enabled")
		return
	}
	if _, ok := rt.mailDomainFor(w, r, st.Domain, AbilityWriteSensitive); !ok {
		return
	}
	t := rt.sysEmailTestSend(r, to)
	st.LastTest = &t
	if err := rt.saveSysEmailState(r.Context(), st); err != nil {
		rt.internalError(w, "api: system email: save state", err)
		return
	}
	writeJSON(w, http.StatusOK, sysEmailResult{State: rt.sysEmailView(r.Context(), st), Steps: testSteps(t)})
}
