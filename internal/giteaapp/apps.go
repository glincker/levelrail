package giteaapp

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// OAuthApp is an OAuth2 application created on the forge for this control
// plane. Secret is only returned once, at creation.
type OAuthApp struct {
	ID       int64
	Name     string
	ClientID string
	Secret   string
}

type createOAuthAppRequest struct {
	Name         string   `json:"name"`
	RedirectURIs []string `json:"redirect_uris"`
	Confidential bool     `json:"confidential_client"`
}

type oauthAppResponse struct {
	ID           int64  `json:"id"`
	Name         string `json:"name"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
}

// CreateOAuthApp registers an OAuth2 application under the token's user
// (POST /user/applications/oauth2, same on Gitea and Forgejo), so connecting
// needs one bootstrap token instead of hand-copied client credentials. The
// caller must not store the bootstrap token.
func (c *Client) CreateOAuthApp(ctx context.Context, instanceURL, accessToken, name, redirectURI string) (OAuthApp, error) {
	payload, err := json.Marshal(createOAuthAppRequest{Name: name, RedirectURIs: []string{redirectURI}, Confidential: true})
	if err != nil {
		return OAuthApp{}, fmt.Errorf("giteaapp: marshal oauth app request: %w", err)
	}
	var out oauthAppResponse
	u := apiBaseURL(instanceURL) + "/user/applications/oauth2"
	if err := c.do(ctx, http.MethodPost, u, "Bearer "+accessToken, bytes.NewReader(payload), &out); err != nil {
		return OAuthApp{}, err
	}
	return OAuthApp{ID: out.ID, Name: out.Name, ClientID: out.ClientID, Secret: out.ClientSecret}, nil
}

// DeleteOAuthApp removes an application created by CreateOAuthApp, which is
// also how a disconnect revokes every token it issued.
func (c *Client) DeleteOAuthApp(ctx context.Context, instanceURL, accessToken string, appID int64) error {
	u := apiBaseURL(instanceURL) + "/user/applications/oauth2/" + strconv.FormatInt(appID, 10)
	return c.do(ctx, http.MethodDelete, u, "Bearer "+accessToken, nil, nil)
}
