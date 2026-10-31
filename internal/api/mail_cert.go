package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/mailcert"
	"github.com/GLINCKER/levelrail/internal/store"
	"github.com/GLINCKER/levelrail/internal/volumefiles"
)

const (
	mailCertStepUpApp     = "mail-cert"
	mailConfigVolume      = mailcert.ConfigVolume
	mailCertPEMEnvKey     = "MAIL_TLS_CERT_PEM"
	mailCertKeyPEMEnvKey  = "MAIL_TLS_KEY_PEM"
	mailCertSelfSignedTag = "certs/.selfsigned"
	errCodeMailCertACME   = "mail_cert_acme_not_implemented"
	errCodeMailCertNoKey  = "mail_cert_no_master_key"
	mailCertSourceUpload  = "uploaded"
)

func mailCertSecretsKey(app string) string { return "mail-tls-cert/" + app }

type mailCertStatusResponse struct {
	App           string `json:"app"`
	Hostname      string `json:"hostname"`
	Source        string `json:"source"`
	Served        bool   `json:"served"`
	Subject       string `json:"subject,omitempty"`
	Issuer        string `json:"issuer,omitempty"`
	NotAfter      string `json:"not_after,omitempty"`
	DaysLeft      int    `json:"days_left"`
	SelfSigned    bool   `json:"self_signed"`
	Serial        string `json:"serial,omitempty"`
	SHA256        string `json:"sha256,omitempty"`
	ProbeError    string `json:"probe_error,omitempty"`
	RestartWindow string `json:"restart_window"`
	NextStep      string `json:"next_step"`
}

type mailCertUploadRequest struct {
	App        string       `json:"app,omitempty"`
	Cert       string       `json:"cert"`
	Key        string       `json:"key"`
	RestartNow bool         `json:"restart_now,omitempty"`
	StepUp     *stepUpProof `json:"step_up,omitempty"`
}

type mailCertUploadResponse struct {
	App          string `json:"app"`
	Outcome      string `json:"outcome"`
	Reason       string `json:"reason,omitempty"`
	Message      string `json:"message"`
	Serial       string `json:"serial"`
	SHA256       string `json:"sha256"`
	NotAfter     string `json:"not_after"`
	DaysLeft     int    `json:"days_left"`
	PreviousKept bool   `json:"previous_kept"`
	// Recovered is set when an earlier interrupted install was finished first.
	Recovered string `json:"recovered,omitempty"`
	// Error repeats Message on failure so clients that read the error field show it.
	Error string `json:"error,omitempty"`
}

func mailCertSource(env map[string]string) string {
	if mailEnvValue(env, "MAIL_TLS_CERT_FILE") == mailcert.CertPath {
		return mailCertSourceUpload
	}
	return mailTLSKind(env)
}

// mailCertServed probes the engine container's private submission port.
func (rt *Router) mailCertServed(ctx context.Context, svc store.DesiredService, cfg mailcert.Config) (mailcert.Info, error) {
	c, err := rt.mailEngineContainer(ctx, svc)
	if err != nil {
		return mailcert.Info{}, err
	}
	if !c.Running {
		return mailcert.Info{}, errMailEngineMissing
	}
	return mailcert.Probe(ctx, containerIP(c), mailcert.SubmissionTLSPort, cfg.ProbeTimeout)
}

