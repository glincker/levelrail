package api

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/awsx/cloudfront"
	"github.com/glincker/levelrail/internal/awsx/ssm"
	"github.com/glincker/levelrail/internal/runtimesettings"
)

// Registered here, not in the shared map literal, so this slice does not collide with other services.
func init() {
	awsServices[cloudfront.ServiceName] = awsServiceDef{runtimesettings.KeyCloudGuardCloudFront, "APP_CLOUD_GUARD_CLOUDFRONT"}
	awsServices["ssm"] = awsServiceDef{runtimesettings.KeyCloudGuardSSM, "APP_CLOUD_GUARD_SSM"}
	awsServices["ssm_run"] = awsServiceDef{runtimesettings.KeyCloudGuardSSMRun, "APP_CLOUD_GUARD_SSM_RUN"}
}

func (rt *Router) cfssmInt(key, env string, def int) int {
	if rt.runtimeSettings != nil {
		return rt.runtimeSettings.Int(context.Background(), key)
	}
	return envIntDefault(env, def)
}

type cfDistWire struct {
	ID      string   `json:"id"`
	ARN     string   `json:"arn"`
	Status  string   `json:"status"`
	Domain  string   `json:"domain"`
	Comment string   `json:"comment"`
	Enabled bool     `json:"enabled"`
	Aliases []string `json:"aliases"`
}

func toCFDistWire(d cloudfront.Distribution) cfDistWire {
	al := d.Aliases
	if al == nil {
		al = []string{}
	}
	return cfDistWire{ID: d.ID, ARN: d.ARN, Status: d.Status, Domain: d.Domain, Comment: d.Comment, Enabled: d.Enabled, Aliases: al}
}

type cfInvWire struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

func (rt *Router) handleAWSCFList(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, cloudfront.ServiceName, "us-east-1")
	if !ok {
		return
	}
	page, err := cloudfront.New(c.client).ListDistributions(r.Context(), r.URL.Query().Get("marker"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	out := make([]cfDistWire, 0, len(page.Items))
	for _, d := range page.Items {
		out = append(out, toCFDistWire(d))
	}
	writeJSON(w, http.StatusOK, map[string]any{"distributions": out, "next": page.Next})
}

func (rt *Router) handleAWSCFGet(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, cloudfront.ServiceName, "us-east-1")
	if !ok {
		return
	}
	d, err := cloudfront.New(c.client).GetDistribution(r.Context(), r.PathValue("did"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, toCFDistWire(d))
}

func (rt *Router) handleAWSCFInvalidations(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, cloudfront.ServiceName, "us-east-1")
	if !ok {
		return
	}
	list, next, err := cloudfront.New(c.client).ListInvalidations(r.Context(), r.PathValue("did"), r.URL.Query().Get("marker"))
	if err != nil {
		writeAWSError(w, err)
		return
	}
	out := make([]cfInvWire, 0, len(list))
	for _, i := range list {
		out = append(out, cfInvWire{ID: i.ID, Status: i.Status, CreatedAt: i.CreatedAt})
	}
	writeJSON(w, http.StatusOK, map[string]any{"invalidations": out, "next": next})
}

type cfInvalidateBody struct {
	Paths []string `json:"paths"`
}

func (rt *Router) handleAWSCFInvalidate(w http.ResponseWriter, r *http.Request) {
	var body cfInvalidateBody
	if err := decodeJSONBody(r, &body); err != nil {
		writeError(w, http.StatusBadRequest, "a JSON body with paths is required")
		return
	}
	paths, err := cloudfront.CleanPaths(body.Paths, rt.cfssmInt(runtimesettings.KeyCloudFrontMaxPaths, "APP_CLOUD_CLOUDFRONT_MAX_INVALIDATION_PATHS", 50))
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	c, ok := rt.awsCallFor(w, r, cloudfront.ServiceName, "us-east-1")
	if !ok {
		return
	}
	did := r.PathValue("did")
	inv, err := cloudfront.New(c.client).CreateInvalidation(r.Context(), did, paths)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.cloudfront.invalidate", map[string]any{"account": c.acct.ID, "distribution": did, "paths": len(paths), "invalidation": inv.ID})
	writeJSON(w, http.StatusCreated, cfInvWire{ID: inv.ID, Status: inv.Status, CreatedAt: inv.CreatedAt})
}

