package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/internal/provision"
	"github.com/GLINCKER/levelrail/internal/store"
)

// nodeProviderCustom is the provider id for bare metal, on-prem or any VM
// with no cloud API: connecting it records metadata only.
const nodeProviderCustom = "custom"

// Connection methods a provider can be connected with.
const (
	methodAPIToken                   = "api_token"
	methodAccessKey                  = "access_key"
	methodAssumeRole                 = "assume_role"
	methodAmbient                    = "ambient"
	methodWorkloadIdentityFederation = "workload_identity_federation"
	methodServiceAccountJSON         = "service_account_json"
	methodManual                     = "manual"
	nodeProviderTestTimeout          = 20 * time.Second
	nodeProviderStatusConnected      = "connected"
	nodeProviderStatusNotConnected   = "not_connected"
	nodeProviderStatusTestFailed     = "test_failed"
	nodeProviderMaxMetaLen           = 500
)

// nodeProviderMethods lists only the methods the backend can use today.
// Azure managed identity and GCP external account JSON are not wired in
// the provisioners, so they are not offered.
var nodeProviderMethods = map[string][]string{
	"hetzner":          {methodAPIToken},
	"digitalocean":     {methodAPIToken},
	"aws":              {methodAccessKey, methodAssumeRole, methodAmbient},
	"azure":            {methodServiceAccountJSON, methodWorkloadIdentityFederation},
	"gcp":              {methodServiceAccountJSON},
	nodeProviderCustom: {methodManual},
}

// nodeProviderTestPermissions is the plain-language least-privilege hint
// shown when a test is refused by the provider.
var nodeProviderTestPermissions = map[string]string{
	"hetzner":      "a Read & Write project API token (Hetzner has no finer scopes)",
	"digitalocean": "a token with the droplet read and create scopes, plus ssh_key read and create",
	"aws":          "ec2:DescribeRegions, ec2:DescribeInstanceTypes, ec2:DescribeInstances, ec2:RunInstances, ec2:TerminateInstances, ec2:CreateTags, ec2:DescribeImages and the security group and VPC actions; for assume_role the base identity needs sts:AssumeRole on the role",
	"azure":        "Reader on the subscription plus Virtual Machine Contributor and Network Contributor on the resource group",
	"gcp":          "compute.regions.list, compute.machineTypes.list, compute.instances.create, compute.instances.get, compute.instances.delete",
}

func methodAllowed(provider, method string) bool {
	for _, m := range nodeProviderMethods[provider] {
		if m == method {
			return true
		}
	}
	return false
}

// nodeProviderResource is the wire shape for GET /api/v1/node-providers.
// No credential value is ever part of it.
type nodeProviderResource struct {
	Provider       string     `json:"provider"`
	HasToken       bool       `json:"has_token"`
	Status         string     `json:"status"`
	Methods        []string   `json:"methods"`
	Method         string     `json:"method,omitempty"`
	Label          string     `json:"label,omitempty"`
	Location       string     `json:"location,omitempty"`
	Notes          string     `json:"notes,omitempty"`
	ConnectedBy    string     `json:"connected_by,omitempty"`
	ConnectedAt    *time.Time `json:"connected_at,omitempty"`
	LastTestedAt   *time.Time `json:"last_tested_at,omitempty"`
	LastTestOK     *bool      `json:"last_test_ok,omitempty"`
	LastTestDetail string     `json:"last_test_detail,omitempty"`
}

// NodeProviderConnectionStore is the store surface for connection metadata.
type NodeProviderConnectionStore interface {
	SaveNodeProviderConnection(ctx context.Context, c store.NodeProviderConnection) error
	RecordNodeProviderTest(ctx context.Context, provider string, ok bool, detail string, at time.Time) error
	DeleteNodeProviderConnection(ctx context.Context, provider string) error
	ListNodeProviderConnections(ctx context.Context) ([]store.NodeProviderConnection, error)
}