func (rt *Router) handleMailCertStatus(w http.ResponseWriter, r *http.Request) {
	cfg, err := mailcert.ConfigFromEnv(os.LookupEnv)
	if err != nil {
		rt.internalError(w, "api: handleMailCertStatus failed", err)
		return
	}
	if !rt.authorizeMailApp(w, r, r.URL.Query().Get("app")) {
		return
	}
	svc, found, err := rt.pickMailApp(r, r.URL.Query().Get("app"))
	if err != nil {
		rt.internalError(w, "api: mail cert status: list services failed", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no mail app is installed, install the mail-mox template first")
		return
	}
	_, host := mailNames(svc.Env)
	out := mailCertStatusResponse{App: svc.Name, Hostname: host, Source: mailCertSource(svc.Env), RestartWindow: cfg.Window.String()}
	info, err := rt.mailCertServed(r.Context(), svc, cfg)
	if err != nil {
		out.ProbeError = "the mail server did not answer TLS on its private submission port"
		out.NextStep = "Start the mail app, then check again."
		writeJSON(w, http.StatusOK, out)
		return
	}
	now := time.Now()
	out.Served, out.Subject, out.Issuer, out.SelfSigned = true, info.Subject, info.Issuer, info.SelfSigned
	out.NotAfter, out.DaysLeft, out.Serial, out.SHA256 = info.NotAfter.Format(time.RFC3339), info.DaysLeft(now), info.Serial, info.SHA256
	switch {
	case info.SelfSigned:
		out.NextStep = "Upload a real certificate for " + host + " with mail cert upload before sending real mail."
	case out.DaysLeft < 0:
		out.NextStep = "The served certificate has expired. Upload a new one."
	default:
		out.NextStep = "No action needed until the certificate nears expiry."
	}
	writeJSON(w, http.StatusOK, out)
}

func (rt *Router) handleMailCertObtain(w http.ResponseWriter, _ *http.Request) {
	writeCodedError(w, http.StatusNotImplemented, errCodeMailCertACME, mailcert.ErrACMENotImplemented.Error())
}

func (rt *Router) handleMailCertUpload(w http.ResponseWriter, r *http.Request) {
	cfg, err := mailcert.ConfigFromEnv(os.LookupEnv)
	if err != nil {
		rt.internalError(w, "api: handleMailCertUpload failed", err)
		return
	}
	var req mailCertUploadRequest
	if err := decodeJSONBodyLimit(r, &req, 2*cfg.MaxPEMBytes+(1<<12)); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body, or the certificate and key are larger than the upload limit")
		return
	}
	certPEM, keyPEM := []byte(strings.TrimSpace(req.Cert)+"\n"), []byte(strings.TrimSpace(req.Key)+"\n")
	req.Key = ""
	if err := cfg.CheckSize(certPEM, keyPEM); err != nil {
		writeError(w, http.StatusRequestEntityTooLarge, err.Error())
		return
	}
	if !rt.authorizeMailApp(w, r, req.App) {
		return
	}
	svc, found, err := rt.pickMailApp(r, req.App)
	if err != nil {
		rt.internalError(w, "api: mail cert upload: list services failed", err)
		return
	}
	if !found {
		writeError(w, http.StatusNotFound, "no mail app is installed, install the mail-mox template first")
		return
	}
	if f := mailEnvValue(svc.Env, "MAIL_TLS_CERT_FILE"); f != "" && f != mailSelfSignedCert && f != mailcert.CertPath {
		writeError(w, http.StatusConflict, "this mail app reads your own certificate files ("+f+"), which are never overwritten. Clear MAIL_TLS_CERT_FILE and MAIL_TLS_KEY_FILE to let uploads manage the certificate.")
		return
	}
	_, host := mailNames(svc.Env)
	now := time.Now()
	info, err := mailcert.Validate(certPEM, keyPEM, host, now)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if !req.RestartNow && !cfg.Window.Contains(now) {
		writeError(w, http.StatusConflict, "installing restarts the mail server, and now is outside the restart window ("+cfg.Window.String()+"). Retry inside it, or pass restart_now.")
		return
	}
	if rt.domainTLSCertSecrets == nil {
		writeCodedError(w, http.StatusNotImplemented, errCodeMailCertNoKey, msgTLSCertNoMasterKey)
		return
	}
	vol, ok := rt.mailCertVolume(w, svc, cfg)
	if !ok {
		return
	}
	if _, ok := rt.stepUpOwner(w, r, req.StepUp, mailCertStepUpApp+":"+svc.Name, "upload a mail certificate", "uploading a mail certificate"); !ok {
		return
	}
	unlock, ok := rt.lockMailCert(svc.Name)
	if !ok {
		writeError(w, http.StatusConflict, mailcert.ErrInstallInProgress.Error())
		return
	}
	defer unlock()
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*cfg.VerifyTimeout)
	defer cancel()
	detail := map[string]any{"app": svc.Name, "host": host, "serial": info.Serial, "sha256": info.SHA256, "not_after": info.NotAfter.Format(time.RFC3339)}
	out := mailCertUploadResponse{
		App: svc.Name, Serial: info.Serial, SHA256: info.SHA256,
		NotAfter: info.NotAfter.Format(time.RFC3339), DaysLeft: info.DaysLeft(now),
	}
	recovered, ok := rt.recoverMailCert(ctx, w, r, svc, cfg, vol, detail, out)
	if !ok {
		return
	}
	out.Recovered = recovered
	for _, kv := range [][2]string{{mailCertPEMEnvKey, string(certPEM)}, {mailCertKeyPEMEnvKey, string(keyPEM)}} {
		if err := rt.domainTLSCertSecrets.SetValue(r.Context(), mailCertSecretsKey(svc.Name), kv[0], kv[1]); err != nil {
			rt.internalError(w, "api: mail cert upload: seal failed", err, slog.String("name", svc.Name))
			return
		}
	}
	rt.auditMailCert(r, "mail.cert.uploaded", detail)

	orig := mailCertOrigin(svc.Env)
	sw := &mailCertSwitch{rt: rt, name: svc.Name, orig: orig, cfg: cfg}
	res, err := mailcert.Install(ctx, cfg, vol, sw, certPEM, keyPEM, info, orig)
	out.Outcome, out.Reason, out.PreviousKept = res.Outcome, res.Reason, res.HadPrev
	detail["reason"] = res.Reason
	status := http.StatusBadGateway
	switch {
	case res.Outcome == mailcert.OutcomeInstalled:
		_ = vol.Remove(ctx, mailCertSelfSignedTag)
		rt.auditMailCert(r, "mail.cert.delivered", detail)
		rt.recordAppEvent(r, store.AppEvent{AppName: svc.Name, Kind: store.AppEventConfigChange, Title: "Mail certificate installed", Detail: "serial " + info.Serial})
		out.Message = "The mail server restarted and now serves the uploaded certificate."
		writeJSON(w, http.StatusOK, out)
		return
	case errors.Is(err, mailcert.ErrInstallInProgress):
		status = http.StatusConflict
		out.Message = mailcert.ErrInstallInProgress.Error()
	case res.Outcome == mailcert.OutcomeRolledBack:
		rt.auditMailCert(r, "mail.cert.rolled_back", detail)
		out.Message = "The mail server did not come back serving the new certificate, so the previous one was restored. The upload stays stored."
	case res.Outcome == mailcert.OutcomeRollbackFailed:
		rt.auditMailCert(r, "mail.cert.rollback_failed", detail)
		out.Message = msgMailCertRollbackFailed
	default:
		rt.auditMailCert(r, "mail.cert.failed", detail)
		out.Message = "Installing the certificate failed. Check the mail app and the node, then retry."
		if errors.Is(err, mailcert.ErrKeyModeTooWide) || errors.Is(err, mailcert.ErrKeyRemoveFailed) {
			out.Message = mailcert.ErrKeyModeTooWide.Error()
			if errors.Is(err, mailcert.ErrKeyRemoveFailed) {
				out.Message = mailcert.ErrKeyRemoveFailed.Error()
			}
		}
	}
	out.Error = out.Outcome + ": " + out.Message
	rt.logger.Error("api: mail cert install failed", slog.String("name", svc.Name), slog.String("outcome", res.Outcome), slog.String("reason", res.Reason), slog.String("error", errString(err)))
	writeJSON(w, status, out)
}