func (rt *Router) handleAWSSSMInstances(w http.ResponseWriter, r *http.Request) {
	c, ok := rt.awsCallFor(w, r, "ssm", r.URL.Query().Get("region"))
	if !ok {
		return
	}
	id := r.URL.Query().Get("instance")
	if id != "" && !ssm.ValidInstance(id) {
		writeError(w, http.StatusBadRequest, "instance is not a valid instance ID")
		return
	}
	list, err := ssm.New(c.client).DescribeInstances(r.Context(), c.region, id)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	type row struct {
		ID           string    `json:"id"`
		PingStatus   string    `json:"ping_status"`
		Platform     string    `json:"platform"`
		PlatformName string    `json:"platform_name"`
		AgentVersion string    `json:"agent_version"`
		ComputerName string    `json:"computer_name"`
		LastPing     time.Time `json:"last_ping"`
	}
	out := make([]row, 0, len(list))
	for _, i := range list {
		out = append(out, row{i.ID, i.PingStatus, i.Platform, i.PlatformName, i.AgentVersion, i.ComputerName, i.LastPing})
	}
	writeJSON(w, http.StatusOK, map[string]any{"region": c.region, "instances": out})
}

type ssmSendBody struct {
	Region   string `json:"region"`
	Instance string `json:"instance"`
	Command  string `json:"command"`
}

func (rt *Router) handleAWSSSMSend(w http.ResponseWriter, r *http.Request) {
	var body ssmSendBody
	if err := decodeJSONBody(r, &body); err != nil || strings.TrimSpace(body.Command) == "" || !ssm.ValidInstance(body.Instance) {
		writeError(w, http.StatusBadRequest, "a valid instance and a command are required")
		return
	}
	c, ok := rt.awsCallFor(w, r, "ssm_run", body.Region)
	if !ok {
		return
	}
	allow := rt.settingList(runtimesettings.KeyCloudSSMCommands)
	if !ssm.Allowed(allow, body.Command) {
		rt.auditCloud(r, "cloud.aws.ssm.run_refused", map[string]any{"account": c.acct.ID, "region": c.region, "instance": body.Instance})
		writeError(w, http.StatusConflict, "this command is not in the command allowlist")
		return
	}
	timeout := rt.cfssmInt(runtimesettings.KeyCloudSSMTimeoutSeconds, "APP_CLOUD_SSM_COMMAND_TIMEOUT_SECONDS", 60)
	id, err := ssm.New(c.client).SendCommand(r.Context(), c.region, body.Instance, body.Command, allow, timeout)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	rt.auditCloud(r, "cloud.aws.ssm.run", map[string]any{"account": c.acct.ID, "region": c.region, "instance": body.Instance, "command_id": id})
	writeJSON(w, http.StatusAccepted, map[string]any{"command_id": id, "instance": body.Instance})
}

type ssmInvocationBody struct {
	Region   string `json:"region"`
	Instance string `json:"instance"`
}

func (rt *Router) handleAWSSSMInvocation(w http.ResponseWriter, r *http.Request) {
	var body ssmInvocationBody
	if err := decodeJSONBody(r, &body); err != nil || !ssm.ValidInstance(body.Instance) {
		writeError(w, http.StatusBadRequest, "a valid instance is required")
		return
	}
	c, ok := rt.awsCallFor(w, r, "ssm_run", body.Region)
	if !ok {
		return
	}
	inv, err := ssm.New(c.client).GetInvocation(r.Context(), c.region, r.PathValue("cid"), body.Instance)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": inv.Status, "detail": inv.Detail, "exit_code": inv.ExitCode, "stdout": inv.Stdout, "stderr": inv.Stderr})
}

func (rt *Router) handleAWSSSMSession(w http.ResponseWriter, r *http.Request) {
	var body ssmInvocationBody
	if err := decodeJSONBody(r, &body); err != nil || !ssm.ValidInstance(body.Instance) {
		writeError(w, http.StatusBadRequest, "a valid instance is required")
		return
	}
	c, ok := rt.awsCallFor(w, r, "ssm_run", body.Region)
	if !ok {
		return
	}
	s, err := ssm.New(c.client).StartSession(r.Context(), c.region, body.Instance)
	if err != nil {
		writeAWSError(w, err)
		return
	}
	// The token is a secret: shown once to the caller, never written to the audit row.
	rt.auditCloud(r, "cloud.aws.ssm.session", map[string]any{"account": c.acct.ID, "region": c.region, "instance": body.Instance, "session": s.ID})
	writeJSON(w, http.StatusCreated, map[string]any{"session_id": s.ID, "token": s.Token, "stream_url": s.StreamURL, "region": c.region})
}
