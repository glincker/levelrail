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

	"github.com/GLINCKER/levelrail/internal/docker"
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
	sysEmailPendingKey  = "pending_password" //nolint:gosec // an envKey name, not a credential value
	envSysEmailDNSTTL   = "APP_MAIL_SYSTEM_EMAIL_DNS_CACHE_SECONDS"
	defSysEmailDNSTTL   = 60
	sysEmailPinWarning  = "pin mismatch: the mail server moved or changed since system email was enabled; re-run levelrail-cli mail system-email enable to re-pin it"
	sysEmailDNSWarning  = "Mail to outside recipients will land in spam or be rejected until SPF, DKIM and DMARC pass for this domain and the server's reverse DNS (PTR) matches its mail host name."
)

// sysEmailRuntime serializes changes, rate limits callers and caches the DNS check; the zero value is ready.
type sysEmailRuntime struct {
	mu    sync.Mutex
	once  sync.Once
	limit *apiRateLimiter
	dnsMu sync.Mutex
	dns   map[string]sysEmailDNSEntry
}

type sysEmailDNSEntry struct {
	v  *sysEmailDNS
	at time.Time
}

func (s *sysEmailRuntime) limiter() *apiRateLimiter {
	s.once.Do(func() { s.limit = newWindowRateLimiter(envInt(envSysEmailPerHour, defSysEmailPerHour), time.Hour) })
	return s.limit
}

var (
	errSysEmailNoIdentity = errors.New("mail: the instance identity is unavailable, so the mail engine cannot be verified")
	errSysEmailNetwork    = errors.New("mail: the mail engine has no address on its own app network")
)

// sysEmailPrevious is the provider configuration enable replaced; its credentials stay sealed with it.
type sysEmailPrevious struct {
	Settings store.EmailSettings `json:"settings"`
	Sealed   map[string]string   `json:"sealed,omitempty"`
}

// sysEmailState is sealed in secret storage, never returned as is.
type sysEmailState struct {
	Enabled  bool              `json:"enabled"`
	Domain   string            `json:"domain"`
	Service  string            `json:"service"`
	Mailbox  string            `json:"mailbox"`
	Addr     string            `json:"addr"`
	TLSMode  string            `json:"tls_mode"`
	Previous *sysEmailPrevious `json:"previous,omitempty"`
	LastTest *sysEmailTest     `json:"last_test,omitempty"`
	// CertSHA256 is the engine certificate fingerprint enable pinned for this service.
	CertSHA256 string    `json:"cert_sha256,omitempty"`
	Error      string    `json:"error,omitempty"`
	UpdatedAt  time.Time `json:"updated_at"`
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
	// PinMismatch means the engine moved since enable; re-running enable re-pins it.
	PinMismatch bool   `json:"pin_mismatch"`
	Error       string `json:"error,omitempty"`
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
		HasPrevious: st.Previous != nil, LastTest: st.LastTest, Demo: rt.mail.demo != nil, Domains: []string{}, Error: st.Error}
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
		if cur, err := rt.sysEmailSubmission(ctx, st.Service, sysEmailPort(st.Addr)); err != nil || cur != st.Addr {
			v.PinMismatch = true
		}
	}
	if list, err := rt.mail.boxes.ListMailDomains(ctx); err == nil {
		for _, d := range list {
			v.Domains = append(v.Domains, d.Domain)
		}
	}
	if st.Domain != "" {
		v.DNS = rt.sysEmailDNSCached(ctx, st.Domain)
		if v.DNS == nil || !v.DNS.Pass {
			v.Warning = sysEmailDNSWarning
		}
	}
	if v.PinMismatch {
		v.Warning = sysEmailPinWarning
	}
	return v
}

// sysEmailDNSCached keeps each DNS answer for a short env-set TTL so polling the status cannot fan out lookups.
func (rt *Router) sysEmailDNSCached(ctx context.Context, domain string) *sysEmailDNS {
	s := &rt.mail.sysEmail
	ttl := time.Duration(envInt(envSysEmailDNSTTL, defSysEmailDNSTTL)) * time.Second
	s.dnsMu.Lock()
	if e, ok := s.dns[domain]; ok && time.Since(e.at) < ttl {
		s.dnsMu.Unlock()
		return e.v
	}
	s.dnsMu.Unlock()
	v := rt.sysEmailDNS(ctx, domain)
	s.dnsMu.Lock()
	if s.dns == nil {
		s.dns = map[string]sysEmailDNSEntry{}
	}
	s.dns[domain] = sysEmailDNSEntry{v: v, at: time.Now()}
	s.dnsMu.Unlock()
	return v
}

