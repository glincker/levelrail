package cloudconn

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/glincker/levelrail/internal/ocisign"
)

// Oracle capability names, one per read the account needs.
const (
	OracleCapCompute = "compute"
	OracleCapUsage   = "usage"
	OracleCapBudgets = "budgets"
	// OracleCapNetwork is the virtual-network read that resolves instance addresses.
	OracleCapNetwork = "network"
)

// OracleStored is the sealed JSON an Oracle cloud account keeps. Only Go callers see the key.
type OracleStored struct {
	TenancyOCID     string `json:"tenancy_ocid"`
	UserOCID        string `json:"user_ocid"`
	Fingerprint     string `json:"fingerprint"`
	PrivateKeyPEM   string `json:"private_key_pem"`
	Region          string `json:"region"`
	CompartmentOCID string `json:"compartment_ocid"`
	SubnetOCID      string `json:"subnet_ocid"`
	SessionToken    string `json:"session_token,omitempty"`
}

// OracleCredentialFromStored decodes the sealed account value into the credential every
// Oracle call here takes, so an account, a node provider secret and a connection agree.
func OracleCredentialFromStored(raw string) (OracleCredential, error) {
	var s OracleStored
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return OracleCredential{}, invalid("the stored oracle credential is not readable")
	}
	return OracleCredential{
		TenancyOCID: s.TenancyOCID, CompartmentOCID: s.CompartmentOCID, Region: s.Region,
		Secret: NewOracleSecret(s.UserOCID, s.Fingerprint, []byte(s.PrivateKeyPEM)), SessionToken: s.SessionToken,
	}, nil
}

// oracleHints say what to fix per capability; fixed text, never provider output.
var oracleHints = map[string]string{
	OracleCapCompute: "Allow the group to read instance-family in the compartment (manage to start, stop or provision).",
	OracleCapUsage:   "Allow the group to read usage-report in the tenancy.",
	OracleCapBudgets: "Allow the group to read usage-budgets in the tenancy.",
	OracleCapNetwork: "Allow the group to read virtual-network-family in the compartment (instance IP addresses).",
}

// OracleProber runs one read-only call per capability with the account key.
// Bases are per service so tests can point each at a fake.
type OracleProber struct {
	Client                  *http.Client
	Now                     func() time.Time
	IaaS, UsageAPI, Budgets func(region string) string
	Identity                func(region string) string
}

// NewOracleProber uses the public regional endpoints and the env bounded timeout.
func NewOracleProber() OracleProber {
	return OracleProber{
		Client:   &http.Client{Timeout: oracleCheckTimeout()},
		Now:      time.Now,
		IaaS:     func(r string) string { return "https://iaas." + r + ".oraclecloud.com/20160918" },
		UsageAPI: func(r string) string { return "https://usageapi." + r + ".oci.oraclecloud.com/20200107" },
		Budgets:  func(r string) string { return "https://usage." + r + ".oci.oraclecloud.com/20190111" },
		Identity: func(r string) string { return "https://identity." + r + ".oraclecloud.com/20160918" },
	}
}

// do signs and sends one request and returns the status and a bounded body.
func (o OracleProber) do(ctx context.Context, c OracleCredential, method, target string, body []byte) (int, []byte, error) {
	now := time.Now
	if o.Now != nil {
		now = o.Now
	}
	key, keyID, err := oracleKeyAndID(c, now())
	if err != nil {
		return 0, nil, err
	}
	var rd io.Reader
	if body != nil {
		rd = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, rd)
	if err != nil {
		return 0, nil, errors.New("cloudconn: oracle endpoint is not valid")
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	sg := ocisign.Signer{
		KeyID: keyID, Key: key, Now: now,
		AllowInsecure: strings.HasPrefix(target, "http://"),
	}
	if err := sg.Sign(req, body); err != nil {
		return 0, nil, errors.New("cloudconn: oracle request could not be signed")
	}
	client := o.Client
	if client == nil {
		client = &http.Client{Timeout: oracleCheckTimeout()}
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, errors.New("cloudconn: oracle endpoint unreachable")
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 256<<10))
	return resp.StatusCode, b, nil
}

func oracleResult(name string, status int, err error) CapabilityResult {
	r := CapabilityResult{Name: name, Method: "read_call", Verified: err == nil && status != 0}
	switch {
	case err == nil && status == http.StatusOK:
		r.OK = true
	case err == nil && (status == http.StatusUnauthorized || status == http.StatusForbidden || status == http.StatusNotFound):
		r.Hint = oracleHints[name]
		r.Missing = []string{name}
	default:
		r.Verified = false
		r.Hint = "The check could not complete, verify the account and try again."
	}
	return r
}

