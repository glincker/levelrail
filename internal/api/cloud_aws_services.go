package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"

	"github.com/glincker/levelrail/internal/awsx"
	"github.com/glincker/levelrail/internal/awsx/acm"
	"github.com/glincker/levelrail/internal/awsx/cloudtrail"
	"github.com/glincker/levelrail/internal/awsx/ecr"
	"github.com/glincker/levelrail/internal/awsx/rds"
	"github.com/glincker/levelrail/internal/awsx/secrets"
	"github.com/glincker/levelrail/internal/awsx/ssm"
	"github.com/glincker/levelrail/internal/cloudconn"
	"github.com/glincker/levelrail/internal/cloudguard"
	"github.com/glincker/levelrail/internal/cloudspend"
	"github.com/glincker/levelrail/internal/runtimesettings"
	"github.com/glincker/levelrail/internal/store"
)

// newAWSXClient is the seam tests replace to point the modules at a fake endpoint.
var newAWSXClient = func(c awsx.Credentials) (*awsx.Client, error) { return awsx.New(awsx.Options{Credentials: c}) }

const (
	awsServiceOffMessage = "This AWS service is not enabled. Turn it on for the account in Services or in runtime settings."
	awsRedacted          = "[redacted]"
	awsDefaultRegion     = "us-east-1"
)

type awsServiceDef struct{ key, env string }

var awsServices = map[string]awsServiceDef{
	ecr.ServiceName: {runtimesettings.KeyCloudGuardECR, "APP_CLOUD_GUARD_ECR"},
	"acm":           {runtimesettings.KeyCloudGuardACM, "APP_CLOUD_GUARD_ACM"},
	"secrets":       {runtimesettings.KeyCloudGuardParams, "APP_CLOUD_GUARD_PARAMS"},
	"cloudtrail":    {runtimesettings.KeyCloudGuardCloudTrail, "APP_CLOUD_GUARD_CLOUDTRAIL"},
	"rds":           {runtimesettings.KeyCloudGuardRDS, "APP_CLOUD_GUARD_RDS"},
	"elasticache":   {runtimesettings.KeyCloudGuardElastiCache, "APP_CLOUD_GUARD_ELASTICACHE"},
	"lightsail":     {runtimesettings.KeyCloudGuardLightsail, "APP_CLOUD_GUARD_LIGHTSAIL"},
	"ebs":           {runtimesettings.KeyCloudGuardEBS, "APP_CLOUD_GUARD_EBS"},
	"cloudwatch":    {runtimesettings.KeyCloudGuardCloudWatch, "APP_CLOUD_GUARD_CLOUDWATCH"},
}

func (rt *Router) awsServiceOn(svc string) bool {
	d, ok := awsServices[svc]
	return ok && rt.cloudSpendBool(d.key, d.env, false)
}

func (rt *Router) settingList(key string) []string {
	if rt.runtimeSettings == nil {
		return nil
	}
	return rt.runtimeSettings.List(context.Background(), key)
}

// cloudGuardConfig maps the live runtime settings onto the guard.
func (rt *Router) cloudGuardConfig() cloudguard.Config {
	ctx := context.Background()
	num := func(key, env string) float64 {
		if rt.runtimeSettings != nil {
			return float64(rt.runtimeSettings.Int(ctx, key))
		}
		return float64(envIntDefault(env, 0))
	}
	caps := map[string]float64{}
	for _, e := range rt.settingList(runtimesettings.KeyCloudGuardAccountCaps) {
		if id, v, ok := strings.Cut(e, "="); ok {
			if f, err := strconv.ParseFloat(strings.TrimSpace(v), 64); err == nil && f >= 0 {
				caps[strings.TrimSpace(id)] = f
			}
		}
	}
	on := map[string]bool{}
	for name := range awsServices {
		on[name] = rt.awsServiceOn(name)
	}
	return cloudguard.Config{
		KillSwitch:      rt.cloudSpendBool(runtimesettings.KeyCloudGuardKillSwitch, "APP_CLOUD_GUARD_KILL_SWITCH", false),
		MonthlyCapUSD:   num(runtimesettings.KeyCloudGuardMonthlyCapUSD, "APP_CLOUD_GUARD_MONTHLY_CAP_USD"),
		AccountCaps:     caps,
		PerCreateMaxUSD: num(runtimesettings.KeyCloudGuardPerCreateUSD, "APP_CLOUD_GUARD_PER_CREATE_MAX_USD"),
		InstanceTypes:   rt.settingList(runtimesettings.KeyCloudGuardInstanceTypes),
		ServicesEnabled: on,
		DefaultTTL:      time.Duration(num(runtimesettings.KeyCloudGuardExpiresHours, "APP_CLOUD_GUARD_DEFAULT_EXPIRES_HOURS")) * time.Hour,
	}
}