func sysEmailPort(addr string) int {
	_, p, _ := net.SplitHostPort(addr)
	n, _ := strconv.Atoi(p)
	return n
}

// sysEmailActive reports whether the instance provider is still the one enable configured.
func sysEmailActive(s store.EmailSettings, st sysEmailState) bool {
	return st.Enabled && st.Mailbox != "" && s.Backend == store.EmailBackendSMTP && strings.EqualFold(s.SMTPUsername, st.Mailbox) &&
		net.JoinHostPort(s.SMTPHost, strconv.Itoa(s.SMTPPort)) == st.Addr
}

// sysEmailCurrent reads the active provider and whether it is still the system email one.
func (rt *Router) sysEmailCurrent(ctx context.Context, st sysEmailState) (store.EmailSettings, bool, error) {
	if rt.mail.demo != nil {
		return store.EmailSettings{}, st.Enabled, nil
	}
	cur, err := rt.emailSettings.GetEmailSettings(ctx)
	if err != nil {
		return cur, false, err
	}
	return cur, sysEmailActive(cur, st), nil
}

func (rt *Router) sysEmailOwnPassword(ctx context.Context) (string, error) {
	pw, err := rt.emailSecrets.Resolve(ctx, sysEmailSecretsNS, store.EmailSecretSMTPPassword)
	if errors.Is(err, secrets.ErrValueNotFound) {
		return "", nil
	}
	return pw, err
}

