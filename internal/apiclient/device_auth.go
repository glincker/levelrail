package apiclient

import (
	"context"
	"net/http"
	"time"
)

// DeviceStartRequest mirrors internal/api's deviceStartRequest.
type DeviceStartRequest struct {
	ClientName string `json:"client_name,omitempty"`
}

// DeviceStartResponse mirrors internal/api's deviceStartResponse.
type DeviceStartResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// DeviceTokenRequest mirrors internal/api's deviceTokenRequest.
type DeviceTokenRequest struct {
	DeviceCode string `json:"device_code"`
}

// DeviceTokenResponse mirrors internal/api's createTokenResponse, the
// shape POST /api/v1/auth/device/token returns once a request is
// approved.
type DeviceTokenResponse struct {
	ID         string     `json:"id"`
	Name       string     `json:"name"`
	Abilities  []string   `json:"abilities"`
	CreatedAt  time.Time  `json:"created_at"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
	RevokedAt  *time.Time `json:"revoked_at,omitempty"`
	Token      string     `json:"token"`
}

// StartDeviceAuth calls POST /api/v1/auth/device/start. Unauthenticated
// by design: call it on a Client built with an empty token
// (NewClient(baseURL, "")).
func (c *Client) StartDeviceAuth(ctx context.Context, req DeviceStartRequest) (DeviceStartResponse, error) {
	var out DeviceStartResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/device/start", req, &out)
	return out, err
}

// PollDeviceAuthToken calls POST /api/v1/auth/device/token once.
// Unauthenticated, same as StartDeviceAuth. Returns *APIError with
// Message one of "authorization_pending"/"access_denied"/
// "expired_token" until the request has been approved; the caller is
// expected to poll this on an interval (DeviceStartResponse.Interval)
// until it either returns a token or a terminal error.
func (c *Client) PollDeviceAuthToken(ctx context.Context, deviceCode string) (DeviceTokenResponse, error) {
	var out DeviceTokenResponse
	err := c.do(ctx, http.MethodPost, "/api/v1/auth/device/token", DeviceTokenRequest{DeviceCode: deviceCode}, &out)
	return out, err
}