const msgMailCertRollbackFailed = "Installing failed and restoring the previous certificate also failed, so the mail server may serve a missing or mismatched pair. Run mail cert status; the next upload retries the rollback first."

// dropMailCertSecret deletes the sealed uploaded pair of a mail app that is being removed.
func (rt *Router) dropMailCertSecret(ctx context.Context, svc store.DesiredService) {
	if svc.TemplateID != mailcert.TemplateID || rt.domainTLSCertSecrets == nil {
		return
	}
	if err := rt.domainTLSCertSecrets.DeleteAll(ctx, mailCertSecretsKey(svc.Name)); err != nil {
		rt.logger.Warn("api: delete app: drop sealed mail certificate failed", slog.String("name", svc.Name), slog.String("error", err.Error()))
	}
}

func mailCertOrigin(env map[string]string) mailcert.Origin {
	cert, set := env["MAIL_TLS_CERT_FILE"]
	return mailcert.Origin{CertFile: cert, KeyFile: env["MAIL_TLS_KEY_FILE"], Set: set}
}

// lockMailCert serializes installs per mail app inside this process; the journal covers other processes and crashes.
func (rt *Router) lockMailCert(name string) (func(), bool) {
	v, _ := rt.mailCertLocks.LoadOrStore(name, &sync.Mutex{})
	mu, _ := v.(*sync.Mutex)
	if !mu.TryLock() {
		return nil, false
	}
	return mu.Unlock, true
}