// awsCall is one request with its account, signed client and guard resolved.
type awsCall struct {
	acct   store.CloudAccount
	client *awsx.Client
	region string
	guard  *cloudguard.Guard
}

// awsCallFor resolves an AWS account for a service. svc may be empty for guard views that need no flag.
func (rt *Router) awsCallFor(w http.ResponseWriter, r *http.Request, svc, region string) (awsCall, bool) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return awsCall{}, false
	}
	if a.Provider != "aws" {
		writeError(w, http.StatusBadRequest, "this route is only available for AWS accounts")
		return awsCall{}, false
	}
	if svc != "" && !rt.awsServiceOn(svc) {
		writeError(w, http.StatusConflict, awsServiceOffMessage)
		return awsCall{}, false
	}
	if a.Status == store.CloudAccountStatusDisabled {
		writeError(w, http.StatusConflict, "cloud account is disabled")
		return awsCall{}, false
	}
	creds, err := rt.accountAWSCredentials(r.Context(), a)
	if err != nil {
		writeError(w, http.StatusBadRequest, "no usable credential is stored for this account")
		return awsCall{}, false
	}
	if region == "" {
		region = creds.Region
	}
	if region == "" {
		region = awsDefaultRegion
	}
	if !regionAllowed(a, region) {
		writeError(w, http.StatusForbidden, "this region is not allowed for the account")
		return awsCall{}, false
	}
	p, err := creds.CredentialsProvider(r.Context(), region)
	var got awsx.Credentials
	if err == nil {
		var c aws.Credentials
		if c, err = p.Retrieve(r.Context()); err == nil {
			got = awsx.Credentials{AccessKeyID: c.AccessKeyID, SecretAccessKey: c.SecretAccessKey, SessionToken: c.SessionToken}
		}
	}
	if err != nil {
		writeError(w, http.StatusBadGateway, "the AWS credential could not be used")
		return awsCall{}, false
	}
	client, err := newAWSXClient(got)
	if err != nil {
		writeError(w, http.StatusBadGateway, "the AWS credential could not be used")
		return awsCall{}, false
	}
	var src cloudguard.SpendSource
	if rd, rerr := rt.cloudSpendReader(r.Context(), a); rerr == nil {
		src = cloudguard.CacheSpend{Cache: rt.cloudSpendCache(), Fetch: func(string) cloudspend.Fetch { return rt.spendFetch(a, rd) }}
	}
	return awsCall{acct: a, client: client, region: region, guard: cloudguard.New(rt.cloudGuardConfig, src)}, true
}

// writeAWSError hides provider messages, which can echo request values.
func writeAWSError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ecr.ErrRefused), errors.Is(err, acm.ErrRefused), errors.Is(err, ssm.ErrRefused):
		writeError(w, http.StatusConflict, err.Error())
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		writeError(w, http.StatusGatewayTimeout, "the AWS call timed out")
	case awsx.ClassOf(err) == awsx.ClassAuth:
		writeError(w, http.StatusForbidden, "the AWS credential is not allowed to do that")
	case awsx.ClassOf(err) == awsx.ClassNotFound:
		writeError(w, http.StatusNotFound, "not found in AWS")
	case awsx.ClassOf(err) == awsx.ClassThrottle:
		writeError(w, http.StatusTooManyRequests, "AWS is throttling requests, retry later")
	default:
		writeError(w, http.StatusBadGateway, "the AWS call failed ("+awsx.CodeOf(err)+")")
	}
}

func (rt *Router) awsTagMeta(r *http.Request, id, env, project string) cloudguard.TagMeta {
	owner := "unknown"
	if _, pid, _, err := rt.callerPrincipal(r); err == nil && pid != "" {
		owner = pid
	}
	if env == "" {
		env = "default"
	}
	if project == "" {
		project = "default"
	}
	return cloudguard.TagMeta{Owner: owner, Env: env, Project: project, ID: id}
}

