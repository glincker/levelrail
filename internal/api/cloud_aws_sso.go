package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/GLINCKER/levelrail/internal/cloudconn"
	"github.com/GLINCKER/levelrail/internal/store"
)

// ssoSession is the server side half of one device login; tokens never reach the browser.
type ssoSession struct {
	actor    string
	region   string
	startURL string
	bound    string // existing account id for a relogin
	reg      cloudconn.SSOBundle
	device   cloudconn.SSODevice
	interval time.Duration
	lastPoll time.Time
	expires  time.Time
	token    *cloudconn.SSOBundle
	accounts []ssoAccountWire
}

type ssoAccountWire struct {
	AccountID   string   `json:"account_id"`
	AccountName string   `json:"account_name"`
	Roles       []string `json:"roles"`
}

var awsSSOSessions = struct {
	sync.Mutex
	m map[string]*ssoSession
}{m: map[string]*ssoSession{}}

func ssoEnvInt(name string, def int) int {
	if v, err := strconv.Atoi(strings.TrimSpace(os.Getenv(name))); err == nil && v > 0 {
		return v
	}
	return def
}

func putSSOSession(s *ssoSession) (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	awsSSOSessions.Lock()
	defer awsSSOSessions.Unlock()
	now := time.Now()
	for k, v := range awsSSOSessions.m {
		if now.After(v.expires) {
			delete(awsSSOSessions.m, k)
		}
	}
	awsSSOSessions.m[id] = s
	return id, nil
}

func getSSOSession(id, actor string) (*ssoSession, bool) {
	awsSSOSessions.Lock()
	defer awsSSOSessions.Unlock()
	s := awsSSOSessions.m[id]
	if s == nil || s.actor != actor || time.Now().After(s.expires) {
		delete(awsSSOSessions.m, id)
		return nil, false
	}
	return s, true
}

func dropSSOSession(id string) {
	awsSSOSessions.Lock()
	delete(awsSSOSessions.m, id)
	awsSSOSessions.Unlock()
}

func isAWSSSOAccount(a store.CloudAccount) bool {
	return a.Provider == cloudconn.ProviderAWS && a.AuthMethod == cloudconn.AuthAWSSSO
}

// persistSSOBundle seals a refreshed bundle back under the account's secret.
func (rt *Router) persistSSOBundle(ctx context.Context, account string, bundle []byte) error {
	st, ok := rt.cloudAcctStore()
	if !ok || rt.nodeProviderSecrets == nil {
		return errors.New("no secrets backend")
	}
	a, err := st.GetCloudAccount(ctx, account)
	if err != nil {
		return err
	}
	return rt.nodeProviderSecrets.SetValue(ctx, cloudAccountSecretRef(a), store.NodeProviderTokenEnvKey, string(bundle))
}

// ssoNeedsRelogin reads the sealed bundle for the account list; a read failure counts as not flagged.
func (rt *Router) ssoNeedsRelogin(ctx context.Context, a store.CloudAccount) bool {
	if !isAWSSSOAccount(a) || rt.nodeProviderSecrets == nil {
		return false
	}
	v, err := rt.nodeProviderSecrets.Resolve(ctx, cloudAccountSecretRef(a), store.NodeProviderTokenEnvKey)
	return err == nil && cloudconn.SSONeedsRelogin([]byte(v), time.Now())
}

type ssoStartRequest struct {
	StartURL string `json:"start_url"`
	Region   string `json:"region"`
}

