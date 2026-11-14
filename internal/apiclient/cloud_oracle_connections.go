package apiclient

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

const oracleConnectionsPath = "/api/v1/settings/cloud-connections/oracle"

// OracleConnectionRequest is everything but the private key, which travels as caller owned bytes.
type OracleConnectionRequest struct {
	Name            string `json:"name"`
	TenancyOCID     string `json:"tenancy_ocid"`
	Region          string `json:"region"`
	CompartmentOCID string `json:"compartment_ocid"`
	UserOCID        string `json:"user_ocid"`
	Fingerprint     string `json:"fingerprint"`
}

// OracleConnection is the server's answer; it never carries credential material.
type OracleConnection struct {
	Name              string `json:"name"`
	Provider          string `json:"provider"`
	AuthMethod        string `json:"auth_method"`
	Region            string `json:"region"`
	CompartmentOCID   string `json:"compartment_ocid"`
	FingerprintPrefix string `json:"fingerprint_prefix"`
}

// CreateOracleConnection calls POST /api/v1/settings/cloud-connections/oracle once, never retried.
// The key is spliced into a body this method wipes on return, so no Go string ever holds it.
func (c *Client) CreateOracleConnection(ctx context.Context, req OracleConnectionRequest, privateKeyPEM []byte) (OracleConnection, error) {
	var out OracleConnection
	head, err := json.Marshal(req)
	if err != nil || len(head) < 2 {
		return out, fmt.Errorf("encode request body: %w", err)
	}
	body := make([]byte, 0, len(head)+len(privateKeyPEM)*2+32)
	defer func() { clear(body[:cap(body)]) }()
	body = append(body, head[:len(head)-1]...)
	body = append(body, `,"private_key_pem":"`...)
	body = appendJSONEscaped(body, privateKeyPEM)
	body = append(body, `"}`...)
	_, err = c.doOnce(ctx, http.MethodPost, oracleConnectionsPath, body, &out)
	return out, err
}

// appendJSONEscaped escapes bytes for a JSON string without building an intermediate string.
func appendJSONEscaped(dst, src []byte) []byte {
	const hexDigits = "0123456789abcdef"
	for _, b := range src {
		switch {
		case b == '"' || b == '\\':
			dst = append(dst, '\\', b)
		case b == '\n':
			dst = append(dst, '\\', 'n')
		case b < 0x20 || b == 0x7f:
			dst = append(dst, '\\', 'u', '0', '0', hexDigits[b>>4], hexDigits[b&0xf])
		default:
			dst = append(dst, b)
		}
	}
	return dst
}
