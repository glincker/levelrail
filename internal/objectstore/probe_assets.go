package objectstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/GLINCKER/levelrail/kit/netguard"
)

// ExposureResult is what an anonymous HEAD of a throwaway object through the public base URL returned.
type ExposureResult struct {
	URL string `json:"url"`
	// Public is true when the anonymous request got a 2xx, so anyone can read objects through that URL.
	Public bool   `json:"public"`
	Status int    `json:"status,omitempty"`
	Error  string `json:"error,omitempty"`
}

// AssetProbeOptions selects the optional steps of ProbeAssets.
type AssetProbeOptions struct {
	// Origin enables the CORS step.
	Origin    string
	ApplyCORS bool
	// PublicBaseURL enables the anonymous exposure step.
	PublicBaseURL string
	// KeyPrefix is the asset bucket prefix; the exposure object is written under it.
	KeyPrefix string
	// HTTPClient defaults to the SSRF-guarded client.
	HTTPClient *http.Client
	// Timeout bounds the exposure request.
	Timeout time.Duration
}

// AssetProbeResult is the connection probe plus the CORS and exposure steps.
type AssetProbeResult struct {
	ProbeResult
	CORS     *CORSCheck      `json:"cors,omitempty"`
	Exposure *ExposureResult `json:"exposure,omitempty"`
}

// ProbeAssets runs the put/get/delete probe, then the CORS and exposure steps when asked. Credentials never appear in the result.
func ProbeAssets(ctx context.Context, c *Client, opts AssetProbeOptions) AssetProbeResult {
	res := AssetProbeResult{ProbeResult: Probe(ctx, c)}
	if !res.OK {
		return res
	}
	if opts.Origin != "" {
		check := CheckCORS(ctx, c, opts.Origin, opts.ApplyCORS)
		res.CORS = &check
		res.Steps = append(res.Steps, ProbeStep{Name: "cors", OK: check.Status == CORSOK || check.Status == CORSApplied, Error: corsStepError(check)})
	}
	if opts.PublicBaseURL != "" {
		exp := probeExposure(ctx, c, opts)
		res.Exposure = &exp
		res.Steps = append(res.Steps, ProbeStep{Name: "exposure", OK: exp.Error == "", Error: exp.Error})
	}
	return res
}

func corsStepError(c CORSCheck) string {
	if c.Status == CORSOK || c.Status == CORSApplied {
		return ""
	}
	return c.Message
}

func probeExposure(ctx context.Context, c *Client, opts AssetProbeOptions) ExposureResult {
	payload := make([]byte, 16)
	if _, err := rand.Read(payload); err != nil {
		return ExposureResult{Error: "could not generate a probe key"}
	}
	key := opts.KeyPrefix + ".probe/" + hex.EncodeToString(payload)
	res := ExposureResult{URL: PublicObjectURL(opts.PublicBaseURL, key)}
	if err := c.Put(ctx, key, bytes.NewReader(payload), "application/octet-stream", ""); err != nil {
		_, msg := Classify(err)
		res.Error = msg
		return res
	}
	defer func() { _ = c.Delete(context.WithoutCancel(ctx), key) }()
	status, err := AnonymousHead(ctx, opts.HTTPClient, res.URL, opts.Timeout)
	if err != nil {
		res.Error = err.Error()
		return res
	}
	res.Status = status
	res.Public = status >= 200 && status < 300
	return res
}

// PublicObjectURL joins a base URL and an object key, escaping each key segment and never guessing a base.
func PublicObjectURL(base, key string) string {
	segs := strings.Split(strings.TrimPrefix(key, "/"), "/")
	for i, s := range segs {
		segs[i] = url.PathEscape(s)
	}
	return strings.TrimRight(base, "/") + "/" + strings.Join(segs, "/")
}

// AnonymousHead sends an unsigned HEAD and returns the status; it never follows redirects off the host.
func AnonymousHead(ctx context.Context, client *http.Client, target string, timeout time.Duration) (int, error) {
	if client == nil {
		client = netguard.NewClient()
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, target, http.NoBody)
	if err != nil {
		return 0, errors.New("the public base URL is not a valid URL")
	}
	noRedirect := *client
	noRedirect.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	resp, err := noRedirect.Do(req)
	if err != nil {
		reason, msg := Classify(err)
		return 0, fmt.Errorf("could not reach the public base URL (%s): %s", reason, msg)
	}
	_ = resp.Body.Close()
	return resp.StatusCode, nil
}