type ssoStartResponse struct {
	SessionID               string `json:"session_id"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	Interval                int    `json:"interval"`
	ExpiresIn               int    `json:"expires_in"`
}

func (rt *Router) beginSSO(w http.ResponseWriter, r *http.Request, startURL, region, bound string) {
	if err := cloudconn.ValidateSSOStart(startURL, region); err != nil {
		writeError(w, http.StatusBadRequest, "start_url and region are not valid")
		return
	}
	br := rt.awsProvider().SSOBroker()
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout)
	defer cancel()
	reg, err := br.Client.Register(ctx, region, startURL, rt.brand.ShortName)
	if err != nil {
		writeError(w, http.StatusBadGateway, "the AWS sign in service could not be reached")
		return
	}
	dev, err := br.Client.StartDevice(ctx, region, startURL, reg)
	if err != nil {
		writeError(w, http.StatusBadGateway, "the AWS sign in service refused the login")
		return
	}
	s := &ssoSession{actor: rt.nodeProviderActor(r), region: region, startURL: startURL, bound: bound, reg: reg, device: dev,
		interval: time.Duration(dev.Interval) * time.Second, expires: time.Now().Add(time.Duration(dev.ExpiresIn) * time.Second)}
	id, err := putSSOSession(s)
	if err != nil {
		rt.internalError(w, "api: mint aws sso session failed", err)
		return
	}
	rt.auditCloud(r, "cloud.aws_sso.started", map[string]any{"region": region, "start_url": startURL, "account": bound})
	writeJSON(w, http.StatusOK, ssoStartResponse{SessionID: id, UserCode: dev.UserCode, VerificationURI: dev.VerificationURI,
		VerificationURIComplete: dev.VerificationURIComplete, Interval: dev.Interval, ExpiresIn: dev.ExpiresIn})
}

func (rt *Router) handleAWSSSOStart(w http.ResponseWriter, r *http.Request) {
	var req ssoStartRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	rt.beginSSO(w, r, strings.TrimSpace(req.StartURL), strings.TrimSpace(req.Region), "")
}

type ssoRELoginRequest struct {
	AccountID string `json:"account_id"`
}

func (rt *Router) handleAWSSSORelogin(w http.ResponseWriter, r *http.Request) {
	var req ssoRELoginRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	st, ok := rt.cloudStoreOr501(w)
	if !ok {
		return
	}
	a, err := st.GetCloudAccount(r.Context(), req.AccountID)
	if err != nil || !isAWSSSOAccount(a) {
		writeError(w, http.StatusNotFound, "no aws sso account with this id")
		return
	}
	var cfg cloudconn.Config
	_ = json.Unmarshal(a.Config, &cfg)
	rt.beginSSO(w, r, cfg.StartURL, cfg.SSORegion, a.ID)
}

type ssoPollRequest struct {
	SessionID string `json:"session_id"`
}

type ssoPollResponse struct {
	Status    string           `json:"status"`
	Interval  int              `json:"interval,omitempty"`
	Accounts  []ssoAccountWire `json:"accounts,omitempty"`
	AccountID string           `json:"account_id,omitempty"`
}

func (rt *Router) handleAWSSSOPoll(w http.ResponseWriter, r *http.Request) {
	var req ssoPollRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s, ok := getSSOSession(req.SessionID, rt.nodeProviderActor(r))
	if !ok {
		writeError(w, http.StatusGone, "the login session is gone, start again")
		return
	}
	if s.token != nil {
		writeJSON(w, http.StatusOK, ssoPollResponse{Status: "done", Accounts: s.accounts, AccountID: s.bound})
		return
	}
	// Honor the interval server side so a fast client cannot trigger slow_down.
	if time.Since(s.lastPoll) < s.interval {
		writeJSON(w, http.StatusOK, ssoPollResponse{Status: "pending", Interval: int(s.interval.Seconds())})
		return
	}
	s.lastPoll = time.Now()
	br := rt.awsProvider().SSOBroker()
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout*3)
	defer cancel()
	tok, err := br.Client.DeviceToken(ctx, s.region, s.reg, s.device.DeviceCode, time.Now())
	switch {
	case errors.Is(err, cloudconn.ErrSSOPending):
		writeJSON(w, http.StatusOK, ssoPollResponse{Status: "pending", Interval: int(s.interval.Seconds())})
		return
	case errors.Is(err, cloudconn.ErrSSOSlowDown):
		s.interval += 5 * time.Second
		writeJSON(w, http.StatusOK, ssoPollResponse{Status: "slow_down", Interval: int(s.interval.Seconds())})
		return
	case errors.Is(err, cloudconn.ErrSSODenied):
		dropSSOSession(req.SessionID)
		writeJSON(w, http.StatusOK, ssoPollResponse{Status: "denied"})
		return
	case err != nil:
		writeError(w, http.StatusBadGateway, "the AWS sign in service could not be reached")
		return
	}
	s.token = &tok
	if s.bound != "" {
		rt.finishSSORelogin(w, r, req.SessionID, s)
		return
	}
	accts, err := br.Client.ListAccounts(ctx, s.region, tok.AccessToken, ssoEnvInt("APP_CLOUD_AWS_SSO_MAX_ACCOUNTS", 100))
	if err != nil {
		s.token = nil
		writeError(w, http.StatusBadGateway, "the AWS account list could not be read")
		return
	}
	for _, a := range accts {
		roles, rerr := br.Client.ListRoles(ctx, s.region, tok.AccessToken, a.AccountID)
		if rerr != nil {
			continue
		}
		s.accounts = append(s.accounts, ssoAccountWire{AccountID: a.AccountID, AccountName: a.AccountName, Roles: roles})
	}
	rt.auditCloud(r, "cloud.aws_sso.authorized", map[string]any{"region": s.region, "accounts": len(s.accounts)})
	writeJSON(w, http.StatusOK, ssoPollResponse{Status: "done", Accounts: s.accounts})
}

// finishSSORelogin seals the new tokens under the existing account and clears the needs_relogin state.
func (rt *Router) finishSSORelogin(w http.ResponseWriter, r *http.Request, id string, s *ssoSession) {
	raw, _ := json.Marshal(s.token) //nolint:gosec // sealed bundle, never logged
	if err := rt.persistSSOBundle(r.Context(), s.bound, raw); err != nil {
		s.token = nil
		rt.internalError(w, "api: seal aws sso relogin failed", err, slog.String("account", s.bound))
		return
	}
	rt.awsProvider().SSOBroker().Forget(s.bound)
	rt.auditCloud(r, "cloud.aws_sso.relogin", map[string]any{"account": s.bound})
	dropSSOSession(id)
	writeJSON(w, http.StatusOK, ssoPollResponse{Status: "done", AccountID: s.bound})
}

type ssoSelectRequest struct {
	SessionID string `json:"session_id"`
	AccountID string `json:"account_id"`
	RoleName  string `json:"role_name"`
	Name      string `json:"name,omitempty"`
	ID        string `json:"id,omitempty"`
	TeamID    string `json:"team_id,omitempty"`
	Region    string `json:"region,omitempty"`
}

func (rt *Router) handleAWSSSOSelect(w http.ResponseWriter, r *http.Request) {
	st, ok := rt.cloudStoreOr501(w)
	if !ok {
		return
	}
	if rt.nodeProviderSecrets == nil {
		writeError(w, http.StatusNotImplemented, "cloud accounts need a master key on this control plane")
		return
	}
	var req ssoSelectRequest
	if err := decodeJSONBody(r, &req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	s, ok := getSSOSession(req.SessionID, rt.nodeProviderActor(r))
	if !ok || s.token == nil || s.bound != "" {
		writeError(w, http.StatusGone, "the login session is gone, start again")
		return
	}
	label, found := "", false
	for _, a := range s.accounts {
		if a.AccountID != req.AccountID {
			continue
		}
		for _, role := range a.Roles {
			if role == req.RoleName {
				label, found = a.AccountName, true
			}
		}
	}
	if !found {
		writeError(w, http.StatusBadRequest, "pick an account and role from the list")
		return
	}
	region := req.Region
	if region == "" {
		region = s.region
	}
	cfg := cloudconn.Config{Region: region, AccountID: req.AccountID, StartURL: s.startURL, SSORegion: s.region, RoleName: req.RoleName}
	if err := rt.awsProvider().ValidateConfig(cfg, cloudconn.AuthAWSSSO); err != nil {
		writeError(w, http.StatusBadRequest, "the account settings are not valid")
		return
	}
	raw, _ := json.Marshal(cfg)
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = label + " " + req.RoleName
	}
	if len(name) > nodeProviderMaxMetaLen {
		writeError(w, http.StatusBadRequest, "label is limited to 500 characters")
		return
	}
	id := req.ID
	if id != "" && (!cloudAccountIDRe.MatchString(id) || strings.HasSuffix(id, store.CloudAccountDefaultSuffix)) {
		writeError(w, http.StatusBadRequest, "id must be lowercase letters, digits and hyphens, and cannot end in -default")
		return
	}
	if id == "" {
		var err error
		if id, err = newCloudID("aws-"); err != nil {
			rt.internalError(w, "api: mint cloud account id failed", err)
			return
		}
	}
	if _, err := st.GetCloudAccount(r.Context(), id); err == nil {
		writeError(w, http.StatusConflict, "a cloud account with this id already exists")
		return
	}
	acct := store.CloudAccount{ID: id, Provider: cloudconn.ProviderAWS, DisplayName: name, AuthMethod: cloudconn.AuthAWSSSO, Config: raw, TeamID: req.TeamID,
		CredentialRef: cloudAccountSecretPref + id, CreatedBy: rt.nodeProviderActor(r)}
	bundle, _ := json.Marshal(s.token) //nolint:gosec // sealed bundle, never logged
	ctx, cancel := context.WithTimeout(r.Context(), nodeProviderTestTimeout)
	defer cancel()
	if res := rt.checkCloudAccount(ctx, acct, string(bundle)); !res.OK {
		writeJSON(w, http.StatusUnprocessableEntity, res)
		return
	}
	if err := rt.nodeProviderSecrets.SetValue(r.Context(), acct.CredentialRef, store.NodeProviderTokenEnvKey, string(bundle)); err != nil {
		rt.internalError(w, "api: seal aws sso bundle failed", err, slog.String("account", id))
		return
	}
	if err := st.CreateCloudAccount(r.Context(), acct); err != nil {
		rt.internalError(w, "api: create cloud account failed", err, slog.String("account", id))
		return
	}
	dropSSOSession(req.SessionID)
	rt.nodeProviderCatalog.invalidate(acct.Provider)
	got, _ := st.GetCloudAccount(r.Context(), id)
	rt.auditCloud(r, "cloud.account.created", cloudAccountAuditDetail(acct))
	writeJSON(w, http.StatusCreated, rt.toCloudAccountWire(r.Context(), got))
}