// recoverMailCert answers 409 while another install's journal is inside its
// lease, and rolls back a stale one before a new install starts.
func (rt *Router) recoverMailCert(ctx context.Context, w http.ResponseWriter, r *http.Request, svc store.DesiredService, cfg mailcert.Config, vol mailcert.Volume, detail map[string]any, out mailCertUploadResponse) (string, bool) {
	j, found, err := mailcert.ReadJournal(ctx, vol)
	if err != nil {
		rt.logger.Error("api: mail cert: read install journal failed", slog.String("name", svc.Name), slog.String("error", err.Error()))
		writeError(w, http.StatusBadGateway, "the install journal on the mail volume could not be read, check the node and retry")
		return "", false
	}
	if !found {
		return "", true
	}
	if !j.Stale(cfg, time.Now()) {
		writeError(w, http.StatusConflict, mailcert.ErrInstallInProgress.Error())
		return "", false
	}
	res, err := mailcert.Recover(ctx, cfg, vol, &mailCertSwitch{rt: rt, name: svc.Name, orig: j.Origin, cfg: cfg}, j)
	rec := maps.Clone(detail)
	rec["reason"], rec["outcome"] = res.Reason, res.Outcome
	if err != nil {
		rt.auditMailCert(r, "mail.cert.rollback_failed", rec)
		rt.logger.Error("api: mail cert: recover interrupted install failed", slog.String("name", svc.Name), slog.String("error", err.Error()))
		out.Outcome, out.Reason, out.Message = mailcert.OutcomeRollbackFailed, res.Reason, msgMailCertRollbackFailed
		out.Error = out.Outcome + ": " + out.Message
		writeJSON(w, http.StatusBadGateway, out)
		return "", false
	}
	rt.auditMailCert(r, "mail.cert.recovered", rec)
	return res.Outcome, true
}

// privateVolumeWriter is the node surface that creates a file 0600.
type privateVolumeWriter interface {
	VolumeWritePrivate(ctx context.Context, volume, path string, content io.Reader, maxBytes int64) error
}

type mailCertNode interface {
	VolumeFileNode
	privateVolumeWriter
}

func (rt *Router) mailCertVolume(w http.ResponseWriter, svc store.DesiredService, cfg mailcert.Config) (*mailCertVolume, bool) {
	dockerVolume, found := store.ServiceVolumeDockerName(svc, mailConfigVolume)
	if !found || rt.execRuntime == nil {
		writeError(w, http.StatusConflict, "this mail app has no "+mailConfigVolume+" volume the control plane can write to")
		return nil, false
	}
	nodeRuntime, err := rt.execRuntime(svc.NodeID)
	if err != nil {
		rt.logger.Error("api: mail cert: resolve node runtime failed", slog.String("error", err.Error()), slog.String("node_id", svc.NodeID))
		writeError(w, http.StatusBadGateway, "the node holding the mail app is not currently reachable")
		return nil, false
	}
	node, ok := nodeRuntime.(mailCertNode)
	if !ok {
		writeError(w, http.StatusNotImplemented, "the node holding the mail app cannot write private files, upgrade its agent")
		return nil, false
	}
	return &mailCertVolume{node: node, volume: dockerVolume, max: cfg.MaxPEMBytes}, true
}

type mailCertVolume struct {
	node   mailCertNode
	volume string
	max    int64
}