// ProbeCapabilities reports per capability whether the key can list instances in the
// compartment, read usage and read budgets. Unknown names are an error.
func (o OracleProber) ProbeCapabilities(ctx context.Context, c OracleCredential, names ...string) ([]CapabilityResult, error) {
	if err := ValidateOracle(c); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		names = []string{OracleCapCompute, OracleCapUsage, OracleCapBudgets, OracleCapNetwork}
	}
	out := make([]CapabilityResult, 0, len(names))
	for _, n := range names {
		var status int
		var err error
		switch n {
		case OracleCapCompute:
			status, _, err = o.do(ctx, c, http.MethodGet, o.IaaS(c.Region)+"/instances?limit=1&compartmentId="+url.QueryEscape(c.CompartmentOCID), nil)
		case OracleCapNetwork:
			status, _, err = o.do(ctx, c, http.MethodGet, o.IaaS(c.Region)+"/vnicAttachments?limit=1&compartmentId="+url.QueryEscape(c.CompartmentOCID), nil)
		case OracleCapUsage:
			day := (o.now().UTC().Truncate(24 * time.Hour)).AddDate(0, 0, -2)
			body, _ := json.Marshal(map[string]any{
				"tenantId": c.TenancyOCID, "timeUsageStarted": day.Format(time.RFC3339), "timeUsageEnded": day.AddDate(0, 0, 1).Format(time.RFC3339),
				"granularity": "DAILY", "queryType": "COST",
			})
			status, _, err = o.do(ctx, c, http.MethodPost, o.UsageAPI(c.Region)+"/usage", body)
		case OracleCapBudgets:
			status, _, err = o.do(ctx, c, http.MethodGet, o.Budgets(c.Region)+"/budgets?limit=1&targetType=ALL&compartmentId="+url.QueryEscape(c.TenancyOCID), nil)
		default:
			return nil, invalid("unknown oracle capability")
		}
		out = append(out, oracleResult(n, status, err))
	}
	return out, nil
}

func (o OracleProber) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

// Regions lists the tenancy's ready region subscriptions through the identity service.
func (o OracleProber) Regions(ctx context.Context, c OracleCredential) ([]string, error) {
	if err := ValidateOracle(c); err != nil {
		return nil, err
	}
	status, body, err := o.do(ctx, c, http.MethodGet, o.Identity(c.Region)+"/tenancies/"+url.PathEscape(c.TenancyOCID)+"/regionSubscriptions", nil)
	if err != nil {
		return nil, err
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("cloudconn: oracle region list refused (%d)", status)
	}
	var subs []struct {
		RegionName string `json:"regionName"`
		Status     string `json:"status"`
	}
	if err := json.Unmarshal(body, &subs); err != nil {
		return nil, errors.New("cloudconn: oracle region list was not readable")
	}
	out := make([]string, 0, len(subs))
	for _, s := range subs {
		if s.RegionName != "" && (s.Status == "READY" || s.Status == "") {
			out = append(out, s.RegionName)
		}
	}
	return out, nil
}

// Regions on the checker keeps OracleProvider.ListRegions working from one account.
func (o OracleHTTPChecker) Regions(ctx context.Context, c OracleCredential) ([]string, error) {
	p := NewOracleProber()
	p.Client, p.Now = o.Client, o.Now
	if o.BaseURL != nil {
		base := o.BaseURL(c.Region)
		p.Identity = func(string) string { return base + "/20160918" }
	}
	return p.Regions(ctx, c)
}

// OraclePolicyKitInput names the policy parts; every name comes from the caller.
type OraclePolicyKitInput struct {
	Group       string
	PolicyName  string
	Compartment string
	// Manage lets Levelrail start, stop and provision; the default is read only.
	Manage bool
}

// OraclePolicyKit is the least privilege policy text the user applies themselves.
type OraclePolicyKit struct {
	PolicyName string   `json:"policy_name"`
	Statements []string `json:"statements"`
	PolicyText string   `json:"policy_text"`
	CLI        string   `json:"oci_cli"`
	Notes      []string `json:"notes"`
}

// BuildOraclePolicyKit renders compute in one compartment plus the two tenancy reads
// (usage and budgets). Levelrail never creates IAM, so this is text only.
func BuildOraclePolicyKit(in OraclePolicyKitInput) (OraclePolicyKit, error) {
	if !policyNameRe.MatchString(in.Group) {
		return OraclePolicyKit{}, invalid("group must be a plain OCI group name")
	}
	name := in.PolicyName
	if name == "" {
		name = "levelrail-access"
	}
	if !policyNameRe.MatchString(name) {
		return OraclePolicyKit{}, invalid("policy name must be a plain name")
	}
	scope, err := oracleScope(in.Compartment)
	if err != nil {
		return OraclePolicyKit{}, err
	}
	var st []string
	if in.Manage {
		st = append(st,
			fmt.Sprintf("Allow group %s to manage instance-family in %s", in.Group, scope),
			fmt.Sprintf("Allow group %s to use virtual-network-family in %s", in.Group, scope),
			fmt.Sprintf("Allow group %s to read app-catalog-listing in %s", in.Group, scope))
	} else {
		st = append(st,
			fmt.Sprintf("Allow group %s to read instance-family in %s", in.Group, scope),
			fmt.Sprintf("Allow group %s to read virtual-network-family in %s", in.Group, scope))
	}
	st = append(st,
		fmt.Sprintf("Allow group %s to read usage-report in tenancy", in.Group),
		fmt.Sprintf("Allow group %s to read usage-budgets in tenancy", in.Group))
	enc, _ := json.Marshal(st)
	return OraclePolicyKit{
		PolicyName: name, Statements: st, PolicyText: strings.Join(st, "\n"),
		CLI: "oci iam policy create --compartment-id <TENANCY_OCID> --name " + name +
			" --description 'Levelrail access' --statements '" + string(enc) + "'",
		Notes: []string{
			"Create the policy in the tenancy root: the usage and budgets statements apply to the whole tenancy.",
			"Then upload an API public key to a user in that group and add the account with its private key.",
			"Run the account capability probe afterwards to confirm compute, network, usage and budgets.",
		},
	}, nil
}