type ecrRepoWire struct {
	Name       string    `json:"name"`
	URI        string    `json:"uri"`
	ARN        string    `json:"arn"`
	ScanOnPush bool      `json:"scan_on_push"`
	CreatedAt  time.Time `json:"created_at"`
}

func toECRRepoWire(r ecr.Repository) ecrRepoWire {
	return ecrRepoWire{Name: r.Name, URI: r.URI, ARN: r.ARN, ScanOnPush: r.ScanOnPush, CreatedAt: r.CreatedAt}
}

func (rt *Router) handleAWSECRList(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, ecr.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	repos, err := ecr.New(c.client, c.guard, ecr.Options{}).DescribeRepositories(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	out := make([]ecrRepoWire, 0, len(repos))
	for _, x := range repos {
		out = append(out, toECRRepoWire(x))
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "repositories": out})
}

type ecrCreateBody struct {
	Name                string  `json:"name"`
	Region              string  `json:"region"`
	Env                 string  `json:"env"`
	Project             string  `json:"project"`
	EstimatedMonthlyUSD float64 `json:"estimated_monthly_usd"`
}

func (rt *Router) handleAWSECRCreate(w http.ResponseWriter, r *http.Request) {
	var body ecrCreateBody
	if err := decodeJSONBody(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		writeError(w, http.StatusBadRequest, "a repository name is required")
		return
	}
	c, ok := rt.awsCallFor(w, r, ecr.ServiceName, body.Region)
	if !ok {
		return
	}
	repo, err := ecr.New(c.client, c.guard, ecr.Options{UntaggedExpireDays: 14, KeepTaggedImages: 30}).CreateRepository(r.Context(), ecr.CreateInput{
		Account: c.acct.ID, Region: c.region, Name: body.Name, Meta: rt.awsTagMeta(r, body.Name, body.Env, body.Project), EstimatedMonthlyUSD: body.EstimatedMonthlyUSD,
	})
	if err != nil && repo.Name == "" {
		rt.auditCloud(r, "cloud.aws.ecr.create_refused", map[string]any{"account": c.acct.ID, "region": c.region, "name": body.Name})
		writeAWSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.ecr.create", map[string]any{"account": c.acct.ID, "region": c.region, "name": repo.Name})
	if err != nil {
		writeError(w, http.StatusBadGateway, "the repository was created but its lifecycle policy failed ("+awsx.CodeOf(err)+")")
		return
	}
	writeJSON(w, http.StatusCreated, toECRRepoWire(repo))
}

// handleAWSECRLoginToken returns a short lived registry login. The password is shown once and never stored or logged.
func (rt *Router) handleAWSECRLoginToken(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, ecr.ServiceName, r.URL.Query().Get("region"))
	if !ok {
		return
	}
	tok, err := ecr.New(c.client, c.guard, ecr.Options{}).GetAuthorizationToken(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.ecr.login_token", map[string]any{"account": c.acct.ID, "region": c.region})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]any{"registry": tok.Registry, "user": tok.User, "password": tok.Password.Reveal(), "expires_at": tok.ExpiresAt})
}

func (rt *Router) handleAWSACMList(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, "acm", r.URL.Query().Get("region"))
	if !ok {
		return
	}
	client := acm.New(c.client, c.guard)
	list, err := client.ListCertificates(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type certWire struct {
		ARN     string          `json:"arn"`
		Domain  string          `json:"domain"`
		Status  string          `json:"status"`
		Records []acmRecordWire `json:"records"`
	}
	out := make([]certWire, 0, len(list))
	for _, s := range list {
		cw := certWire{ARN: s.ARN, Domain: s.Domain, Status: s.Status, Records: []acmRecordWire{}}
		if s.Status == "PENDING_VALIDATION" {
			if d, derr := client.DescribeCertificate(r.Context(), c.region, s.ARN); derr == nil {
				cw.Records = toACMRecords(d.Records)
			}
		}
		out = append(out, cw)
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "certificates": out})
}