func (v *mailCertVolume) Read(ctx context.Context, p string) ([]byte, bool, error) {
	rc, err := v.node.VolumeRead(ctx, v.volume, p)
	if errors.Is(volumefiles.Classify(err), volumefiles.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rc.Close() }()
	b, err := io.ReadAll(io.LimitReader(rc, v.max+1))
	if errors.Is(volumefiles.Classify(err), volumefiles.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if int64(len(b)) > v.max {
		return nil, false, volumefiles.ErrTooLarge
	}
	return b, true, nil
}

func (v *mailCertVolume) Write(ctx context.Context, p string, b []byte, private bool) error {
	if private {
		return v.node.VolumeWritePrivate(ctx, v.volume, p, bytes.NewReader(b), v.max)
	}
	return v.node.VolumeWrite(ctx, v.volume, p, bytes.NewReader(b), v.max)
}

func (v *mailCertVolume) Mode(ctx context.Context, p string) (fs.FileMode, error) {
	entries, _, err := v.node.VolumeList(ctx, v.volume, path.Dir(p), 0)
	if err != nil {
		return 0, err
	}
	for _, e := range entries {
		if e.Path == p {
			return parsePermString(e.Mode)
		}
	}
	return 0, volumefiles.ErrNotFound
}

func (v *mailCertVolume) Remove(ctx context.Context, p string) error {
	if err := v.node.VolumeRemove(ctx, v.volume, p); err != nil && !errors.Is(volumefiles.Classify(err), volumefiles.ErrNotFound) {
		return err
	}
	return nil
}

// parsePermString reads fs.FileMode.Perm().String() output such as "-rw-------".
func parsePermString(s string) (fs.FileMode, error) {
	if len(s) != 10 {
		return 0, fmt.Errorf("mail cert: unexpected file mode %q", s)
	}
	var m fs.FileMode
	for i, c := range s[1:] {
		if c != '-' {
			m |= 1 << (8 - i)
		}
	}
	return m, nil
}

// mailCertSwitch points the mail app at the managed files and restarts it through the normal restart path.
type mailCertSwitch struct {
	rt   *Router
	name string
	orig mailcert.Origin
	cfg  mailcert.Config
}

func (s *mailCertSwitch) apply(ctx context.Context, cert, key string, present bool) error {
	svc, err := s.rt.apps.GetDesiredService(ctx, s.name)
	if err != nil {
		return err
	}
	next := *svc
	next.Env = maps.Clone(svc.Env)
	if next.Env == nil {
		next.Env = map[string]string{}
	}
	if present {
		next.Env["MAIL_TLS_CERT_FILE"], next.Env["MAIL_TLS_KEY_FILE"] = cert, key
	} else {
		delete(next.Env, "MAIL_TLS_CERT_FILE")
		delete(next.Env, "MAIL_TLS_KEY_FILE")
	}
	if !maps.Equal(next.Env, svc.Env) {
		if err := s.rt.apps.SaveDesiredService(ctx, next); err != nil {
			return err
		}
	}
	if err := s.rt.apps.RestartService(ctx, s.name); err != nil {
		return err
	}
	s.rt.nudgeReconciler()
	return nil
}

func (s *mailCertSwitch) Activate(ctx context.Context) error {
	return s.apply(ctx, mailcert.CertPath, mailcert.KeyPath, true)
}

func (s *mailCertSwitch) Revert(ctx context.Context) error {
	return s.apply(ctx, s.orig.CertFile, s.orig.KeyFile, s.orig.Set)
}

func (s *mailCertSwitch) Served(ctx context.Context) (mailcert.Info, error) {
	svc, err := s.rt.apps.GetDesiredService(ctx, s.name)
	if err != nil {
		return mailcert.Info{}, err
	}
	return s.rt.mailCertServed(ctx, *svc, s.cfg)
}

// auditMailCert writes a row with public certificate facts only: never key material.
func (rt *Router) auditMailCert(r *http.Request, action string, detail map[string]any) {
	if rt.auditLog == nil {
		return
	}
	a := rt.accessActor(r)
	id, err := store.NewAuditEntryID()
	if err != nil {
		rt.logger.Warn("api: mail cert audit id failed", slog.String("error", err.Error()))
		return
	}
	raw, _ := json.Marshal(detail)
	entry := store.AuditEntry{
		ID: id, ActorType: a.kind, ActorID: a.id, ActorName: a.name, Ability: AbilityWriteSensitive,
		Method: r.Method, Path: r.URL.Path, StatusCode: http.StatusOK, RemoteAddr: clientIP(r),
		CreatedAt: store.FormatAuditTime(time.Now()), ClientKind: clientKindFromUserAgent(r.Header.Get("User-Agent")),
		Action: action, Detail: string(raw),
	}
	if err := rt.auditLog.SaveAuditEntry(context.WithoutCancel(r.Context()), entry); err != nil {
		rt.logger.Warn("api: mail cert audit save failed", slog.String("action", action), slog.String("error", err.Error()))
	}
}