func (rt *Router) nodeProviderResources(ctx context.Context) ([]nodeProviderResource, error) {
	conns := map[string]store.NodeProviderConnection{}
	if rt.nodeProviderConns != nil {
		list, err := rt.nodeProviderConns.ListNodeProviderConnections(ctx)
		if err != nil {
			return nil, fmt.Errorf("list node provider connections: %w", err)
		}
		for _, c := range list {
			conns[c.Provider] = c
		}
	}
	names := append(append([]string{}, nodeProviderNames...), nodeProviderCustom)
	out := make([]nodeProviderResource, 0, len(names))
	for _, p := range names {
		res := nodeProviderResource{Provider: p, Methods: nodeProviderMethods[p], Status: nodeProviderStatusNotConnected}
		c, hasRow := conns[p]
		if p == nodeProviderCustom {
			res.HasToken = hasRow
		} else if rt.nodeProviderSecrets != nil {
			exists, err := rt.nodeProviderSecrets.Exists(ctx, store.NodeProviderSecretsKey(p), store.NodeProviderTokenEnvKey)
			if err != nil {
				return nil, fmt.Errorf("check node provider token: %w", err)
			}
			res.HasToken = exists
		}
		if res.HasToken {
			res.Status = nodeProviderStatusConnected
		}
		if hasRow && res.HasToken {
			connectedAt := c.ConnectedAt
			res.Method, res.Label, res.Location, res.Notes = c.Method, c.Label, c.Location, c.Notes
			res.ConnectedBy, res.ConnectedAt = c.ConnectedBy, &connectedAt
			if c.LastTestedAt != nil {
				ok := c.LastTestOK
				res.LastTestedAt, res.LastTestOK, res.LastTestDetail = c.LastTestedAt, &ok, c.LastTestDetail
				if !ok {
					res.Status = nodeProviderStatusTestFailed
				}
			}
		}
		out = append(out, res)
	}
	return out, nil
}

