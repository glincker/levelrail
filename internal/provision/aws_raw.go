package provision

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
)

const (
	awsRawMaxBody = 1 << 20
	// awsVCPUQuotaCode is the Running On-Demand Standard instances vCPU quota.
	awsVCPUQuotaCode = "L-1216C47A"
	awsDefaultWindow = 7 * 24 * time.Hour
)

// awsRawBase replaces the service endpoint in tests; empty means the public one.
func (a *AWSInstances) awsRawURL(service, region string) string {
	if a.rawBase != "" {
		return a.rawBase
	}
	return "https://" + service + "." + region + ".amazonaws.com/"
}

// awsRawCall signs and sends one JSON 1.1 request for services the EC2 client does not cover.
func (a *AWSInstances) awsRawCall(ctx context.Context, region, service, target string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	p, err := a.creds.CredentialsProvider(ctx, region)
	if err != nil {
		return classify(ErrInstanceForbidden, "credentials", err)
	}
	cr, err := p.Retrieve(ctx)
	if err != nil {
		return classify(ErrInstanceForbidden, "credentials", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, a.awsRawURL(service, region), bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", target)
	sum := sha256.Sum256(body)
	if err := v4.NewSigner().SignHTTP(ctx, cr, req, hex.EncodeToString(sum[:]), service, region, time.Now()); err != nil {
		return classify(ErrInstanceForbidden, "sign", err)
	}
	res, err := (&http.Client{Timeout: 15 * time.Second}).Do(req)
	if err != nil {
		return fmt.Errorf("provision: aws %s call: %w", service, err)
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, awsRawMaxBody))
	if res.StatusCode >= 300 {
		return classifyAWSRaw(res.StatusCode, string(raw))
	}
	if out == nil {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// classifyAWSRaw maps a status and error type to a class; the body text never leaves.
func classifyAWSRaw(status int, body string) error {
	cause := errors.New("provision: aws raw call failed")
	switch {
	case status == 403 || strings.Contains(body, "AccessDenied"):
		return classify(ErrInstanceForbidden, "AccessDenied", cause)
	case status == 429 || strings.Contains(body, "Throttling"):
		return classify(ErrInstanceThrottled, "Throttling", cause)
	case status == 404:
		return classify(ErrInstanceNotFound, "", cause)
	}
	return classify(ErrInstanceInvalid, fmt.Sprint(status), cause)
}

// cloudTrailEvents reads recent control plane events that name the instance.
func (a *AWSInstances) cloudTrailEvents(ctx context.Context, region, iid string, since time.Time) ([]InstanceEvent, error) {
	end := time.Now().UTC()
	if since.IsZero() {
		since = end.Add(-awsDefaultWindow)
	}
	in := map[string]any{
		"LookupAttributes": []map[string]string{{"AttributeKey": "ResourceName", "AttributeValue": iid}},
		"StartTime":        since.Unix(), "EndTime": end.Unix(), "MaxResults": 50,
	}
	var out struct {
		Events []struct {
			EventName string  `json:"EventName"`
			EventTime float64 `json:"EventTime"`
		} `json:"Events"`
	}
	if err := a.awsRawCall(ctx, region, "cloudtrail", "com.amazonaws.cloudtrail.v20131101.CloudTrail_20131101.LookupEvents", in, &out); err != nil {
		return nil, err
	}
	rows := make([]InstanceEvent, 0, len(out.Events))
	for _, e := range out.Events {
		rows = append(rows, InstanceEvent{Time: time.Unix(int64(e.EventTime), 0).UTC(), Kind: e.EventName, Status: "recorded"})
	}
	return rows, nil
}

// ResizeQuota says whether the vCPU quota in the instance's region fits the new type.
// known is false when the quota could not be read, which callers treat as no signal.
func (a *AWSInstances) ResizeQuota(ctx context.Context, id, newType string) (limit, projected float64, known bool, err error) {
	c, region, _, inst, err := a.fetch(ctx, id)
	if err != nil {
		return 0, 0, false, err
	}
	var q struct {
		Quota struct {
			Value float64 `json:"Value"`
		} `json:"Quota"`
	}
	if err := a.awsRawCall(ctx, region, "servicequotas", "ServiceQuotasV1.GetServiceQuota",
		map[string]string{"ServiceCode": "ec2", "QuotaCode": awsVCPUQuotaCode}, &q); err != nil {
		if errors.Is(err, ErrInstanceForbidden) {
			return 0, 0, false, nil
		}
		return 0, 0, false, err
	}
	list, err := a.ListInstances(ctx, InstanceScope{Region: region})
	if err != nil {
		return 0, 0, false, err
	}
	used := 0.0
	for _, i := range list {
		if i.State == InstanceStateRunning || i.State == InstanceStatePending {
			used += i.OCPUs
		}
	}
	cur := a.toInstance(region, inst)
	spec := typeSpecs(ctx, c, []string{cur.Shape, newType})
	curV, newV := 0.0, 0.0
	if t, ok := spec[cur.Shape]; ok && t.VCpuInfo != nil && t.VCpuInfo.DefaultVCpus != nil {
		curV = float64(*t.VCpuInfo.DefaultVCpus)
	}
	t, ok := spec[newType]
	if !ok || t.VCpuInfo == nil || t.VCpuInfo.DefaultVCpus == nil {
		return 0, 0, false, nil
	}
	newV = float64(*t.VCpuInfo.DefaultVCpus)
	return q.Quota.Value, used - curV + newV, true, nil
}