func (rt *Router) setSysEmailOwnPassword(ctx context.Context, pw string) error {
	if pw == "" {
		err := rt.emailSecrets.DeleteValue(ctx, sysEmailSecretsNS, store.EmailSecretSMTPPassword)
		if errors.Is(err, secrets.ErrValueNotFound) {
			return nil
		}
		return err
	}
	return rt.emailSecrets.SetValue(ctx, sysEmailSecretsNS, store.EmailSecretSMTPPassword, pw)
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

// sysNoreply is the prepared noreply account; undo puts the engine and the record back on a failed enable.
type sysNoreply struct {
	address  string
	password string
	undo     func(context.Context)
}

// ensureNoreplyMailbox creates noreply@domain as a system mailbox, or marks, resets and resumes the existing one.
// The password goes straight to the caller's sealing step and is never returned or logged. oldPW is the sealed
// system email password, used to put the engine back when enable fails.
func (rt *Router) ensureNoreplyMailbox(r *http.Request, e mail.Engine, d store.MailDomain, actor, oldPW string) (sysNoreply, error) {
	ctx, db := r.Context(), rt.mail.boxes
	out := sysNoreply{address: mail.AccountNameFor(sysEmailLocalpart + "@" + d.Domain), undo: func(context.Context) {}}
	m, err := db.GetMailbox(ctx, out.address)
	switch {
	case err == nil:
		if out.password, err = mail.GeneratePassword(); err != nil {
			return out, err
		}
		if err := e.SetPassword(ctx, out.address, out.password); err != nil {
			return out, err
		}
		was := m
		out.undo = func(ctx context.Context) {
			if oldPW != "" {
				_ = e.SetPassword(ctx, was.Address, oldPW)
			}
			if was.Suspended {
				_ = mail.SuspendAccount(ctx, e, was.Address)
				_ = db.SetMailboxSuspended(ctx, was.Address, true)
			}
			if !was.System {
				_ = db.SetMailboxSystem(ctx, was.Address, false)
			}
		}
		if !m.System {
			if err := db.SetMailboxSystem(ctx, out.address, true); err != nil {
				out.undo(ctx)
				return out, err
			}
		}
		if m.Suspended {
			if err := mail.ResumeAccount(ctx, e, out.address); err != nil {
				out.undo(ctx)
				return out, err
			}
			if err := db.SetMailboxSuspended(ctx, out.address, false); err != nil {
				out.undo(ctx)
				return out, err
			}
		}
		return out, nil
	case !errors.Is(err, store.ErrMailboxNotFound):
		return out, err
	}
	if out.password, err = mail.CreateAccount(ctx, e, out.address, mail.DefaultSendCapPerHour()); err != nil {
		return out, err
	}
	box := store.Mailbox{Address: out.address, Domain: d.Domain, Localpart: sysEmailLocalpart, Service: d.Service, TeamID: d.TeamID, CreatedBy: actor, System: true}
	if err := db.CreateMailbox(ctx, box); err != nil {
		_ = e.RemoveAccount(ctx, out.address)
		return out, err
	}
	out.undo = func(ctx context.Context) {
		_ = e.RemoveAccount(ctx, box.Address)
		_ = db.DeleteMailbox(ctx, box.Address)
	}
	return out, nil
}

// sysEmailSubmission resolves the engine's submission address on the private app network.
func (rt *Router) sysEmailSubmission(ctx context.Context, service string, port int) (string, error) {
	if rt.mail.demo != nil {
		return net.JoinHostPort("mail.demo.invalid", strconv.Itoa(port)), nil
	}
	// Fail closed: without the instance identity the label check in mailEngineContainer is skipped.
	if rt.instance == nil {
		return "", errSysEmailNoIdentity
	}
	svc, err := rt.apps.GetDesiredService(ctx, service)
	if err != nil {
		return "", err
	}
	c, err := rt.mailEngineContainer(ctx, *svc)
	if err != nil {
		return "", err
	}
	if !c.Running {
		return "", errMailEngineMissing
	}
	ip := sysEmailAppNetworkIP(c, svc.AppID)
	if ip == "" {
		return "", errSysEmailNetwork
	}
	return net.JoinHostPort(ip, strconv.Itoa(port)), nil
}

// sysEmailAppNetworkIP returns the engine's address on its own app network only, never another attached network.
func sysEmailAppNetworkIP(c docker.ContainerState, appID string) string {
	if appID == "" {
		return ""
	}
	for _, n := range c.Networks {
		if n.IPAddress != "" && strings.HasSuffix(n.Name, "-app-"+appID) {
			return n.IPAddress
		}
	}
	return ""
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
		if errors.Is(err, email.ErrPinMismatch) {
			t.Kind, t.Message = "pin_mismatch", sysEmailPinWarning
		}
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

// sysEmailEngineUnavailable explains a failed strict engine lookup without the engine's address.
func (rt *Router) sysEmailEngineUnavailable(w http.ResponseWriter, app string, err error) {
	rt.logger.Warn("api: system email: engine lookup failed", slog.String("app", app), slog.String("error", err.Error()))
	msg := "the mail server is not running on its app network; check levelrail-cli mail status"
	if errors.Is(err, errSysEmailNoIdentity) {
		msg = "the instance identity is unavailable, so the mail server cannot be verified; system email is refused until it is back"
	}
	writeError(w, http.StatusServiceUnavailable, msg)
}

// handleEnableSystemEmail handles POST /api/v1/mail/system-email/enable. Nothing is persisted until the new provider
// is applied, and any failure puts the previous credentials, provider and mailbox back.
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
	demo := rt.mail.demo != nil
	current, active, err := rt.sysEmailCurrent(ctx, st)
	if err != nil {
		rt.internalError(w, "api: system email: read settings", err)
		return
	}
	// Consult the provider that is active now, not the stored flag: someone may have switched since enable.
	if !demo && !active && current.Backend != "" && !req.ReplaceExisting {
		writeCodedError(w, http.StatusConflict, "provider_exists", "an email provider is already configured; pass replace_existing=true to replace it (disable puts it back)")
		return
	}
	addr, err := rt.sysEmailSubmission(ctx, d.Service, req.Port)
	if err != nil {
		rt.sysEmailEngineUnavailable(w, d.Service, err)
		return
	}
	e, _, ok := rt.mailEngineFor(w, r, d.Service)
	if !ok {
		return
	}
	pin, snapshot, ownOld := "", map[string]string{}, ""
	if !demo {
		pctx, cancel := context.WithTimeout(ctx, sysEmailProbeWindow)
		pin, err = email.FetchSMTPCertSHA256(pctx, addr)
		cancel()
		if err != nil {
			rt.logger.Warn("api: system email: submission probe failed", slog.String("error", err.Error()))
			writeError(w, http.StatusBadGateway, "the mail server submission listener did not answer with TLS")
			return
		}
		if snapshot, err = rt.sealedSlots(ctx); err != nil {
			rt.internalError(w, "api: system email: snapshot provider", err)
			return
		}
		if ownOld, err = rt.sysEmailOwnPassword(ctx); err != nil {
			rt.internalError(w, "api: system email: read credential", err)
			return
		}
	}
	prev := st.Previous
	if !active {
		prev = nil
		if !demo && current.Backend != "" {
			prev = &sysEmailPrevious{Settings: current, Sealed: snapshot}
		}
	}
	nb, err := rt.ensureNoreplyMailbox(r, e, d, actor, ownOld)
	if err != nil {
		rt.mailEngineError(w, "prepare the noreply mailbox", err)
		return
	}
	rollback := func(applied bool) {
		rctx := context.WithoutCancel(ctx)
		if !demo {
			if applied {
				if err := rt.applyEmailSettings(rctx, current); err != nil {
					rt.logger.Error("api: system email: roll back provider", slog.String("error", err.Error()))
				}
			}
			if err := rt.restoreSealed(rctx, snapshot); err != nil {
				rt.logger.Error("api: system email: roll back credentials", slog.String("error", err.Error()))
			}
			if err := rt.setSysEmailOwnPassword(rctx, ownOld); err != nil {
				rt.logger.Error("api: system email: roll back own credential", slog.String("error", err.Error()))
			}
		}
		nb.undo(rctx)
	}
	steps := []sysEmailStep{{ID: "mailbox", OK: true, Message: nb.address + " is ready (send cap " + strconv.Itoa(mail.DefaultSendCapPerHour()) + " per hour)"}}
	if st.CertSHA256 != "" && st.Service == d.Service && pin != "" && pin != st.CertSHA256 {
		steps = append(steps, sysEmailStep{ID: "certificate_changed", OK: true, Message: "the mail server certificate changed since the last enable; the new one is pinned"})
	}
	tlsMode := email.TLSStartTLS
	if req.Port == 465 {
		tlsMode = email.TLSImplicit
	}
	if !demo {
		host, _, _ := net.SplitHostPort(addr)
		next := current
		next.Backend, next.SMTPHost, next.SMTPPort, next.SMTPUsername, next.SMTPFrom = store.EmailBackendSMTP, host, req.Port, nb.address, nb.address
		next.FromAddress, next.EnvelopeFrom, next.TLSMode, next.TLSSkipVerify = nb.address, "", tlsMode, false
		key := store.EmailSettingsSecretsKey()
		err := rt.setSysEmailOwnPassword(ctx, nb.password)
		if err == nil {
			err = rt.emailSecrets.SetValue(ctx, key, store.EmailSecretSMTPPassword, nb.password)
		}
		if err == nil {
			err = rt.emailSecrets.SetValue(ctx, key, store.EmailSecretSMTPTLSPin, email.FormatPin(addr, pin))
		}
		if err != nil {
			rollback(false)
			rt.internalError(w, "api: system email: seal credential", err)
			return
		}
		if err := rt.applyEmailSettings(ctx, next); err != nil {
			rollback(false)
			rt.writeProviderError(w, "api: system email: save provider", err)
			return
		}
	}
	next := st
	next.Enabled, next.Domain, next.Service, next.Mailbox, next.Addr, next.TLSMode = true, domain, d.Service, nb.address, addr, tlsMode
	next.Previous, next.CertSHA256, next.Error = prev, pin, ""
	if err := rt.saveSysEmailState(ctx, next); err != nil {
		rollback(true)
		rt.internalError(w, "api: system email: save state", err)
		return
	}
	if !demo {
		rt.auditEmailSecretChange(r, "", []string{store.EmailSecretSMTPPassword, store.EmailSecretSMTPTLSPin})
	}
	steps = append(steps, sysEmailStep{ID: "provider_saved", OK: true, Message: "system email now submits to " + addr + " as " + nb.address})
	t := rt.sysEmailTestSend(r, to)
	next.LastTest = &t
	if err := rt.saveSysEmailState(ctx, next); err != nil {
		rt.internalError(w, "api: system email: save state", err)
		return
	}
	rt.logger.Info("api: system email enabled", slog.String("domain", domain), slog.String("by", actor), slog.Bool("test_ok", t.OK))
	writeJSON(w, http.StatusOK, sysEmailResult{State: rt.sysEmailView(ctx, next), Steps: append(steps, testSteps(t)...)})
}

// handleDisableSystemEmail handles POST /api/v1/mail/system-email/disable: puts the previous provider back only while
// the system email one is still active, then suspends (never deletes) the noreply mailbox.
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
	rt.sysEmailDisableLocked(w, r, st, d)
}

