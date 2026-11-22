package cloudconn

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

// simulateRequest asks which actions a principal may perform under the managed tag and region.
type simulateRequest struct {
	PolicySource string
	Actions      []string
	Region       string
	TagKey       string
	// Resources maps an action to example resource ARNs; an action without an entry uses "*".
	Resources map[string][]string
}

// PolicySimulator answers an action to decision map ("allowed", "implicitDeny", "explicitDeny").
type PolicySimulator interface {
	Simulate(ctx context.Context, req simulateRequest) (map[string]string, error)
}

// iamSimulator calls the IAM query API directly with SigV4, which avoids a whole SDK module
// for the one read-only call this package needs.
type iamSimulator struct {
	cfg  aws.Config
	http aws.HTTPClient
}

func newIAMSimulator(cfg aws.Config) PolicySimulator {
	var hc aws.HTTPClient = &http.Client{Timeout: oracleCheckTimeout()}
	if cfg.HTTPClient != nil {
		hc = cfg.HTTPClient
	}
	return iamSimulator{cfg: cfg, http: hc}
}

// iamEndpoint returns the global IAM endpoint and signing region for the partition of region.
func iamEndpoint(region string) (string, string) {
	switch {
	case strings.HasPrefix(region, "cn-"):
		return "https://iam.cn-north-1.amazonaws.com.cn/", "cn-north-1"
	case strings.HasPrefix(region, "us-gov-"):
		return "https://iam.us-gov.amazonaws.com/", "us-gov-west-1"
	}
	return "https://iam.amazonaws.com/", "us-east-1"
}

type simulateResponse struct {
	Results []struct {
		Action   string `xml:"EvalActionName"`
		Decision string `xml:"EvalDecision"`
	} `xml:"SimulatePrincipalPolicyResult>EvaluationResults>member"`
	Truncated bool   `xml:"SimulatePrincipalPolicyResult>IsTruncated"`
	Marker    string `xml:"SimulatePrincipalPolicyResult>Marker"`
}

type iamErrorResponse struct {
	Code string `xml:"Error>Code"`
}

// awsAPIError carries an AWS error code through errors.As without the provider message.
type awsAPIError struct{ code string }

func (e awsAPIError) Error() string     { return "aws api error " + e.code }
func (e awsAPIError) ErrorCode() string { return e.code }

func simulateBatch() int { return envIntDefault("APP_CLOUD_AWS_SIMULATE_BATCH", 50) }

func (s iamSimulator) Simulate(ctx context.Context, req simulateRequest) (map[string]string, error) {
	creds, err := s.cfg.Credentials.Retrieve(ctx)
	if err != nil {
		return nil, awsErr("credentials", err)
	}
	endpoint, signRegion := iamEndpoint(req.Region)
	if s.cfg.BaseEndpoint != nil {
		endpoint = *s.cfg.BaseEndpoint
	}
	out := map[string]string{}
	step := simulateBatch()
	// One call carries one resource list, so actions are grouped by their resource set.
	groups := map[string][]string{}
	var order []string
	for _, a := range req.Actions {
		k := strings.Join(req.Resources[a], "\n")
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], a)
	}
	for _, k := range order {
		acts := groups[k]
		var res []string
		if k != "" {
			res = strings.Split(k, "\n")
		}
		for i := 0; i < len(acts); i += step {
			end := min(i+step, len(acts))
			if err := s.page(ctx, creds, endpoint, signRegion, req, acts[i:end], res, out); err != nil {
				return nil, err
			}
		}
	}
	return out, nil
}

func (s iamSimulator) page(ctx context.Context, creds aws.Credentials, endpoint, signRegion string, req simulateRequest, actions, resources []string, out map[string]string) error {
	marker := ""
	for {
		form := url.Values{"Action": {"SimulatePrincipalPolicy"}, "Version": {"2010-05-08"}, "PolicySourceArn": {req.PolicySource}}
		for i, a := range actions {
			form.Set(fmt.Sprintf("ActionNames.member.%d", i+1), a)
		}
		for i, r := range resources {
			form.Set(fmt.Sprintf("ResourceArns.member.%d", i+1), r)
		}
		ctxEntries := [][2]string{
			{"aws:RequestTag/" + req.TagKey, "true"}, {"aws:ResourceTag/" + req.TagKey, "true"}, {"aws:RequestedRegion", req.Region},
		}
		for i, e := range ctxEntries {
			pre := fmt.Sprintf("ContextEntries.member.%d.", i+1)
			form.Set(pre+"ContextKeyName", e[0])
			form.Set(pre+"ContextKeyType", "string")
			form.Set(pre+"ContextKeyValues.member.1", e[1])
		}
		if marker != "" {
			form.Set("Marker", marker)
		}
		body := form.Encode()
		hreq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
		if err != nil {
			return err
		}
		hreq.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=utf-8")
		sum := sha256.Sum256([]byte(body))
		if err := v4.NewSigner().SignHTTP(ctx, creds, hreq, hex.EncodeToString(sum[:]), "iam", signRegion, time.Now()); err != nil {
			return errors.New("cloudconn: aws iam request could not be signed")
		}
		resp, err := s.http.Do(hreq)
		if err != nil {
			return errors.New("cloudconn: aws iam request failed")
		}
		raw, rerr := io.ReadAll(io.LimitReader(resp.Body, int64(envIntDefault("APP_CLOUD_AWS_MAX_RESPONSE_BYTES", 4<<20))))
		_ = resp.Body.Close()
		if rerr != nil {
			return errors.New("cloudconn: aws iam response could not be read")
		}
		if resp.StatusCode != http.StatusOK {
			var e iamErrorResponse
			_ = xml.NewDecoder(bytes.NewReader(raw)).Decode(&e)
			return fmt.Errorf("cloudconn: aws SimulatePrincipalPolicy failed: %w", awsAPIError{code: e.Code})
		}
		var r simulateResponse
		if err := xml.Unmarshal(raw, &r); err != nil {
			return errors.New("cloudconn: aws iam response is not valid")
		}
		for _, m := range r.Results {
			out[m.Action] = m.Decision
		}
		if !r.Truncated || r.Marker == "" {
			return nil
		}
		marker = r.Marker
	}
}
