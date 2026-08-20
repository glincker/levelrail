# Git provider integrations: shared abstraction, GHE, Bitbucket, connect-flow UX

Status: proposed. Feeds the implementation plan for Bitbucket support, GitHub Enterprise Server support, and the connect-flow preview step.

Context: GitHub App and GitLab OAuth App integrations are both shipped and independently typed. Bitbucket is next, and GitHub needs self-hosted (GHES) support. This is the "rule of three" moment, so the abstraction question gets answered on purpose rather than by default.

## 1. Shared Go abstraction: split the answer, do not unify the handshake

**Recommendation: a narrow abstraction. Introduce `internal/gitprovider` covering only the post-authentication repository surface. Keep every auth handshake in its own package and on its own routes. Do not put `Connect` in any interface.**

### Why not the full `GitProvider` interface

The convergence point already exists and it is downstream of auth, not at it. `internal/api/git_sources.go`'s `connectGitSource` is already the single place a repository becomes an app's deploy source, and `handleUseGitLabProjectAsSource` already funnels into it, producing the identical `store.GitSource` row and pointing at the identical webhook receiver a hand-typed source uses. Bitbucket gets that for free without any new interface. An interface that spans connect through webhook creation would therefore be re-abstracting a path that is already converged, in order to unify the one part that genuinely is not.

### What actually differs between the handshakes

These are not cosmetic differences and an interface that hid them would be lying:

- **Who creates the app.** GitHub's manifest flow creates the App programmatically: the browser form POSTs a JSON manifest to `settings/apps/new`, GitHub creates the App and hands back a one-time code. GitLab and Bitbucket have no equivalent. The operator registers the app by hand, out of band, and pastes `client_id`/`client_secret` in.
- **The browser is a required participant, differently.** GitHub needs a server-rendered self-submitting HTML form (`writeGitHubAppManifestForm`). GitLab and Bitbucket need a 302 to an authorize URL. Any interface method covering both collapses to `func(http.ResponseWriter, *http.Request)`, which is a router, not an abstraction.
- **Two-step completion means two different second steps.** GitHub: create App, then *install* it on an account or org, yielding `installation_id` verified server-side against the App's own `app_id` (`handleGitHubAppInstalled`). GitLab and Bitbucket: configure, then *authorize*, yielding an access token. `installed` and `authorized` are already separate booleans in the two status resources for exactly this reason.
- **Token lifecycle is structurally different.** GitHub mints a fresh installation token per call by signing an RS256 JWT with a stored PEM private key: no refresh token, no stored access token. GitLab and Bitbucket store an access token plus a refresh token plus an expiry, and refresh on demand (`gitlabAccessToken`). Bitbucket Cloud tokens expire in 2 hours, so refresh is mandatory rather than incidental.
- **Credential shape differs.** GitHub: `app_id`, `client_id`, `installation_id`, `account_login` plus three secrets. GitLab: `instance_url`, `client_id` plus four secrets. Bitbucket adds a workspace concept neither of the others has.
- **Token scope semantics differ.** A GitHub installation token is scoped to the installation ("repos this installation was granted"). GitLab and Bitbucket tokens are scoped to the authorizing user ("repos this user is a member of"). Same method name, materially different meaning.

### What is genuinely uniform, and is worth the interface

Everything after a token exists: list repos, list branches, create a push webhook.