// sysEmailDisableLocked runs disable for an authorized caller holding the lock.
func (rt *Router) sysEmailDisableLocked(w http.ResponseWriter, r *http.Request, st sysEmailState, d store.MailDomain) {
	ctx := r.Context()
	_, active, err := rt.sysEmailCurrent(ctx, st)
	if err != nil {
		rt.internalError(w, "api: system email: read settings", err)
		return
	}
	var steps []sysEmailStep
	switch {
	case active && rt.mail.demo == nil:
		if !rt.sysEmailRestorePrevious(w, r, st) {
			return
		}
		steps = append(steps, sysEmailStep{ID: "provider_restored", OK: true, Message: "the previous email provider is back"})
	case active:
		steps = append(steps, sysEmailStep{ID: "provider_restored", OK: true, Message: "the previous email provider is back"})
	default:
		steps = append(steps, sysEmailStep{ID: "provider_kept", OK: true, Message: "the email provider was changed after enable, so it is left as it is"})
	}
	st.Previous = nil
	suspended := sysEmailStep{ID: "mailbox_suspended", OK: true, Message: st.Mailbox + " is suspended; its mail is kept"}
	if err := rt.sysEmailSuspend(r, d.Service, st.Mailbox); err != nil {
		// Stay enabled with the error so status shows it and a re-run of disable finishes the job.
		rt.logger.Warn("api: system email: suspend noreply failed", slog.String("error", err.Error()))
		suspended.OK, suspended.Message = false, "the noreply mailbox could not be suspended; system email stays enabled until disable succeeds, re-run it"
		st.Error = suspended.Message
	} else {
		st.Enabled, st.Error = false, ""
		if rt.mail.demo == nil {
			if err := rt.setSysEmailOwnPassword(ctx, ""); err != nil {
				rt.logger.Warn("api: system email: drop own credential", slog.String("error", err.Error()))
			}
		}
	}
	if err := rt.saveSysEmailState(ctx, st); err != nil {
		rt.internalError(w, "api: system email: save state", err)
		return
	}
	writeJSON(w, http.StatusOK, sysEmailResult{State: rt.sysEmailView(ctx, st), Steps: append(steps, suspended)})
}