// handleListNodeProviders handles GET /api/v1/node-providers: every
// provider, its supported methods and connection status. No live
// provider calls.
func (rt *Router) handleListNodeProviders(w http.ResponseWriter, r *http.Request) {
	out, err := rt.nodeProviderResources(r.Context())
	if err != nil {
		rt.internalError(w, "api: list node providers failed", err)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// setNodeProviderCredentialRequest is the body of POST
// /api/v1/node-providers and POST /api/v1/node-providers/{provider}/test.
// Token carries the API token, the AWS access key id, or the Azure and
// GCP credential JSON, depending on provider and method.
type setNodeProviderCredentialRequest struct {
	Provider string `json:"provider"`
	Method   string `json:"method,omitempty"`
	Token    string `json:"token"`
	// Verify runs the read-only connection test first and refuses to
	// store anything when it fails.
	Verify                bool   `json:"verify,omitempty"`
	SecretAccessKey       string `json:"secret_access_key,omitempty"`
	SessionToken          string `json:"session_token,omitempty"`
	Region                string `json:"region,omitempty"`
	RoleARN               string `json:"role_arn,omitempty"`
	UseAmbientCredentials bool   `json:"use_ambient_credentials,omitempty"`
	Label                 string `json:"label,omitempty"`
	Location              string `json:"location,omitempty"`
	Notes                 string `json:"notes,omitempty"`
}

// inferNodeProviderMethod keeps older clients that send no method working.
func inferNodeProviderMethod(req setNodeProviderCredentialRequest) string {
	switch req.Provider {
	case "aws":
		switch {
		case req.RoleARN != "":
			return methodAssumeRole
		case req.UseAmbientCredentials:
			return methodAmbient
		default:
			return methodAccessKey
		}
	case "azure":
		var cred struct {
			ClientSecret       string `json:"client_secret"`
			FederatedTokenFile string `json:"federated_token_file"`
		}
		if json.Unmarshal([]byte(req.Token), &cred) == nil && cred.ClientSecret == "" && cred.FederatedTokenFile != "" {
			return methodWorkloadIdentityFederation
		}
		return methodServiceAccountJSON
	case "gcp":
		return methodServiceAccountJSON
	case nodeProviderCustom:
		return methodManual
	default:
		return methodAPIToken
	}
}

// nodeProviderCredentialValue validates req against its method and
// returns the string stored under store.NodeProviderTokenEnvKey. msg is
// non-empty (and value unused) when req fails validation.
func nodeProviderCredentialValue(req setNodeProviderCredentialRequest) (value, msg string) {
	switch req.Method {
	case methodAPIToken:
		if req.Token == "" {
			return "", "token is required"
		}
		return req.Token, ""
	case methodAccessKey, methodAssumeRole, methodAmbient:
		return awsCredentialValue(req)
	case methodServiceAccountJSON, methodWorkloadIdentityFederation:
		return jsonCredentialValue(req)
	default:
		return "", "unsupported method"
	}
}

func awsCredentialValue(req setNodeProviderCredentialRequest) (value, msg string) {
	creds := provision.AWSCredentials{
		AccessKeyID: req.Token, SecretAccessKey: req.SecretAccessKey, SessionToken: req.SessionToken,
		Region: req.Region, RoleARN: req.RoleARN, UseAmbientCredentials: req.UseAmbientCredentials,
	}
	hasKey := creds.AccessKeyID != "" && creds.SecretAccessKey != ""
	switch req.Method {
	case methodAmbient:
		creds.UseAmbientCredentials, creds.AccessKeyID, creds.SecretAccessKey, creds.SessionToken, creds.RoleARN = true, "", "", "", ""
	case methodAccessKey:
		creds.RoleARN, creds.UseAmbientCredentials = "", false
		if !hasKey {
			return "", "an access key id (token) and secret_access_key are required"
		}
	case methodAssumeRole:
		if creds.RoleARN == "" {
			return "", "role_arn is required"
		}
		if !hasKey && !creds.UseAmbientCredentials {
			return "", "assume_role needs base credentials: an access key id and secret_access_key, or use_ambient_credentials"
		}
	}
	encoded, err := json.Marshal(creds) //nolint:gosec // the encoded json is passed to nodeProviderSecrets.SetValue, which encrypts it at rest; it is never logged
	if err != nil {
		return "", "internal error"
	}
	return string(encoded), ""
}

func jsonCredentialValue(req setNodeProviderCredentialRequest) (value, msg string) {
	if req.Token == "" {
		return "", "token (the credential JSON) is required"
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(req.Token), &obj); err != nil {
		return "", "the credential must be a JSON object"
	}
	if req.Provider == "azure" {
		secret, _ := obj["client_secret"].(string)
		file, _ := obj["federated_token_file"].(string)
		if req.Method == methodWorkloadIdentityFederation && (file == "" || secret != "") {
			return "", "federated credentials need federated_token_file and no client_secret"
		}
		if req.Method == methodServiceAccountJSON && secret == "" {
			return "", "a service principal credential needs client_secret"
		}
	}
	return req.Token, ""
}

// nodeProviderTestResult is the wire shape of a connection test.
type nodeProviderTestResult struct {
	OK       bool      `json:"ok"`
	Message  string    `json:"message"`
	TestedAt time.Time `json:"tested_at"`
}

// testNodeProviderValue runs one read-only call (list regions) against the
// provider with the candidate credential value and describes the outcome
// in plain language. The credential is never part of the message.
func (rt *Router) testNodeProviderValue(ctx context.Context, provider, value string) (ok bool, message string) {
	factory := rt.nodeProvisionerFactory
	if factory == nil {
		factory = rt.defaultNodeProvisionerFactory
	}
	p, err := factory(provider, value)
	if err != nil {
		return false, "The credential could not be read: " + sanitizeProviderErr(err)
	}
	ctx, cancel := context.WithTimeout(ctx, nodeProviderTestTimeout)
	defer cancel()
	if _, err := p.ListRegions(ctx); err != nil {
		var pe *provision.ProviderError
		if errors.As(err, &pe) && (pe.Status == http.StatusUnauthorized || pe.Status == http.StatusForbidden) {
			return false, "The provider refused the credential (" + http.StatusText(pe.Status) + "). It needs: " + nodeProviderTestPermissions[provider] + "."
		}
		return false, "Could not complete a read-only call to the provider: " + sanitizeProviderErr(err)
	}
	return true, "Connected: the provider accepted the credential for a read-only call."
}

// sanitizeProviderErr trims an upstream error to a short single line.
func sanitizeProviderErr(err error) string {
	s := strings.Join(strings.Fields(err.Error()), " ")
	if len(s) > 200 {
		s = s[:200]
	}
	return s
}

func (rt *Router) nodeProviderActor(r *http.Request) string {
	_, _, name, ok := rt.currentActor(r)
	if !ok {
		return ""
	}
	return name
}

// decodeNodeProviderRequest reads and normalizes a connect or test body.
func decodeNodeProviderRequest(r *http.Request, provider string) (setNodeProviderCredentialRequest, string) {
	var req setNodeProviderCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, "invalid request body"
	}
	if provider != "" {
		req.Provider = provider
	}
	if req.Provider != nodeProviderCustom && !isKnownNodeProvider(req.Provider) {
		return req, unknownNodeProviderMessage() + ", " + nodeProviderCustom
	}
	if req.Method == "" {
		req.Method = inferNodeProviderMethod(req)
	}
	if !methodAllowed(req.Provider, req.Method) {
		return req, "method " + req.Method + " is not supported for " + req.Provider + ", want one of " + strings.Join(nodeProviderMethods[req.Provider], ", ")
	}
	if len(req.Label) > nodeProviderMaxMetaLen || len(req.Location) > nodeProviderMaxMetaLen || len(req.Notes) > nodeProviderMaxMetaLen {
		return req, "label, location and notes are limited to 500 characters"
	}
	return req, ""
}

// handleSetNodeProviderCredential handles POST /api/v1/node-providers:
// connects (or replaces) one provider. With verify=true the read-only
// test runs first and nothing is stored when it fails.
func (rt *Router) handleSetNodeProviderCredential(w http.ResponseWriter, r *http.Request) {
	req, msg := decodeNodeProviderRequest(r, "")
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.Provider == nodeProviderCustom {
		rt.connectCustomProvider(w, r, req)
		return
	}
	if rt.nodeProviderSecrets == nil {
		writeError(w, http.StatusNotImplemented, "node provisioning is not configured on this control plane (no master key set)")
		return
	}
	value, msg := nodeProviderCredentialValue(req)
	if msg != "" {
		writeError(w, http.StatusBadRequest, msg)
		return
	}
	if req.Verify {
		if ok, detail := rt.testNodeProviderValue(r.Context(), req.Provider, value); !ok {
			writeError(w, http.StatusUnprocessableEntity, detail)
			return
		}
	}
	if err := rt.nodeProviderSecrets.SetValue(r.Context(), store.NodeProviderSecretsKey(req.Provider), store.NodeProviderTokenEnvKey, value); err != nil {
		rt.internalError(w, "api: save node provider token failed", err, slog.String("provider", req.Provider))
		return
	}
	rt.nodeProviderCatalog.invalidate(req.Provider)
	rt.saveNodeProviderConnection(r, req)
	writeJSON(w, http.StatusOK, rt.nodeProviderResourceFor(r.Context(), req.Provider))
}

func (rt *Router) connectCustomProvider(w http.ResponseWriter, r *http.Request, req setNodeProviderCredentialRequest) {
	if strings.TrimSpace(req.Label) == "" {
		writeError(w, http.StatusBadRequest, "label is required: a name for this data center")
		return
	}
	if rt.nodeProviderConns == nil {
		writeError(w, http.StatusNotImplemented, "provider connections are not available")
		return
	}
	rt.saveNodeProviderConnection(r, req)
	writeJSON(w, http.StatusOK, rt.nodeProviderResourceFor(r.Context(), req.Provider))
}

func (rt *Router) saveNodeProviderConnection(r *http.Request, req setNodeProviderCredentialRequest) {
	if rt.nodeProviderConns == nil {
		return
	}
	err := rt.nodeProviderConns.SaveNodeProviderConnection(r.Context(), store.NodeProviderConnection{
		Provider: req.Provider, Method: req.Method, Label: strings.TrimSpace(req.Label),
		Location: strings.TrimSpace(req.Location), Notes: strings.TrimSpace(req.Notes),
		ConnectedBy: rt.nodeProviderActor(r), ConnectedAt: time.Now().UTC(),
	})
	if err != nil {
		rt.logger.Warn("api: save node provider connection failed", slog.String("provider", req.Provider), slog.String("error", err.Error()))
	}
}

func (rt *Router) nodeProviderResourceFor(ctx context.Context, provider string) nodeProviderResource {
	all, err := rt.nodeProviderResources(ctx)
	if err == nil {
		for _, p := range all {
			if p.Provider == provider {
				return p
			}
		}
	}
	return nodeProviderResource{Provider: provider, HasToken: true, Status: nodeProviderStatusConnected, Methods: nodeProviderMethods[provider]}
}

// handleTestNodeProvider handles POST /api/v1/node-providers/{provider}/test.
// A body with credentials tests that candidate and stores nothing; an
// empty body tests the stored credential and records the result.
func (rt *Router) handleTestNodeProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider == nodeProviderCustom {
		writeError(w, http.StatusBadRequest, "a manual data center has no cloud API to test")
		return
	}
	if !isKnownNodeProvider(provider) {
		writeError(w, http.StatusNotFound, "unknown provider")
		return
	}
	if rt.nodeProviderSecrets == nil {
		writeError(w, http.StatusNotImplemented, "node provisioning is not configured on this control plane (no master key set)")
		return
	}
	var value string
	candidate := r.ContentLength != 0
	if candidate {
		req, msg := decodeNodeProviderRequest(r, provider)
		if msg != "" {
			writeError(w, http.StatusBadRequest, msg)
			return
		}
		var vmsg string
		if value, vmsg = nodeProviderCredentialValue(req); vmsg != "" {
			writeError(w, http.StatusBadRequest, vmsg)
			return
		}
	} else {
		stored, err := rt.nodeProviderSecrets.Resolve(r.Context(), store.NodeProviderSecretsKey(provider), store.NodeProviderTokenEnvKey)
		if err != nil {
			writeError(w, http.StatusBadRequest, "no credential is stored for this provider yet")
			return
		}
		value = stored
	}
	ok, message := rt.testNodeProviderValue(r.Context(), provider, value)
	now := time.Now().UTC()
	if !candidate && rt.nodeProviderConns != nil {
		if err := rt.nodeProviderConns.RecordNodeProviderTest(r.Context(), provider, ok, message, now); err != nil && !errors.Is(err, store.ErrNodeProviderConnectionNotFound) {
			rt.logger.Warn("api: record node provider test failed", slog.String("provider", provider), slog.String("error", err.Error()))
		}
	}
	writeJSON(w, http.StatusOK, nodeProviderTestResult{OK: ok, Message: message, TestedAt: now})
}