```go
// Package gitprovider holds the provider-agnostic surface that only
// exists after a provider connection is authenticated. Authentication
// itself is deliberately absent: see docs/design/git-provider-integrations.md.
package gitprovider

type Kind string

const (
	KindGitHub    Kind = "github"
	KindGitLab    Kind = "gitlab"
	KindBitbucket Kind = "bitbucket"
)

// Repo is one repository the connected credential can reach.
type Repo struct {
	// ID is provider-native and opaque: a GitHub "owner/repo" pair, a
	// GitLab numeric project ID, a Bitbucket "workspace/repo_slug".
	// String, not int64, because only GitLab's is numeric. Callers pass
	// it back verbatim and never parse it.
	ID            string
	FullName      string
	CloneURL      string
	DefaultBranch string
	Private       bool
	WebURL        string
}

// Branch is one branch head.
type Branch struct {
	Name string
	SHA  string
}

// PushHook is the webhook this control plane wants registered on a repo.
type PushHook struct {
	URL    string
	Secret string
}

// Source is one authenticated provider connection. Implementations are
// constructed per request by internal/api, closing over whatever token
// acquisition that provider needs, so no method here takes a credential
// argument.
//
// ListRepos returns installation-scoped repositories on GitHub and
// user-scoped memberships on GitLab and Bitbucket. That difference is
// real and not normalized away.
type Source interface {
	Kind() Kind
	ListRepos(ctx context.Context) ([]Repo, error)
	ListBranches(ctx context.Context, repoID string) ([]Branch, error)
	GetRepo(ctx context.Context, repoID string) (Repo, error)
	CreatePushHook(ctx context.Context, repoID string, hook PushHook) error
}
```

`ID string` is the one real cost: GitHub's client currently takes `owner, repo`, GitLab's takes an `int64` project ID. Each adapter parses its own ID format at the edge. That is acceptable because the alternative (an `any` ID, or a three-armed union) is worse and would violate the no-`any`-in-exported-signatures rule.

**The interface only pays for itself if the HTTP surface unifies too.** So this work also collapses the repo-facing routes:

- `GET /api/v1/git-providers/{kind}/repos`
- `GET /api/v1/git-providers/{kind}/repos/{id...}/branches`
- `POST /api/v1/git-providers/{kind}/repos/{id...}/use-as-source`

Existing `/api/v1/github-app/repos`, `/api/v1/github-app/repos/{owner}/{repo}/branches`, and `/api/v1/gitlab-app/projects*` stay as thin aliases delegating to the same handlers, so nothing already shipped breaks.

Connect, callback, disconnect, and status routes stay per provider, unchanged, under `/api/v1/{github,gitlab,bitbucket}-app`.

Note today GitHub has no `use-as-source` and registers no repo webhook (its manifest sets `hook_attributes.active: false`). Unifying the route surface makes that gap visible and closable rather than invisible.

### Small dedupes to fold into the same work

- `githubAppRegistrationState` (`internal/api/github_app.go`) and `pendingState` (`internal/api/pending_state.go`) are the same type twice. Delete the GitHub copy, use `pendingState` for all three providers.
- `verifyGitPushWebhookAuth` (`internal/api/git_webhook.go`) already branches on GitHub's `X-Hub-Signature-256` vs GitLab's `X-Gitlab-Token`. Bitbucket adds a third branch, `X-Hub-Signature` (sha256 HMAC, no `-256` suffix). Table-driven test over the header matrix, including "no known header present" failing closed.
- `githubAppBaseURL` is used by the GitLab handlers too, under its GitHub name. Rename to `controlPlaneBaseURL` and move it out of `github_app.go`. Same for `errGitHubAppNoPrimaryDomain`.

### What this recommendation does not do

It does not create a `GitProvider` interface with `Connect`/`Disconnect`. It does not merge the three connection tables into one polymorphic table. It does not change how any provider stores credentials. If a fourth provider (Gitea, which is GitLab-shaped) lands and the connect handshakes are still three copies, revisit then with real evidence rather than now with two.

## 2. GitHub Enterprise Server: single `instance_url`, plus a reachability preflight

**Recommendation: follow GitLab's existing single `instance_url` field exactly. Do not adopt Dokploy's external-URL/internal-URL split.**