// sysEmailRestorePrevious puts back the provider enable replaced, undoing the credential swap if the apply fails.
func (rt *Router) sysEmailRestorePrevious(w http.ResponseWriter, r *http.Request, st sysEmailState) bool {
	ctx := r.Context()
	prev, sealed := store.EmailSettings{}, map[string]string{}
	if st.Previous != nil {
		prev, sealed = st.Previous.Settings, st.Previous.Sealed
	}
	snapshot, err := rt.sealedSlots(ctx)
	if err != nil {
		rt.internalError(w, "api: system email: snapshot provider", err)
		return false
	}
	if prev.Backend == "" {
		if err := rt.ensureNoBackupProvider(ctx); err != nil {
			rt.writeProviderError(w, "api: system email: provider check", err)
			return false
		}
	}
	if err := rt.restoreSealed(ctx, sealed); err != nil {
		_ = rt.restoreSealed(context.WithoutCancel(ctx), snapshot)
		rt.internalError(w, "api: system email: restore credentials", err)
		return false
	}
	if err := rt.applyEmailSettings(ctx, prev); err != nil {
		_ = rt.restoreSealed(context.WithoutCancel(ctx), snapshot)
		rt.writeProviderError(w, "api: system email: restore provider", err)
		return false
	}
	rt.auditEmailSecretChange(r, "", []string{store.EmailSecretSMTPPassword, store.EmailSecretSMTPTLSPin})
	return true
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

// handleRotateSystemEmail handles POST /api/v1/mail/system-email/rotate: a new generated password, sealed as pending
// before the engine changes, then committed to the system email slot (and the live provider only while it is active).
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
	ctx := r.Context()
	e, _, ok := rt.mailEngineFor(w, r, d.Service)
	if !ok {
		return
	}
	pw, err := mail.GeneratePassword()
	if err != nil {
		rt.internalError(w, "api: system email: generate credential", err)
		return
	}
	if rt.mail.demo != nil {
		if err := e.SetPassword(ctx, st.Mailbox, pw); err != nil {
			rt.mailEngineError(w, "set the noreply password", err)
			return
		}
	} else if !rt.sysEmailRotateSealed(w, r, e, st, pw) {
		return
	}
	writeJSON(w, http.StatusOK, sysEmailResult{State: rt.sysEmailView(ctx, st), Steps: []sysEmailStep{{ID: "rotated", OK: true, Message: "a new password is sealed for system email; it is never shown"}}})
}

