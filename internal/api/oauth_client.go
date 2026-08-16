package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/GLINCKER/levelrail/internal/store"
	"golang.org/x/oauth2"
	xgithub "golang.org/x/oauth2/github"
	xgoogle "golang.org/x/oauth2/google"
)

// oauthUserInfo is what any provider's flow reduces to: enough to look
// up or create a store.OAuthIdentity/store.User.
type oauthUserInfo struct {
	ProviderUserID string
	Email          string
	DisplayName    string
}

// oauthProviderClient is the narrow, consumer-defined seam oauth.go
// depends on instead of golang.org/x/oauth2 directly, so tests can
// substitute a hand-written fake (the same convention GitHubAppClient
// uses).
type oauthProviderClient interface {
	AuthCodeURL(state string) string
	Exchange(ctx context.Context, code string) (*oauth2.Token, error)
	FetchUserInfo(ctx context.Context, token *oauth2.Token) (oauthUserInfo, error)
}

// oauthClientFactory builds an oauthProviderClient for one sign-in
// attempt. A Router field of func type, like fetch/listBranches, not an
// interface: tests override rt.oauthClientFactory directly with a fake.
type oauthClientFactory func(provider string, settings store.OAuthProviderSettings, clientSecret, redirectURL string) (oauthProviderClient, error)

func defaultOAuthClientFactory(provider string, settings store.OAuthProviderSettings, clientSecret, redirectURL string) (oauthProviderClient, error) {
	switch provider {
	case store.OAuthProviderGoogle:
		return &googleOAuthClient{cfg: oauth2.Config{
			ClientID:     settings.ClientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     xgoogle.Endpoint,
			Scopes:       []string{"openid", "email", "profile"},
		}}, nil
	case store.OAuthProviderGitHub:
		return &githubOAuthClient{cfg: oauth2.Config{
			ClientID:     settings.ClientID,
			ClientSecret: clientSecret,
			RedirectURL:  redirectURL,
			Endpoint:     xgithub.Endpoint,
			Scopes:       []string{"read:user", "user:email"},
		}}, nil
	default:
		return nil, fmt.Errorf("api: unknown oauth provider %q", provider)
	}
}

type googleOAuthClient struct {
	cfg oauth2.Config
}

func (c *googleOAuthClient) AuthCodeURL(state string) string {
	return c.cfg.AuthCodeURL(state)
}

func (c *googleOAuthClient) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return c.cfg.Exchange(ctx, code)
}

// FetchUserInfo calls Google's OIDC userinfo endpoint. email_verified is
// required: an unverified email isn't trustworthy enough to match
// against an existing local account (completeOAuthSignin).
func (c *googleOAuthClient) FetchUserInfo(ctx context.Context, token *oauth2.Token) (oauthUserInfo, error) {
	client := c.cfg.Client(ctx, token)
	var body struct {
		Sub           string `json:"sub"`
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := getJSON(ctx, client, "https://www.googleapis.com/oauth2/v3/userinfo", &body); err != nil {
		return oauthUserInfo{}, fmt.Errorf("fetch google userinfo: %w", err)
	}
	if body.Sub == "" || body.Email == "" || !body.EmailVerified {
		return oauthUserInfo{}, fmt.Errorf("google userinfo: missing subject or unverified email")
	}
	name := body.Name
	if name == "" {
		name = body.Email
	}
	return oauthUserInfo{ProviderUserID: body.Sub, Email: body.Email, DisplayName: name}, nil
}

type githubOAuthClient struct {
	cfg oauth2.Config
}

func (c *githubOAuthClient) AuthCodeURL(state string) string {
	return c.cfg.AuthCodeURL(state)
}

func (c *githubOAuthClient) Exchange(ctx context.Context, code string) (*oauth2.Token, error) {
	return c.cfg.Exchange(ctx, code)
}

// FetchUserInfo calls GitHub's /user endpoint, then /user/emails as a
// fallback since /user.email is null whenever the email is private.
func (c *githubOAuthClient) FetchUserInfo(ctx context.Context, token *oauth2.Token) (oauthUserInfo, error) {
	client := c.cfg.Client(ctx, token)
	var user struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
		Name  string `json:"name"`
		Email string `json:"email"`
	}
	if err := getJSON(ctx, client, "https://api.github.com/user", &user); err != nil {
		return oauthUserInfo{}, fmt.Errorf("fetch github user: %w", err)
	}
	if user.ID == 0 {
		return oauthUserInfo{}, fmt.Errorf("github user: missing id")
	}

	email := user.Email
	if email == "" {
		var emails []struct {
			Email    string `json:"email"`
			Primary  bool   `json:"primary"`
			Verified bool   `json:"verified"`
		}
		if err := getJSON(ctx, client, "https://api.github.com/user/emails", &emails); err != nil {
			return oauthUserInfo{}, fmt.Errorf("fetch github user emails: %w", err)
		}
		for _, e := range emails {
			if e.Primary && e.Verified {
				email = e.Email
				break
			}
		}
	}
	if email == "" {
		return oauthUserInfo{}, fmt.Errorf("github user: no verified email available")
	}

	name := user.Name
	if name == "" {
		name = user.Login
	}
	return oauthUserInfo{
		ProviderUserID: fmt.Sprintf("%d", user.ID),
		Email:          email,
		DisplayName:    name,
	}, nil
}

func getJSON(ctx context.Context, client *http.Client, url string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
