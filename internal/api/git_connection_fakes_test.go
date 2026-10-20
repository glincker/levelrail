package api

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/GLINCKER/levelrail/internal/bitbucketapp"
	"github.com/GLINCKER/levelrail/internal/giteaapp"
	"github.com/GLINCKER/levelrail/internal/gitlabapp"
	"github.com/GLINCKER/levelrail/internal/gitprovider"
	"github.com/GLINCKER/levelrail/internal/store"
)

// fakeGitConnSecrets is a hand-written fake for GitConnSecrets, the
// same pattern fakeGitHubAppSecrets already establishes, extended with
// Exists the way fakeGitSourceSecrets is (this interface needs both).
type fakeGitConnSecrets struct {
	mu     sync.Mutex
	values map[string]string

	setErr, resolveErr, existsErr error
}

func newFakeGitConnSecrets() *fakeGitConnSecrets {
	return &fakeGitConnSecrets{values: map[string]string{}}
}

func (f *fakeGitConnSecrets) SetValue(_ context.Context, serviceName, envKey, plaintext string) error {
	if f.setErr != nil {
		return f.setErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.values[serviceName+"|"+envKey] = plaintext
	return nil
}

func (f *fakeGitConnSecrets) Resolve(_ context.Context, serviceName, envKey string) (string, error) {
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.values[serviceName+"|"+envKey]
	if !ok {
		return "", errFakeSecretNotSet
	}
	return v, nil
}

func (f *fakeGitConnSecrets) Exists(_ context.Context, serviceName, envKey string) (bool, error) {
	if f.existsErr != nil {
		return false, f.existsErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.values[serviceName+"|"+envKey]
	return ok, nil
}

func (f *fakeGitConnSecrets) DeleteAll(_ context.Context, serviceName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for k := range f.values {
		if strings.HasPrefix(k, serviceName+"|") {
			delete(f.values, k)
		}
	}
	return nil
}

// fakeGitLabAppClient is a hand-written fake for GitLabAppClient, every
// method's return value set directly on the struct before use, the same
// convention fakeGitHubAppClient already establishes.
type fakeGitLabAppClient struct {
	revokedToken   string
	revokeErr      error
	listCalls      atomic.Int32
	listResult     gitprovider.ListResult
	exchangeTokens gitlabapp.Tokens
	exchangeErr    error
	gotCode        string
	// exchangeByCode and accountByToken let one fake serve several accounts.
	exchangeByCode map[string]gitlabapp.Tokens
	accountByToken map[string]gitlabapp.Account
	account        gitlabapp.Account
	accountErr     error
	refreshCount   atomic.Int32
	refreshGate    chan struct{}
	revokedTokens  []string

	refreshTokens gitlabapp.Tokens
	refreshErr    error

	projects        []gitlabapp.Project
	projectsByToken map[string][]gitlabapp.Project
	projectsErr     error

	project    gitlabapp.Project
	projectErr error

	branches    []gitlabapp.Branch
	branchesErr error

	createHookErr    error
	createHookURL    string
	createHookTok    string
	createHookAccess string
	createHookCall   bool

	noteErr    error
	prComments fakePRComments
	noteCalls  []fakeGitLabNote

	statusErr   error
	statusCalls []fakeGitLabCommitStatus
}

// fakeGitLabNote records one fakeGitLabAppClient.CreateMergeRequestNote
// call, mirroring fakeIssueComment (github_app_test.go).
type fakeGitLabNote struct {
	projectPath string
	mrIID       int
	body        string
}

// fakeGitLabCommitStatus records one
// fakeGitLabAppClient.CreateCommitStatus call, mirroring
// fakeCommitStatus (github_app_test.go).
type fakeGitLabCommitStatus struct {
	projectPath, sha string
	state            gitlabapp.CommitState
	targetURL        string
	description      string
	name             string
}

func (f *fakeGitLabAppClient) ExchangeCode(_ context.Context, _, _, _, _, code string) (gitlabapp.Tokens, error) {
	f.gotCode = code
	if t, ok := f.exchangeByCode[code]; ok {
		return t, f.exchangeErr
	}
	return f.exchangeTokens, f.exchangeErr
}

func (f *fakeGitLabAppClient) RevokeToken(_ context.Context, _, _, _, token string) error {
	f.revokedToken = token
	f.revokedTokens = append(f.revokedTokens, token)
	return f.revokeErr
}

func (f *fakeGitLabAppClient) RefreshToken(_ context.Context, _, _, _, _ string) (gitlabapp.Tokens, error) {
	f.refreshCount.Add(1)
	if f.refreshGate != nil {
		<-f.refreshGate
	}
	return f.refreshTokens, f.refreshErr
}

func (f *fakeGitLabAppClient) CurrentUser(_ context.Context, _, accessToken string) (gitlabapp.Account, error) {
	if f.accountErr != nil {
		return gitlabapp.Account{}, f.accountErr
	}
	if a, ok := f.accountByToken[accessToken]; ok {
		return a, nil
	}
	return f.account, nil
}

func (f *fakeGitLabAppClient) ListProjects(_ context.Context, _, accessToken string, _ gitprovider.ListOptions) ([]gitlabapp.Project, gitprovider.ListResult, error) {
	f.listCalls.Add(1)
	if p, ok := f.projectsByToken[accessToken]; ok {
		return p, f.listResult, f.projectsErr
	}
	return f.projects, f.listResult, f.projectsErr
}

func (f *fakeGitLabAppClient) GetProject(_ context.Context, _, _ string, _ int64) (gitlabapp.Project, error) {
	return f.project, f.projectErr
}

func (f *fakeGitLabAppClient) ListBranches(_ context.Context, _, _ string, _ int64) ([]gitlabapp.Branch, error) {
	return f.branches, f.branchesErr
}

func (f *fakeGitLabAppClient) CreateProjectWebhook(_ context.Context, _, accessToken string, _ int64, hookURL, secretToken string) error {
	f.createHookCall = true
	f.createHookAccess = accessToken
	f.createHookURL = hookURL
	f.createHookTok = secretToken
	return f.createHookErr
}

func (f *fakeGitLabAppClient) CreateMergeRequestNote(_ context.Context, _, _, projectPath string, mrIID int, body string) (int64, error) {
	f.noteCalls = append(f.noteCalls, fakeGitLabNote{projectPath: projectPath, mrIID: mrIID, body: body})
	if f.noteErr != nil {
		return 0, f.noteErr
	}
	return f.prComments.create(body), nil
}

func (f *fakeGitLabAppClient) ListMergeRequestNotes(_ context.Context, _, _, _ string, _ int) ([]gitprovider.Comment, error) {
	return f.prComments.list()
}

func (f *fakeGitLabAppClient) UpdateMergeRequestNote(_ context.Context, _, _, _ string, _ int, noteID int64, body string) error {
	return f.prComments.update(noteID, body)
}

func (f *fakeGitLabAppClient) CreateCommitStatus(_ context.Context, _, _, projectPath, sha string, state gitlabapp.CommitState, targetURL, description, name string) error {
	f.statusCalls = append(f.statusCalls, fakeGitLabCommitStatus{
		projectPath: projectPath, sha: sha, state: state, targetURL: targetURL, description: description, name: name,
	})
	return f.statusErr
}

type fakeBitbucketAppClient struct {
	listCalls      atomic.Int32
	listResult     gitprovider.ListResult
	exchangeTokens bitbucketapp.Tokens
	exchangeErr    error
	gotCode        string

	refreshTokens bitbucketapp.Tokens
	refreshErr    error

	repos    []bitbucketapp.Repo
	reposErr error

	repo    bitbucketapp.Repo
	repoErr error

	branches    []bitbucketapp.Branch
	branchesErr error

	createHookErr  error
	createHookURL  string
	createHookTok  string
	createHookCall bool

	commentErr   error
	prComments   fakePRComments
	commentCalls []fakeBitbucketPRComment

	statusErr   error
	statusCalls []fakeBitbucketBuildStatus
}

// fakeBitbucketPRComment records one
// fakeBitbucketAppClient.CreatePullRequestComment call, mirroring
// fakeIssueComment (github_app_test.go).
type fakeBitbucketPRComment struct {
	fullName string
	prID     int
	body     string
}

// fakeBitbucketBuildStatus records one
// fakeBitbucketAppClient.CreateCommitBuildStatus call, mirroring
// fakeCommitStatus (github_app_test.go).
type fakeBitbucketBuildStatus struct {
	fullName, commit string
	state            bitbucketapp.BuildStatusState
	targetURL        string
	description      string
	key              string
}

func (f *fakeBitbucketAppClient) ExchangeCode(_ context.Context, _, _, code string) (bitbucketapp.Tokens, error) {
	f.gotCode = code
	return f.exchangeTokens, f.exchangeErr
}

func (f *fakeBitbucketAppClient) RefreshToken(_ context.Context, _, _, _ string) (bitbucketapp.Tokens, error) {
	return f.refreshTokens, f.refreshErr
}

func (f *fakeBitbucketAppClient) ListRepos(_ context.Context, _ string, _ gitprovider.ListOptions) ([]bitbucketapp.Repo, gitprovider.ListResult, error) {
	f.listCalls.Add(1)
	return f.repos, f.listResult, f.reposErr
}

func (f *fakeBitbucketAppClient) GetRepo(_ context.Context, _, _ string) (bitbucketapp.Repo, error) {
	return f.repo, f.repoErr
}

func (f *fakeBitbucketAppClient) ListBranches(_ context.Context, _, _ string) ([]bitbucketapp.Branch, error) {
	return f.branches, f.branchesErr
}

func (f *fakeBitbucketAppClient) CreateRepoWebhook(_ context.Context, _, _, hookURL, secret string) error {
	f.createHookCall = true
	f.createHookURL = hookURL
	f.createHookTok = secret
	return f.createHookErr
}

func (f *fakeBitbucketAppClient) CreatePullRequestComment(_ context.Context, _, fullName string, prID int, body string) (int64, error) {
	f.commentCalls = append(f.commentCalls, fakeBitbucketPRComment{fullName: fullName, prID: prID, body: body})
	if f.commentErr != nil {
		return 0, f.commentErr
	}
	return f.prComments.create(body), nil
}

func (f *fakeBitbucketAppClient) ListPullRequestComments(_ context.Context, _, _ string, _ int) ([]gitprovider.Comment, error) {
	return f.prComments.list()
}

func (f *fakeBitbucketAppClient) UpdatePullRequestComment(_ context.Context, _, _ string, _ int, commentID int64, body string) error {
	return f.prComments.update(commentID, body)
}

func (f *fakeBitbucketAppClient) CreateCommitBuildStatus(_ context.Context, _, fullName, commit string, state bitbucketapp.BuildStatusState, targetURL, description, key string) error {
	f.statusCalls = append(f.statusCalls, fakeBitbucketBuildStatus{
		fullName: fullName, commit: commit, state: state, targetURL: targetURL, description: description, key: key,
	})
	return f.statusErr
}

type fakeGiteaAppClient struct {
	listCalls      atomic.Int32
	listResult     gitprovider.ListResult
	exchangeTokens giteaapp.Tokens
	exchangeErr    error
	gotCode        string

	refreshTokens giteaapp.Tokens
	refreshErr    error

	repos    []giteaapp.Repo
	reposErr error

	repo    giteaapp.Repo
	repoErr error

	branches    []giteaapp.Branch
	branchesErr error

	createHookErr  error
	createHookURL  string
	createHookTok  string
	createHookCall bool

	commentErr   error
	prComments   fakePRComments
	commentCalls []fakeGiteaIssueComment

	statusErr   error
	statusCalls []fakeGiteaCommitStatus
}

// fakeGiteaIssueComment records one
// fakeGiteaAppClient.CreateIssueComment call, mirroring
// fakeIssueComment (github_app_test.go).
type fakeGiteaIssueComment struct {
	fullName string
	number   int
	body     string
}

// fakeGiteaCommitStatus records one
// fakeGiteaAppClient.CreateCommitStatus call, mirroring
// fakeCommitStatus (github_app_test.go).
type fakeGiteaCommitStatus struct {
	fullName, sha string
	state         giteaapp.CommitStatusState
	targetURL     string
	description   string
	context       string
}

func (f *fakeGiteaAppClient) ExchangeCode(_ context.Context, _, _, _, _, code string) (giteaapp.Tokens, error) {
	f.gotCode = code
	return f.exchangeTokens, f.exchangeErr
}

func (f *fakeGiteaAppClient) RefreshToken(_ context.Context, _, _, _, _ string) (giteaapp.Tokens, error) {
	return f.refreshTokens, f.refreshErr
}

func (f *fakeGiteaAppClient) ListRepos(_ context.Context, _, _ string, _ gitprovider.ListOptions) ([]giteaapp.Repo, gitprovider.ListResult, error) {
	f.listCalls.Add(1)
	return f.repos, f.listResult, f.reposErr
}

func (f *fakeGiteaAppClient) GetRepo(_ context.Context, _, _, _ string) (giteaapp.Repo, error) {
	return f.repo, f.repoErr
}

func (f *fakeGiteaAppClient) ListBranches(_ context.Context, _, _, _ string) ([]giteaapp.Branch, error) {
	return f.branches, f.branchesErr
}

func (f *fakeGiteaAppClient) CreateRepoWebhook(_ context.Context, _, _, _, hookURL, secret string) error {
	f.createHookCall = true
	f.createHookURL = hookURL
	f.createHookTok = secret
	return f.createHookErr
}

func (f *fakeGiteaAppClient) CreateIssueComment(_ context.Context, _, _, fullName string, number int, body string) (int64, error) {
	f.commentCalls = append(f.commentCalls, fakeGiteaIssueComment{fullName: fullName, number: number, body: body})
	if f.commentErr != nil {
		return 0, f.commentErr
	}
	return f.prComments.create(body), nil
}

func (f *fakeGiteaAppClient) ListIssueComments(_ context.Context, _, _, _ string, _ int) ([]gitprovider.Comment, error) {
	return f.prComments.list()
}

func (f *fakeGiteaAppClient) UpdateIssueComment(_ context.Context, _, _, _ string, commentID int64, body string) error {
	return f.prComments.update(commentID, body)
}

func (f *fakeGiteaAppClient) CreateCommitStatus(_ context.Context, _, _, fullName, sha string, state giteaapp.CommitStatusState, targetURL, description, statusContext string) error {
	f.statusCalls = append(f.statusCalls, fakeGiteaCommitStatus{
		fullName: fullName, sha: sha, state: state, targetURL: targetURL, description: description, context: statusContext,
	})
	return f.statusErr
}

func (f *fakeBitbucketAppClient) CurrentUser(_ context.Context, _ string) (bitbucketapp.Account, error) {
	return bitbucketapp.Account{ID: "bb-user", Login: "bbuser"}, nil
}

func (f *fakeGiteaAppClient) CurrentUser(_ context.Context, _, _ string) (giteaapp.Account, error) {
	return giteaapp.Account{ID: "gi-user", Login: "giuser"}, nil
}

// newGitConnTestRouter builds a router wired with fake connection secrets and clients.
func newGitConnTestRouter(t *testing.T, secrets GitConnSecrets, opts ...Option) (*Router, *store.DB) {
	t.Helper()
	db := openTestDB(t)
	rt := NewRouter(discardLogger(), testBrand(), db, append([]Option{WithGitConnectionSecrets(secrets)}, opts...)...)
	return rt, db
}

// seedGitConn stores a connection and, when authorized, a token that is valid for hours.
func seedGitConn(t *testing.T, db *store.DB, secrets GitConnSecrets, provider, host, account string, authorized bool) store.GitConnection {
	t.Helper()
	id, err := store.NewGitConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	c := store.GitConnection{
		ID: id, Provider: provider, Label: gitConnDefaultLabel(provider, host, account), Host: host, InstanceURL: "https://" + host,
		ClientID: "cid-" + id, SecretsKey: store.GitConnectionSecretsKey(id), AccountID: account, AccountName: account,
		CreatedAt: time.Now().UTC().Add(time.Duration(len(id)) * time.Nanosecond).Format(time.RFC3339Nano),
	}
	if err := db.CreateGitConnection(t.Context(), c); err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	secretKey := gitConnClientSecretKey
	if provider == gitProviderBitbucket {
		secretKey = bitbucketSecretKey
	}
	mustSetGitConnSecret(t, secrets, c.SecretsKey, secretKey, "csecret")
	if authorized {
		mustSetGitConnSecret(t, secrets, c.SecretsKey, gitConnAccessTokenKey, "at-"+account)
		mustSetGitConnSecret(t, secrets, c.SecretsKey, gitConnRefreshTokenKey, "rt-"+account)
		mustSetGitConnSecret(t, secrets, c.SecretsKey, gitConnTokenExpiresKey, "99999999999")
	}
	got, err := db.GetGitConnection(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func mustSetGitConnSecret(t *testing.T, s GitConnSecrets, key, envKey, val string) {
	t.Helper()
	if err := s.SetValue(context.Background(), key, envKey, val); err != nil {
		t.Fatalf("seed %s: %v", envKey, err)
	}
}

// Single-account helpers for tests that need one working connection per provider.
func legacyTestKey(provider string) string { return provider + "-test/connection" }

func authorizedConnSecrets(t *testing.T, provider string) *fakeGitConnSecrets {
	t.Helper()
	s := newFakeGitConnSecrets()
	key := legacyTestKey(provider)
	secretKey := gitConnClientSecretKey
	if provider == gitProviderBitbucket {
		secretKey = bitbucketSecretKey
	}
	mustSetGitConnSecret(t, s, key, secretKey, "csecret")
	mustSetGitConnSecret(t, s, key, gitConnAccessTokenKey, "at-1")
	mustSetGitConnSecret(t, s, key, gitConnTokenExpiresKey, "99999999999")
	return s
}

func authorizedGitLabSecrets(t *testing.T) *fakeGitConnSecrets {
	return authorizedConnSecrets(t, gitProviderGitLab)
}
func authorizedBitbucketSecrets(t *testing.T) *fakeGitConnSecrets {
	return authorizedConnSecrets(t, gitProviderBitbucket)
}
func authorizedGiteaSecrets(t *testing.T) *fakeGitConnSecrets {
	return authorizedConnSecrets(t, gitProviderGitea)
}

func seedProviderConn(t *testing.T, db *store.DB, provider, instanceURL, clientID string) store.GitConnection {
	t.Helper()
	id, err := store.NewGitConnectionID()
	if err != nil {
		t.Fatal(err)
	}
	c := store.GitConnection{
		ID: id, Provider: provider, Label: provider, Host: store.RepoHost(instanceURL), InstanceURL: instanceURL, ClientID: clientID,
		SecretsKey: legacyTestKey(provider), AccountID: "tester", AccountName: "tester", CreatedAt: "2026-08-16T00:00:00Z",
	}
	if err := db.CreateGitConnection(t.Context(), c); err != nil {
		t.Fatalf("seed %s connection: %v", provider, err)
	}
	return c
}

func seedGitLabAppConnection(t *testing.T, db *store.DB) store.GitConnection {
	return seedProviderConn(t, db, gitProviderGitLab, "https://gitlab.example.com", "cid")
}

func seedBitbucketAppConnection(t *testing.T, db *store.DB) {
	seedProviderConn(t, db, gitProviderBitbucket, "https://bitbucket.org", "bbkey")
}

func seedGiteaAppConnection(t *testing.T, db *store.DB) {
	seedProviderConn(t, db, gitProviderGitea, "https://git.example.com", "cid")
}

func newTestRouterWithGitLabApp(t *testing.T, secrets GitConnSecrets, client GitLabAppClient) (*Router, *store.DB) {
	t.Helper()
	rt, db := newGitConnTestRouter(t, secrets)
	rt.gitlabAppClient = client
	return rt, db
}

func newTestRouterWithProviderAndGitSource(t *testing.T, secrets GitConnSecrets, gitSourceSecrets GitSourceSecrets) (*Router, *store.DB) {
	t.Helper()
	return newGitConnTestRouter(t, secrets, WithGitSourceSecrets(gitSourceSecrets))
}

func newTestRouterWithGitLabAndGitSource(t *testing.T, secrets GitConnSecrets, client GitLabAppClient, gitSourceSecrets GitSourceSecrets) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouterWithProviderAndGitSource(t, secrets, gitSourceSecrets)
	rt.gitlabAppClient = client
	return rt, db
}

func newTestRouterWithBitbucketAndGitSource(t *testing.T, secrets GitConnSecrets, client BitbucketAppClient, gitSourceSecrets GitSourceSecrets) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouterWithProviderAndGitSource(t, secrets, gitSourceSecrets)
	rt.bitbucketAppClient = client
	return rt, db
}

func newTestRouterWithGiteaAndGitSource(t *testing.T, secrets GitConnSecrets, client GiteaAppClient, gitSourceSecrets GitSourceSecrets) (*Router, *store.DB) {
	t.Helper()
	rt, db := newTestRouterWithProviderAndGitSource(t, secrets, gitSourceSecrets)
	rt.giteaAppClient = client
	return rt, db
}