func (rt *Router) sysEmailRotateSealed(w http.ResponseWriter, r *http.Request, e mail.Engine, st sysEmailState, pw string) bool {
	ctx := r.Context()
	_, active, err := rt.sysEmailCurrent(ctx, st)
	if err != nil {
		rt.internalError(w, "api: system email: read settings", err)
		return false
	}
	old, err := rt.sysEmailOwnPassword(ctx)
	if err != nil {
		rt.internalError(w, "api: system email: read credential", err)
		return false
	}
	dropPending := func() {
		_ = rt.emailSecrets.DeleteValue(context.WithoutCancel(ctx), sysEmailSecretsNS, sysEmailPendingKey)
	}
	if err := rt.emailSecrets.SetValue(ctx, sysEmailSecretsNS, sysEmailPendingKey, pw); err != nil {
		rt.internalError(w, "api: system email: seal pending credential", err)
		return false
	}
	if err := e.SetPassword(ctx, st.Mailbox, pw); err != nil {
		dropPending()
		rt.mailEngineError(w, "set the noreply password", err)
		return false
	}
	err = rt.setSysEmailOwnPassword(ctx, pw)
	if err == nil && active {
		err = rt.emailSecrets.SetValue(ctx, store.EmailSettingsSecretsKey(), store.EmailSecretSMTPPassword, pw)
	}
	if err != nil {
		rctx := context.WithoutCancel(ctx)
		if old != "" {
			_ = e.SetPassword(rctx, st.Mailbox, old)
			if active {
				_ = rt.emailSecrets.SetValue(rctx, store.EmailSettingsSecretsKey(), store.EmailSecretSMTPPassword, old)
			}
		}
		_ = rt.setSysEmailOwnPassword(rctx, old)
		dropPending()
		rt.internalError(w, "api: system email: seal credential", err)
		return false
	}
	dropPending()
	if active {
		rt.auditEmailSecretChange(r, "", []string{store.EmailSecretSMTPPassword})
	}
	return true
}

// handleTestSystemEmail handles POST /api/v1/mail/system-email/test, after step-up like the other actions.
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
	st, _, ok := rt.sysEmailEnabled(w, r, req.StepUp)
	if !ok {
		return
	}
	defer rt.mail.sysEmail.mu.Unlock()
	if rt.mail.demo == nil {
		if cur, err := rt.sysEmailSubmission(r.Context(), st.Service, sysEmailPort(st.Addr)); err != nil || cur != st.Addr {
			writeCodedError(w, http.StatusConflict, "pin_mismatch", sysEmailPinWarning)
			return
		}
	}
	t := rt.sysEmailTestSend(r, to)
	st.LastTest = &t
	if err := rt.saveSysEmailState(r.Context(), st); err != nil {
		rt.internalError(w, "api: system email: save state", err)
		return
	}
	writeJSON(w, http.StatusOK, sysEmailResult{State: rt.sysEmailView(r.Context(), st), Steps: testSteps(t)})
}