type acmRecordWire struct {
	Domain string `json:"domain"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Value  string `json:"value"`
	Status string `json:"status"`
}

func toACMRecords(in []acm.ValidationRecord) []acmRecordWire {
	out := make([]acmRecordWire, 0, len(in))
	for _, x := range in {
		out = append(out, acmRecordWire{Domain: x.Domain, Name: x.Name, Type: x.Type, Value: x.Value, Status: x.Status})
	}
	return out
}

type acmRequestBody struct {
	Domain   string   `json:"domain"`
	AltNames []string `json:"alt_names"`
	Region   string   `json:"region"`
	Env      string   `json:"env"`
	Project  string   `json:"project"`
}

func (rt *Router) handleAWSACMRequest(w http.ResponseWriter, r *http.Request) {
	var body acmRequestBody
	if err := decodeJSONBody(r, &body); err != nil || strings.TrimSpace(body.Domain) == "" {
		writeError(w, http.StatusBadRequest, "a domain is required")
		return
	}
	c, ok := rt.awsCallFor(w, r, "acm", body.Region)
	if !ok {
		return
	}
	client := acm.New(c.client, c.guard)
	arn, err := client.RequestCertificate(r.Context(), acm.RequestInput{Account: c.acct.ID, Region: c.region, Domain: body.Domain, AltNames: body.AltNames,
		Meta: rt.awsTagMeta(r, body.Domain, body.Env, body.Project)})
	if err != nil {
		rt.auditCloud(r, "cloud.aws.acm.request_refused", map[string]any{"account": c.acct.ID, "region": c.region, "domain": body.Domain})
		writeAWSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.acm.request", map[string]any{"account": c.acct.ID, "region": c.region, "domain": body.Domain, "arn": arn})
	// Validation records appear a few seconds after the request; an empty list means poll the list route.
	records := []acmRecordWire{}
	if d, derr := client.DescribeCertificate(r.Context(), c.region, arn); derr == nil {
		records = toACMRecords(d.Records)
	}
	writeJSON(w, http.StatusCreated, map[string]any{"arn": arn, "domain": body.Domain, "records": records})
}

type ssmParamWire struct {
	Name  string `json:"name"`
	Type  string `json:"type"`
	Value string `json:"value"`
}

// awsNamePrefix is the live secret and parameter name prefix, matching the role policy scope.
func (rt *Router) awsNamePrefix() string {
	v := os.Getenv("APP_CLOUD_AWS_NAME_PREFIX")
	if rt.runtimeSettings != nil {
		v = rt.runtimeSettings.String(context.Background(), runtimesettings.KeyCloudAWSNamePrefix)
	}
	return cloudconn.NormalizeNamePrefix(v)
}

// writeNamePrefixError names the prefix so the operator knows which setting to change.
func writeNamePrefixError(w http.ResponseWriter, what, prefix string) {
	writeError(w, http.StatusBadRequest, what+" must start with the name prefix "+prefix+" (setting cloud.aws.name_prefix)")
}

func (rt *Router) handleAWSSecretsList(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, "secrets", r.URL.Query().Get("region"))
	if !ok {
		return
	}
	np := rt.awsNamePrefix()
	paramRoot := "/" + np + "/"
	prefix := r.URL.Query().Get("path")
	if prefix == "" {
		prefix = paramRoot
	}
	if !strings.HasPrefix(prefix, paramRoot) {
		writeNamePrefixError(w, "path", paramRoot)
		return
	}
	cl := secrets.New(c.client)
	params, err := cl.GetParametersByPath(r.Context(), c.region, prefix, true)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	ssm := make([]ssmParamWire, 0, len(params))
	for _, p := range params {
		ssm = append(ssm, ssmParamWire{Name: p.Name, Type: p.Type, Value: awsRedacted})
	}
	sm, err := cl.ListSecrets(r.Context(), c.region)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type smWire struct {
		Name  string `json:"name"`
		ARN   string `json:"arn"`
		Value string `json:"value"`
	}
	managed := make([]smWire, 0, len(sm))
	for _, s := range sm {
		if !strings.HasPrefix(s.Name, np+"/") {
			continue
		}
		managed = append(managed, smWire{Name: s.Name, ARN: s.ARN, Value: awsRedacted})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "name_prefix": np, "ssm_parameters": ssm, "secrets_manager": managed})
}

type secretPutBody struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Region    string `json:"region"`
	Overwrite bool   `json:"overwrite"`
}

func (rt *Router) handleAWSSecretsPut(w http.ResponseWriter, r *http.Request) {
	var body secretPutBody
	if err := decodeJSONBody(r, &body); err != nil || !strings.HasPrefix(body.Name, "/") || body.Value == "" {
		writeError(w, http.StatusBadRequest, "a parameter name starting with / and a value are required")
		return
	}
	if root := "/" + rt.awsNamePrefix() + "/"; !strings.HasPrefix(body.Name, root) {
		writeNamePrefixError(w, "name", root)
		return
	}
	c, ok := rt.awsCallFor(w, r, "secrets", body.Region)
	if !ok {
		return
	}
	ver, err := secrets.New(c.client).PutParameter(r.Context(), c.region, body.Name, awsx.Secret(body.Value), body.Overwrite)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.secrets.put", map[string]any{"account": c.acct.ID, "region": c.region, "name": body.Name, "version": ver})
	writeJSON(w, http.StatusOK, map[string]any{"name": body.Name, "version": ver})
}

type secretRevealBody struct {
	Kind   string `json:"kind"`
	Name   string `json:"name"`
	Region string `json:"region"`
}

// handleAWSSecretsReveal is the only route that returns a secret value; it is audited by name and never logs the value.
func (rt *Router) handleAWSSecretsReveal(w http.ResponseWriter, r *http.Request) {
	var body secretRevealBody
	if err := decodeJSONBody(r, &body); err != nil || body.Name == "" || (body.Kind != "ssm" && body.Kind != "secretsmanager") {
		writeError(w, http.StatusBadRequest, "kind (ssm or secretsmanager) and name are required")
		return
	}
	np := rt.awsNamePrefix()
	if root := np + "/"; body.Kind == "secretsmanager" && !strings.HasPrefix(body.Name, root) {
		writeNamePrefixError(w, "name", root)
		return
	} else if root = "/" + root; body.Kind == "ssm" && !strings.HasPrefix(body.Name, root) {
		writeNamePrefixError(w, "name", root)
		return
	}
	c, ok := rt.awsCallFor(w, r, "secrets", body.Region)
	if !ok {
		return
	}
	cl := secrets.New(c.client)
	var value awsx.Secret
	var err error
	if body.Kind == "secretsmanager" {
		value, err = cl.GetSecretValue(r.Context(), c.region, body.Name)
	} else {
		var params []secrets.Parameter
		params, err = cl.GetParametersByPath(r.Context(), c.region, path.Dir(body.Name), false)
		found := false
		for _, p := range params {
			if p.Name == body.Name {
				value, found = p.Value, true
			}
		}
		if err == nil && !found {
			writeError(w, http.StatusNotFound, "not found in AWS")
			return
		}
	}
	if err != nil {
		writeAWSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.secrets.reveal", map[string]any{"account": c.acct.ID, "region": c.region, "kind": body.Kind, "name": body.Name})
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, http.StatusOK, map[string]string{"name": body.Name, "value": value.Reveal()})
}

func (rt *Router) handleAWSAudit(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	resource := q.Get("resource")
	if resource == "" {
		writeError(w, http.StatusBadRequest, "resource is required")
		return
	}
	c, ok := rt.awsCallFor(w, r, "cloudtrail", q.Get("region"))
	if !ok {
		return
	}
	events, err := cloudtrail.New(c.client).LookupEvents(r.Context(), cloudtrail.Query{Region: c.region, AttrKey: "ResourceName", AttrValue: resource, MaxPages: rt.cloudSpendConfig().MaxPages})
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type eventWire struct {
		ID        string    `json:"id"`
		Name      string    `json:"name"`
		Source    string    `json:"source"`
		Username  string    `json:"username"`
		Time      time.Time `json:"time"`
		Resources []string  `json:"resources"`
	}
	out := make([]eventWire, 0, len(events))
	for _, e := range events {
		res := e.Resources
		if res == nil {
			res = []string{}
		}
		out = append(out, eventWire{ID: e.ID, Name: e.Name, Source: e.Source, Username: e.Username, Time: e.Time, Resources: res})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "resource": resource, "events": out})
}

func (rt *Router) handleAWSGuardStatus(w http.ResponseWriter, r *http.Request) {
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	cfg := rt.cloudGuardConfig()
	limit := cfg.MonthlyCapUSD
	if v, has := cfg.AccountCaps[a.ID]; has {
		limit = v
	}
	status := map[string]any{
		"kill_switch": cfg.KillSwitch, "monthly_cap_usd": limit, "per_create_max_usd": cfg.PerCreateMaxUSD,
		"instance_types": nonNilStrings(cfg.InstanceTypes), "default_expires_hours": int(cfg.DefaultTTL / time.Hour),
		"services": cfg.ServicesEnabled, "mtd_spend_usd": nil, "spend_state": "unavailable",
	}
	if a.Provider == "aws" {
		if rd, err := rt.cloudSpendReader(r.Context(), a); err == nil {
			src := cloudguard.CacheSpend{Cache: rt.cloudSpendCache(), Fetch: func(string) cloudspend.Fetch { return rt.spendFetch(a, rd) }}
			if v, serr := src.MonthToDateUSD(r.Context(), a.ID); serr == nil {
				status["mtd_spend_usd"], status["spend_state"] = v, "ok"
			}
		}
	}
	writeJSON(w, http.StatusOK, status)
}

type killSwitchBody struct {
	Enabled bool `json:"enabled"`
}

func (rt *Router) handleAWSKillSwitch(w http.ResponseWriter, r *http.Request) {
	var body killSwitchBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	a, ok := rt.loadCloudAccount(w, r)
	if !ok {
		return
	}
	if rt.runtimeSettings == nil {
		writeError(w, http.StatusNotImplemented, "runtime settings are not available")
		return
	}
	raw, _ := json.Marshal(body.Enabled)
	if _, err := rt.runtimeSettings.Set(r.Context(), runtimesettings.KeyCloudGuardKillSwitch, raw, rt.eventActor(r)); err != nil {
		rt.internalError(w, "save cloud kill switch", err)
		return
	}
	rt.auditCloud(r, "cloud.aws.guard.kill_switch", map[string]any{"account": a.ID, "enabled": body.Enabled})
	writeJSON(w, http.StatusOK, map[string]bool{"kill_switch": body.Enabled})
}

// handleAWSCleanupPlan is a dry run only: there is no delete route for what it lists.
func (rt *Router) handleAWSCleanupPlan(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, "", r.URL.Query().Get("region"))
	if !ok {
		return
	}
	pages := rt.cloudSpendConfig().MaxPages
	plan, err := cloudguard.PlanExpired(r.Context(), c.client, rt.cloudGuardConfig(), c.region, pages)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	orphans, err := cloudguard.FindOrphans(r.Context(), c.client, c.region, pages)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	restop := []map[string]any{}
	if rt.awsServiceOn(rds.ServiceName) {
		age := time.Duration(envIntDefault("APP_CLOUD_RDS_RESTOP_AFTER_HOURS", 168)) * time.Hour
		if items, rerr := rds.New(c.client, c.guard).PlanRestop(r.Context(), c.region, age); rerr == nil {
			for _, it := range items {
				restop = append(restop, map[string]any{"id": it.ID, "stopped_at": it.StoppedAt})
			}
		}
	}
	snapExpired, idle := []map[string]any{}, []idleWire{}
	if rt.awsServiceOn("ebs") {
		snapExpired = rt.snapshotExpiredPlan(r.Context(), c)
	}
	if rt.awsServiceOn("cloudwatch") {
		if items, ierr := rt.idleSuggestions(r.Context(), c); ierr == nil {
			idle = items
		}
	}
	type expiredWire struct {
		ARN       string    `json:"arn"`
		ExpiredAt time.Time `json:"expired_at"`
	}
	type orphanWire struct {
		ID     string `json:"id"`
		Detail string `json:"detail"`
	}
	exp := make([]expiredWire, 0, len(plan.Items))
	for _, it := range plan.Items {
		exp = append(exp, expiredWire{ARN: it.ARN, ExpiredAt: it.ExpiredAt})
	}
	conv := func(in []cloudguard.Orphan) []orphanWire {
		out := make([]orphanWire, 0, len(in))
		for _, o := range in {
			out = append(out, orphanWire{ID: o.ID, Detail: o.Detail})
		}
		return out
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"dry_run": true, "region": c.region, "expired": exp, "rds_restop": restop, "snapshot_expired": snapExpired, "idle": idle, "unparsed": nonNilStrings(plan.Unparsed),
		"orphans": map[string]any{"volumes": conv(orphans.Volumes), "addresses": conv(orphans.Addresses), "snapshots": conv(orphans.Snapshots)},
	})
}
