package objectstore

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"

	"github.com/aws/smithy-go"
	smithyhttp "github.com/aws/smithy-go/transport/http"

	"github.com/GLINCKER/levelrail/internal/netguard"
)

// Failure reason codes returned by Probe.
const (
	ReasonInvalidCredentials = "invalid_credentials" //nolint:gosec // reason code, not a credential
	ReasonAccessDenied       = "access_denied"
	ReasonBucketNotFound     = "bucket_not_found"
	ReasonRegionMismatch     = "region_mismatch"
	ReasonEndpointBlocked    = "endpoint_blocked"
	ReasonTLS                = "tls_error"
	ReasonUnreachable        = "unreachable"
	ReasonIntegrity          = "integrity_mismatch"
	ReasonUnknown            = "unknown"
)

// ProbeStep is the outcome of one probe operation.
type ProbeStep struct {
	Name  string `json:"name"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// ProbeResult reports a put, get and delete round trip on a throwaway key.
type ProbeResult struct {
	OK      bool        `json:"ok"`
	Reason  string      `json:"reason,omitempty"`
	Message string      `json:"message,omitempty"`
	Steps   []ProbeStep `json:"steps"`
}

// Probe writes, reads back and deletes a random object to prove the
// credentials can do everything log archive needs.
func Probe(ctx context.Context, c *Client) ProbeResult {
	res := ProbeResult{Steps: []ProbeStep{}}
	payload := make([]byte, 16)
	if _, err := rand.Read(payload); err != nil {
		return failed(res, "put", fmt.Errorf("generate probe payload: %w", err))
	}
	key := ".probe/" + hex.EncodeToString(payload)

	if err := c.Put(ctx, key, bytes.NewReader(payload), "application/octet-stream", ""); err != nil {
		return failed(res, "put", err)
	}
	res.Steps = append(res.Steps, ProbeStep{Name: "put", OK: true})

	body, _, err := c.Get(ctx, key)
	if err != nil {
		return failed(res, "get", err)
	}
	got, readErr := io.ReadAll(io.LimitReader(body, 1024))
	_ = body.Close()
	if readErr != nil {
		return failed(res, "get", readErr)
	}
	if !bytes.Equal(got, payload) {
		res.Steps = append(res.Steps, ProbeStep{Name: "get", Error: "object read back did not match what was written"})
		res.Reason, res.Message = ReasonIntegrity, "object read back did not match what was written"
		return res
	}
	res.Steps = append(res.Steps, ProbeStep{Name: "get", OK: true})

	if err := c.Delete(ctx, key); err != nil {
		return failed(res, "delete", err)
	}
	res.Steps = append(res.Steps, ProbeStep{Name: "delete", OK: true})
	res.OK = true
	return res
}

func failed(res ProbeResult, step string, err error) ProbeResult {
	reason, msg := Classify(err)
	res.Steps = append(res.Steps, ProbeStep{Name: step, Error: msg})
	res.Reason, res.Message = reason, msg
	return res
}

// Classify maps an S3 or transport error to a stable reason code and a
// message safe to show (it never contains credentials).
func Classify(err error) (string, string) {
	if err == nil {
		return "", ""
	}
	if errors.Is(err, netguard.ErrBlockedAddress) {
		return ReasonEndpointBlocked, "the endpoint resolves to an internal address, which is blocked (set " + netguard.AllowPrivateEnv + "=true to allow a private endpoint such as a local MinIO)"
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "InvalidAccessKeyId", "SignatureDoesNotMatch", "InvalidToken", "ExpiredToken":
			return ReasonInvalidCredentials, "the storage endpoint rejected the access key id or secret access key"
		case "AccessDenied", "AllAccessDisabled", "Forbidden":
			return ReasonAccessDenied, "the credentials are valid but not allowed to write, read and delete objects in this bucket"
		case "NoSuchBucket":
			return ReasonBucketNotFound, "bucket not found at the configured endpoint: check the bucket name, region and endpoint"
		case "AuthorizationHeaderMalformed", "PermanentRedirect", "IllegalLocationConstraintException", "InvalidRegionName":
			return ReasonRegionMismatch, "the bucket lives in a different region than configured: check the region"
		}
	}
	var respErr *smithyhttp.ResponseError
	if errors.As(err, &respErr) {
		switch respErr.HTTPStatusCode() {
		case http.StatusUnauthorized:
			return ReasonInvalidCredentials, "the storage endpoint rejected the access key id or secret access key"
		case http.StatusForbidden:
			return ReasonAccessDenied, "the storage endpoint refused the request: check the credentials and the bucket policy"
		case http.StatusNotFound:
			return ReasonBucketNotFound, "bucket not found at the configured endpoint: check the bucket name, region and endpoint"
		case http.StatusMovedPermanently, http.StatusTemporaryRedirect:
			return ReasonRegionMismatch, "the bucket lives in a different region than configured: check the region"
		}
	}
	var certErr x509.UnknownAuthorityError
	var hostErr x509.HostnameError
	if errors.As(err, &certErr) || errors.As(err, &hostErr) || strings.Contains(err.Error(), "x509:") || strings.Contains(err.Error(), "tls:") {
		return ReasonTLS, "TLS handshake with the endpoint failed: check that the endpoint uses a valid certificate"
	}
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return ReasonUnreachable, "the endpoint did not respond in time"
	}
	var opErr *net.OpError
	var dnsErr *net.DNSError
	if errors.As(err, &opErr) || errors.As(err, &dnsErr) {
		return ReasonUnreachable, "could not connect to the endpoint: check the endpoint host and port"
	}
	return ReasonUnknown, "storage request failed: " + truncate(err.Error(), 300)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