// handleDisconnectNodeProvider handles DELETE /api/v1/node-providers/{provider}.
func (rt *Router) handleDisconnectNodeProvider(w http.ResponseWriter, r *http.Request) {
	provider := r.PathValue("provider")
	if provider != nodeProviderCustom && !isKnownNodeProvider(provider) {
		writeError(w, http.StatusNotFound, "unknown provider")
		return
	}
	if provider != nodeProviderCustom && rt.nodeProviderSecrets != nil {
		deleter, ok := rt.nodeProviderSecrets.(interface {
			DeleteAll(ctx context.Context, serviceName string) error
		})
		if !ok {
			writeError(w, http.StatusNotImplemented, "disconnecting is not supported by this secrets backend")
			return
		}
		if err := deleter.DeleteAll(r.Context(), store.NodeProviderSecretsKey(provider)); err != nil {
			rt.internalError(w, "api: delete node provider token failed", err, slog.String("provider", provider))
			return
		}
	}
	if rt.nodeProviderConns != nil {
		if err := rt.nodeProviderConns.DeleteNodeProviderConnection(r.Context(), provider); err != nil {
			rt.internalError(w, "api: delete node provider connection failed", err, slog.String("provider", provider))
			return
		}
	}
	rt.nodeProviderCatalog.invalidate(provider)
	w.WriteHeader(http.StatusNoContent)
}
