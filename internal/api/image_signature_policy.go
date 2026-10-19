package api

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"net/http"
)

const (
	sigIssueKeyUnreadable   = "key_unreadable"
	sigIssueEnforceNoKey    = "enforce_without_key"
	sigIssueKeyButNotForced = "key_without_enforcement"
	sigIssueSampleFailed    = "sample_failed"
)

// WithImageVerifyIssue records a signature policy that could not be applied at boot, so Validate can say why.
func WithImageVerifyIssue(issue, requestedMode string) Option {
	return func(rt *Router) {
		rt.imageVerifyIssue = issue
		rt.imageVerifyRequested = requestedMode
	}
}

type imageSignatureIssue struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type imageSignaturePolicyRequest struct {
	Ref string `json:"ref,omitempty"`
}

type imageSignaturePolicyResponse struct {
	Mode           string                `json:"mode"`
	RequestedMode  string                `json:"requested_mode"`
	KeyConfigured  bool                  `json:"key_configured"`
	KeyFingerprint string                `json:"key_fingerprint,omitempty"`
	OK             bool                  `json:"ok"`
	Issues         []imageSignatureIssue `json:"issues"`
	Sample         *imageVerifyResponse  `json:"sample,omitempty"`
}

func (rt *Router) imageSignaturePolicy() imageSignaturePolicyResponse {
	requested := rt.imageVerifyMode
	if rt.imageVerifyRequested != "" {
		requested = rt.imageVerifyRequested
	}
	if requested != ImageVerifyEnforce {
		requested = ImageVerifyOff
	}
	out := imageSignaturePolicyResponse{
		Mode:          rt.imageSignatureMode(),
		RequestedMode: requested,
		KeyConfigured: rt.imageVerifyKey != nil,
		Issues:        []imageSignatureIssue{},
	}
	if rt.imageVerifyKey != nil {
		if der, err := x509.MarshalPKIXPublicKey(rt.imageVerifyKey); err == nil {
			sum := sha256.Sum256(der)
			out.KeyFingerprint = hex.EncodeToString(sum[:8])
		}
	}
	if rt.imageVerifyIssue != "" {
		out.Issues = append(out.Issues, imageSignatureIssue{Code: sigIssueKeyUnreadable, Message: "the public key could not be loaded, so signature checks are off"})
	}
	if requested == ImageVerifyEnforce && rt.imageVerifyKey == nil {
		out.Issues = append(out.Issues, imageSignatureIssue{Code: sigIssueEnforceNoKey, Message: "enforcement was requested but no usable public key is configured, so nothing is being enforced"})
	}
	if rt.imageVerifyKey != nil && requested != ImageVerifyEnforce {
		out.Issues = append(out.Issues, imageSignatureIssue{Code: sigIssueKeyButNotForced, Message: "a key is loaded but enforcement is off: unsigned images can still be promoted"})
	}
	return out
}

// handleValidateImageSignaturePolicy checks the configured signature policy and, with a ref, verifies one sample image.
func (rt *Router) handleValidateImageSignaturePolicy(w http.ResponseWriter, r *http.Request) {
	var req imageSignaturePolicyRequest
	if r.ContentLength != 0 {
		if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&req); err != nil {
			writeError(w, http.StatusBadRequest, "invalid request body")
			return
		}
	}
	out := rt.imageSignaturePolicy()
	if req.Ref != "" && rt.imageVerifyKey != nil && rt.imageVerifier != nil {
		res := rt.verifyImageRef(r.Context(), req.Ref)
		out.Sample = &res
		if res.Status != verifyStatusVerified {
			out.Issues = append(out.Issues, imageSignatureIssue{Code: sigIssueSampleFailed, Message: res.Message})
		}
	}
	out.OK = len(out.Issues) == 0 || (len(out.Issues) == 1 && out.Issues[0].Code == sigIssueKeyButNotForced)
	writeJSON(w, http.StatusOK, out)
}
