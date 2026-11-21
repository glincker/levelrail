// Package ocisign signs HTTP requests for the OCI API with the draft-cavage
// "Signature" scheme, using only the standard library.
package ocisign

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Signer holds one API key. AllowInsecure exists so tests can use a plain httptest server.
type Signer struct {
	KeyID         string
	Key           *rsa.PrivateKey
	Now           func() time.Time
	AllowInsecure bool
}

// KeyID builds the keyId field: tenancy/user/fingerprint.
func KeyID(tenancyOCID, userOCID, fingerprint string) string {
	return tenancyOCID + "/" + userOCID + "/" + fingerprint
}

// SessionKeyID builds the keyId for a session token login: ST$ plus the JWT. The request
// is then signed with the session private key, not an API key.
func SessionKeyID(token string) string {
	return "ST$" + token
}

func hasBody(method string) bool {
	return method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch
}

// Headers returns the signed header list for a method. Body methods add the three
// body headers, in the order OCI documents.
func Headers(method string) []string {
	h := []string{"date", "(request-target)", "host"}
	if hasBody(method) {
		h = append(h, "content-length", "content-type", "x-content-sha256")
	}
	return h
}

// SigningString builds the text the signature covers for req. The body headers must
// already be set on req for POST, PUT and PATCH.
func SigningString(req *http.Request) string {
	target := req.URL.EscapedPath()
	if req.URL.RawQuery != "" {
		target += "?" + req.URL.RawQuery
	}
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	lines := make([]string, 0, 6)
	for _, name := range Headers(req.Method) {
		switch name {
		case "(request-target)":
			lines = append(lines, name+": "+strings.ToLower(req.Method)+" "+target)
		case "host":
			lines = append(lines, "host: "+host)
		default:
			lines = append(lines, name+": "+req.Header.Get(name))
		}
	}
	return strings.Join(lines, "\n")
}

// Sign sets Date and Authorization, and for POST, PUT and PATCH also the body headers.
// body must be the exact bytes to send: a stream would make the hash and length unknown.
// Call it again on every retry so the Date is fresh.
func (s Signer) Sign(req *http.Request, body []byte) error {
	if s.Key == nil || s.KeyID == "" {
		return errors.New("ocisign: signer has no key")
	}
	if req.URL == nil || (req.URL.Scheme != "https" && !s.AllowInsecure) {
		return errors.New("ocisign: refusing to sign a non-HTTPS request")
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	req.Header.Set("Date", now().UTC().Format(http.TimeFormat))
	if hasBody(req.Method) {
		sum := sha256.Sum256(body)
		if req.Header.Get("Content-Type") == "" {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Content-Length", strconv.Itoa(len(body)))
		req.Header.Set("X-Content-Sha256", base64.StdEncoding.EncodeToString(sum[:]))
		req.ContentLength = int64(len(body))
		req.Body = io.NopCloser(bytes.NewReader(body))
		if len(body) == 0 {
			// A non-nil empty reader would make net/http send chunked, dropping the signed length.
			req.Body = http.NoBody
		}
		req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(body)), nil }
	} else if len(body) > 0 {
		return errors.New("ocisign: body is not allowed on this method")
	}
	digest := sha256.Sum256([]byte(SigningString(req)))
	sig, err := rsa.SignPKCS1v15(rand.Reader, s.Key, crypto.SHA256, digest[:])
	if err != nil {
		return errors.New("ocisign: request signing failed")
	}
	req.Header.Set("Authorization", fmt.Sprintf(
		`Signature version="1",keyId="%s",algorithm="rsa-sha256",headers="%s",signature="%s"`,
		s.KeyID, strings.Join(Headers(req.Method), " "), base64.StdEncoding.EncodeToString(sig)))
	return nil
}
