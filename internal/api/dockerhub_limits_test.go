package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/glincker/levelrail/kit/dockerhub"
)

// fakeHubAccount satisfies DockerHubClient and DockerHubAccountClient.
type fakeHubAccount struct {
	fakeDockerHubClient
	validateErr error
	limitsErr   error
	limits      dockerhub.PullLimits
	gotToken    string
}

func (f *fakeHubAccount) CachedValidate(_ context.Context, c dockerhub.Credentials, _ time.Duration, _ bool) (dockerhub.CachedValidation, error) {
	f.gotToken = c.Token
	if f.validateErr != nil {
		return dockerhub.CachedValidation{}, f.validateErr
	}
	return dockerhub.CachedValidation{CheckedAt: time.Now(), Validation: dockerhub.Validation{
		Kind: c.Kind(), Username: c.Username, Scopes: []string{"repository:ratelimitpreview/test:pull"},
		Capabilities: []dockerhub.Capability{{Name: "repository_list", Status: dockerhub.StatusOK}}, PrivateReposVisible: true,
	}}, nil
}

func (f *fakeHubAccount) CachedPullLimits(context.Context, dockerhub.Credentials, time.Duration, bool) (dockerhub.PullLimits, time.Time, bool, error) {
	return f.limits, time.Now(), false, f.limitsErr
}

func (f *fakeHubAccount) ListRepositories(context.Context, dockerhub.Credentials, string, string, int, int) (dockerhub.RepositoryPage, error) {
	return dockerhub.RepositoryPage{}, nil
}

func (f *fakeHubAccount) ListTagsPage(context.Context, dockerhub.Credentials, string, string, int, int) (dockerhub.TagPage, error) {
	return dockerhub.TagPage{}, nil
}

func (f *fakeHubAccount) ResolveDigest(context.Context, dockerhub.Credentials, string, string, string) (dockerhub.Manifest, error) {
	return dockerhub.Manifest{}, nil
}

const hubCreateBody = `{"name":"hub","registry_host":"docker.io","username":"acme","password":"dckr_pat_topsecret"}`

func TestDockerHubCreateRejectedTokenIsNotStored(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	rt.dockerHubClient = &fakeHubAccount{validateErr: dockerhub.ErrUnauthorized}
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", hubCreateBody))
	if rec.Code != http.StatusBadRequest || len(setter.calls) != 0 {
		t.Fatalf("status=%d secretWrites=%d body=%s", rec.Code, len(setter.calls), rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "topsecret") {
		t.Fatal("response leaks the token")
	}
}

func TestDockerHubStatusAndLimits(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{resolveValue: "dckr_pat_topsecret"}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	fake := &fakeHubAccount{limits: dockerhub.PullLimits{Known: true, Limit: 200, Remaining: 10, WindowSeconds: 21600, Authenticated: true}}
	rt.dockerHubClient = fake
	cookie := loginTestSession(t, rt, db)

	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", hubCreateBody))
	if rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	var created registryCredentialResource
	_ = json.NewDecoder(rec.Body).Decode(&created)

	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/dockerhub", ""))
	if rec.Code != http.StatusOK {
		t.Fatalf("status: %d %s", rec.Code, rec.Body.String())
	}
	var st dockerHubStatusResponse
	if err := json.NewDecoder(rec.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if !st.Valid || st.TokenKind != "personal" || st.Namespace != "acme" || st.ExpiryKnown || st.Limits == nil || !st.Limits.Low || st.Scopes == nil {
		t.Fatalf("status = %+v", st)
	}
	if strings.Contains(rec.Body.String(), "topsecret") {
		t.Fatal("status leaks the token")
	}

	fake.limitsErr = &dockerhub.RateLimitedError{RetryAfter: 30 * time.Second}
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/limits", ""))
	if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "30" || !strings.Contains(rec.Body.String(), "registry_rate_limited") {
		t.Fatalf("limits 429: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
}

func TestDockerHubStatusRejectedToken(t *testing.T) {
	setter := &fakeRegistryCredentialSecretsSetter{resolveValue: "dckr_pat_x"}
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, setter)
	fake := &fakeHubAccount{}
	rt.dockerHubClient = fake
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodPost, "/api/v1/registry-credentials", hubCreateBody))
	var created registryCredentialResource
	_ = json.NewDecoder(rec.Body).Decode(&created)

	fake.validateErr = dockerhub.ErrUnauthorized
	rec = httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/"+created.ID+"/dockerhub?refresh=true", ""))
	var st dockerHubStatusResponse
	_ = json.NewDecoder(rec.Body).Decode(&st)
	if rec.Code != http.StatusOK || st.Valid || st.Message == "" || st.Capabilities == nil {
		t.Fatalf("rejected status: %d %+v", rec.Code, st)
	}
}

func TestDockerHubRoutesNeedAccountClient(t *testing.T) {
	rt, db := newTestRouterWithRegistryCredentialSecrets(t, &fakeRegistryCredentialSecretsSetter{})
	rt.dockerHubClient = &fakeDockerHubClient{}
	cookie := loginTestSession(t, rt, db)
	rec := httptest.NewRecorder()
	rt.Handler().ServeHTTP(rec, authedRequest(t, cookie, http.MethodGet, "/api/v1/registry-credentials/x/limits", ""))
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d", rec.Code)
	}
}