The split exists because the browser's reachable address and the control plane's reachable address can differ on an internal instance. It buys that case at the cost of a second field every operator must reason about, and it has a live bug from exactly that complexity (Dokploy/dokploy#3848, self-hosted OAuth callback ECONNREFUSED). The cheaper mitigation for the same failure mode is a preflight: on save, the control plane calls the instance's own version endpoint (`/api/v3/meta` for GHES) and returns a specific, actionable 400 if it cannot reach it, turning a silent runtime OAuth failure into an immediate save error, without a second field.

Concrete changes:

**Store.** New migration adding `instance_url TEXT NOT NULL DEFAULT 'https://github.com'` to `github_app_connections`. The default makes every existing row correct with no backfill.

**`internal/githubapp/client.go`.** Add `APIBaseURL(instanceURL string) string`: returns `https://api.github.com` when `instanceURL` is `https://github.com` (or empty), otherwise `strings.TrimRight(instanceURL, "/") + "/api/v3"`. GHES's REST root is `/api/v3` under the instance host, not a separate `api.` hostname. Thread `instanceURL` as the first argument of every method, mirroring `gitlabapp.Client`, and widen the `GitHubAppClient` interface in `internal/api/github_app.go` to match.

**`internal/githubapp/manifest.go`.** `BuildManifest` needs no signature change: every URL it emits is derived from our own `baseURL`, not GitHub's.

- `writeGitHubAppManifestForm` hardcodes `https://github.com/settings/apps/new`. Becomes `<instance>/settings/apps/new`, and for an org-owned App, `<instance>/organizations/<org>/settings/apps/new`.
- `handleGitHubAppCallback`'s post-create redirect hardcodes `https://github.com/apps/<slug>/installations/new`. Becomes `<instance>/apps/<slug>/installations/new`.

**Manual connect (`github_app_manual.go`)** gains the same `instance_url` field, defaulting to `https://github.com`.

**Explicitly out of scope:** no `insecure_skip_verify` / custom CA toggle. GHES instances with a private CA are supported by adding that CA to the host trust store, documented, not by adding a flag that turns off certificate verification for a credential-bearing connection.

## 3. Bitbucket: Cloud now, Server not scheduled

**Recommendation: Bitbucket Cloud only. Bitbucket Server / Data Center is explicitly not on the roadmap, and this is not a "fast follow" with a date attached.**

1. Bitbucket Server has no OAuth-consumer equivalent. Integration is via Application Links or HTTP access tokens, a paste-a-token flow.
2. **A paste-a-token flow already ships here.** `PUT /api/v1/apps/{name}/git-source` takes a repo URL plus an optional deploy token and returns a webhook secret, and `handleGitPushWebhook` verifies GitHub-style HMAC. A Bitbucket Data Center user can connect a repo today by pasting the clone URL and an HTTP access token, then adding the webhook by hand.
3. Atlassian ended Bitbucket Server sales and support (Feb 2024); remaining installs are Data Center. Building a first-class integration against a shrinking, self-hosted-only, token-paste-shaped target is a poor use of the next slice of work.

Document point 2 so "no Bitbucket Server support" reads as a documented path rather than a hole.

### Minimum viable Bitbucket Cloud, matching the bar GitHub and GitLab already meet

Status endpoint, connect and disconnect, credential storage through `internal/secrets`, repo listing behind `AbilityReadSensitive`, branch listing, use-as-source landing in `store.GitSource`, automatic push-webhook registration, and a webhook receiver that verifies the request and triggers a deploy. GitLab meets all of it; GitHub currently meets all but webhook registration.

- **`internal/bitbucketapp`**, structured like `internal/gitlabapp`: OAuth code exchange and refresh against `https://bitbucket.org/site/oauth2/access_token`, plus the API slice below. No instance URL parameter (Cloud only, fixed `api.bitbucket.org/2.0`).
- **Operator-registered OAuth consumer**: `key` (client id) plus `secret`. The connect dialog must tell the operator which permissions to tick on the consumer itself (Account: read, Repositories: read, Webhooks: read and write), since Bitbucket scopes are configured on the consumer, not requested per authorization.
- **Token refresh is mandatory**, not optional: Bitbucket access tokens live 2 hours. Reuse the `gitlabAccessToken` pattern.
- **Repo listing**: `GET /2.0/repositories?role=member`. Pagination is cursor-based via a `next` URL in the response body, a third pagination shape distinct from both existing clients' page-counter loops. Cap the follow count the way `listPageCap` already does.
- **Branch listing**: `GET /2.0/repositories/{workspace}/{repo_slug}/refs/branches`.
- **Webhook registration**: `POST /2.0/repositories/{workspace}/{repo_slug}/hooks`, event `repo:push`, `active: true`, with a secret. If the secret cannot be set, fail the use-as-source call with the same "git source connected, but registering the webhook failed; add it manually" 502 the GitLab path already returns, rather than registering an unauthenticated hook.
- **Push payload adapter**, the single unavoidable backend change. `internal/webhook.ParsePushEvent` requires top-level `ref` and `after`, which GitHub and GitLab both send. Bitbucket sends neither: `push.changes[].new.name` (a bare branch name, not a `refs/heads/` ref) and `push.changes[].new.target.hash`. Add a provider-aware parse selected by `X-Event-Key: repo:push`, normalizing into the existing `PushEvent`. Table-driven test with fixtures for all three providers, including a Bitbucket branch-delete change where `new` is null.
- **Webhook auth**: third branch in `verifyGitPushWebhookAuth` for `X-Hub-Signature`.
- **Frontend**: `BitbucketAppConnectionCard.tsx`, `queries/bitbucketApp.ts`, `types/bitbucketApp.ts`, repo picker reusing the unified `/api/v1/git-providers/{kind}/repos` route.

Out of scope for the first slice: Bitbucket Server / Data Center, app passwords as an alternative credential, workspace-restriction UI, pull-request preview environments, auto-populating the clone deploy token from the OAuth token.

## 4. Manifest preview and confirm step

`GitHubAppConnectionCard.tsx` currently does `window.location.href = '/api/v1/github-app/register/start'` with no preview. Everything the operator is about to grant lives in a warning paragraph next to the button. GitHub's own confirmation page then lets them edit the App name and nothing else, so callback URL, webhook URL, permissions, and events must be right before the redirect. That is the definition of a step that needs a confirm.

**Backend: `GET /api/v1/github-app/register/preview`**, `AbilityRoot`, returning JSON:

```json
{
  "instance_url": "", "app_name": "", "homepage_url": "",
  "callback_url": "", "setup_url": "", "webhook_url": "",
  "webhook_active": false, "permissions": {"contents": "read"},
  "events": [], "public": false, "request_oauth_on_install": false,
  "owner": "user", "organization": ""
}
```

Built by calling `githubapp.BuildManifest` with the same inputs `handleStartGitHubAppRegistration` uses, so preview and the real POST cannot drift. The preview endpoint must not call `pendingState.begin()`: state is minted only when the operator actually confirms and hits `register/start`.

`register/start` gains two optional, validated query parameters: `name` (the one field GitHub itself lets you change) and `org` (an org login, changing the form action to `<instance>/organizations/<org>/settings/apps/new`).

**Frontend: `GitHubAppManifestPreviewDialog`**, opened by the existing "Add GitHub App" button:

- Editable App name, prefilled from `app_name`.
- Owner selector: personal account or organization, with a login input when organization is chosen.
- Read-only monospace list of the four URLs (homepage, callback, setup, webhook), each labelled with what GitHub will do with it.
- Permissions as `contents: read` style rows, plus the events list, with an honest "webhooks are declared but inactive" note when `webhook_active` is false.
- The existing reachability warning, moved here from the card body.
- Confirm navigates with `?name=&org=`; Cancel does nothing.

**Should this generalize?**

- The manifest preview stays GitHub-specific. GitLab and Bitbucket create nothing remotely on the operator's behalf, so there is no surprise to prevent.
- One piece does generalize, and fixes a live bug: `GitLabAppConnectionCard.tsx`'s `ConfigureDialog` tells the operator to register redirect URI `${window.location.origin}/api/v1/gitlab-app/callback`, but the backend builds its redirect URI from `IngressSettings.PrimaryDomain`. An operator viewing the dashboard over an IP, tunnel, or secondary hostname is told to register a redirect URI that will not match, and OAuth fails. Both status endpoints already return `base_url`. Extract `<CallbackURLNotice baseURL={status.base_url} path="/api/v1/gitlab-app/callback" />` rendering the server-derived URL with a copy button and an explicit "set a primary domain first" state, used in all three cards and inside the GitHub preview dialog.

## 5. Frontend: extract four primitives, keep three cards

**Recommendation: no generic `ProviderConnectionCard` and no `useProviderConnection` hook. Extract four small primitives and let each card keep its own body.**

The two existing cards are 465 and 322 lines, and the parts that are actually identical are the chrome, not the logic. The state machines genuinely differ: GitHub is not-connected/connected/installed with two competing entry actions on one row; GitLab is a strict not-connected/configured/authorized sequence; Bitbucket will match GitLab's shape but with a workspace concept. A config-driven generic card would need a discriminated union over three state machines plus per-provider render slots, more code than the three cards and trending toward the 500-line cap.

Worth extracting, because it is byte-identical apart from strings and Bitbucket would make it a third copy:

1. `<DisconnectProviderDialog>` taking `{ triggerLabel, title, description, isPending, onConfirm }`.
2. `<ProviderCardHeader>` for the icon tile plus title plus description block.
3. `<ConnectionStatusRow>` for the "connected with badge / not connected" left column plus actions right column layout.
4. `<CallbackURLNotice>` from section 4.

**Also recommended:** collapse `/settings/github-app` and `/settings/gitlab-app` into a single `/settings/git-providers` route rendering all three cards, with the two existing routes kept as redirects. Three sidebar entries for three providers gets noisy at four.

## Sequencing

Each step is a complete, coherent PR.

1. **Groundwork, no user-visible change.** Dedupe `pendingState`; rename `githubAppBaseURL` to `controlPlaneBaseURL`; fix the GitLab callback URI mismatch bug; add the Bitbucket branch to `verifyGitPushWebhookAuth` behind a table-driven test; add provider-aware push-payload parsing with fixtures for all three providers.
2. **`internal/gitprovider` plus the unified repo routes.** New package, GitHub and GitLab adapters, `/api/v1/git-providers/{kind}/...` routes, old routes aliased.
3. **GitHub Enterprise Server.** Migration, `APIBaseURL`, `instanceURL` threaded through the client, form action and install redirect derived from the instance, manual-connect field, reachability preflight, frontend instance URL field.
4. **Manifest preview and confirm.** Preview endpoint, `name`/`org` parameters, `GitHubAppManifestPreviewDialog`, the four extracted frontend primitives, `<CallbackURLNotice>` rolled into the GitLab card, `/settings/git-providers` route.
5. **Bitbucket Cloud.** `internal/bitbucketapp`, connection table and migration, connect/callback/disconnect/status handlers, `gitprovider.Source` adapter, connection card, queries and types, webhook registration on use-as-source.

Steps 3, 4, and 5 are independent of each other once 1 and 2 land, so they can run in parallel across sessions against a frozen contract.

## Decisions recorded elsewhere

This is a design note, not an ADR. Two decisions here are ADR-shaped and should get one at the next free number: "post-auth-only git provider abstraction, no unified connect handshake," and "single instance URL for self-hosted providers, no internal/external split."

## Known bugs found while researching this design

1. `web/src/components/GitLabAppConnectionCard.tsx` tells the operator to register redirect URI `${window.location.origin}/api/v1/gitlab-app/callback`, but the backend builds its redirect URI from `IngressSettings.PrimaryDomain`. Viewing the dashboard over an IP, tunnel, or secondary hostname yields a mismatched redirect URI and OAuth fails.
2. `internal/api/github_app.go`'s `githubAppRegistrationState` and `internal/api/pending_state.go`'s `pendingState` are the same type implemented twice.
