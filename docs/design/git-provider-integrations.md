# Git provider integrations: shared abstraction, GHE, Bitbucket, connect-flow UX

**Status:** Proposed. Feeds implementation for Bitbucket support, GitHub Enterprise Server, and connect-flow preview.

**Context:** GitHub App and GitLab OAuth integrations are both shipped and independently typed. Bitbucket is next, and GitHub needs self-hosted (GHES) support. This is the "rule of three" moment to answer the abstraction question intentionally rather than by default.

## 1. Shared Go abstraction: split the answer, do not unify the handshake

**Recommendation: a narrow abstraction. Introduce `internal/gitprovider` covering only the post-authentication repository surface. Keep every auth handshake in its own package and on its own routes. Do not put `Connect` in any interface.**

### Why not the full `GitProvider` interface

The convergence point already exists downstream of auth, not at it. `internal/api/git_sources.go`'s `connectGitSource` is the single place a repository becomes a deploy source. `handleUseGitLabProjectAsSource` already funnels into it, producing the same `store.GitSource` row and webhook receiver as hand-typed sources.

Bitbucket gets that for free without any new interface. An interface spanning connect through webhook creation would re-abstract a path already converged, merely to unify the part that genuinely is not.

### What actually differs between the handshakes

These are structural differences, not cosmetic. Hiding them in an interface would be dishonest:

- **App creation:** GitHub creates the App programmatically (browser POSTs manifest to `settings/apps/new`, GitHub returns one-time code). GitLab and Bitbucket require manual registration. Operator pastes `client_id`/`client_secret`.

- **Browser interaction:** GitHub needs a server-rendered self-submitting form (`writeGitHubAppManifestForm`). GitLab and Bitbucket need a 302 redirect to an authorize URL. Any interface method covering both becomes `func(http.ResponseWriter, *http.Request)`, which is a router, not an abstraction.

- **Completion flow:** GitHub: create App, then install on account/org, yielding `installation_id` verified server-side against `app_id` (`handleGitHubAppInstalled`). GitLab and Bitbucket: configure, then authorize, yielding an access token. Status resources already use separate `installed` and `authorized` booleans for this reason.

- **Token lifecycle:** GitHub mints fresh installation tokens per call via RS256 JWT with stored PEM key. No refresh token or stored access token. GitLab and Bitbucket store access token plus refresh token plus expiry, refresh on demand (`gitlabAccessToken`). Bitbucket Cloud tokens expire in 2 hours, making refresh mandatory.

- **Credential shape:** GitHub stores `app_id`, `client_id`, `installation_id`, `account_login` plus three secrets. GitLab adds `instance_url` plus four secrets. Bitbucket adds a workspace concept.

- **Token scope:** GitHub installation tokens scope to "repos this installation was granted". GitLab and Bitbucket tokens scope to "repos this user is a member of". Same method name, different meaning.

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

`ID string` is the cost: GitHub takes `owner, repo`, GitLab takes `int64`, each adapter parses its own format. That is acceptable because alternatives (`any` ID, three-armed union) violate the no-`any`-in-exported-signatures rule.

**The interface only pays for itself if the HTTP surface unifies.** Collapse repo-facing routes:

- `GET /api/v1/git-providers/{kind}/repos`
- `GET /api/v1/git-providers/{kind}/repos/{id...}/branches`
- `POST /api/v1/git-providers/{kind}/repos/{id...}/use-as-source`

Keep existing routes (`/api/v1/github-app/repos`, `/api/v1/github-app/repos/{owner}/{repo}/branches`, `/api/v1/gitlab-app/projects*`) as thin aliases delegating to the same handlers for backward compatibility.

Connect, callback, disconnect, and status routes stay per-provider under `/api/v1/{github,gitlab,bitbucket}-app`.

Note: GitHub currently has no `use-as-source` and registers no repo webhook (manifest sets `hook_attributes.active: false`). Unifying the route surface makes this gap visible and closable.

### Small dedupes to fold into the same work

- `githubAppRegistrationState` (`internal/api/github_app.go`) and `pendingState` (`internal/api/pending_state.go`) are identical types. Delete the GitHub copy, use `pendingState` for all three providers.

- `verifyGitPushWebhookAuth` (`internal/api/git_webhook.go`) already branches on GitHub's `X-Hub-Signature-256` vs GitLab's `X-Gitlab-Token`. Bitbucket adds `X-Hub-Signature` (sha256 HMAC, no `-256` suffix). Write a table-driven test over the header matrix, including "no known header" failing closed.

- `githubAppBaseURL` is used by GitLab handlers under its GitHub name. Rename to `controlPlaneBaseURL` and move out of `github_app.go`. Same for `errGitHubAppNoPrimaryDomain`.

### What this recommendation does not do

- Does not create a `GitProvider` interface with `Connect`/`Disconnect`.
- Does not merge the three connection tables into one polymorphic table.
- Does not change how any provider stores credentials.

If a fourth provider (Gitea, GitLab-shaped) lands and connect handshakes are still three copies, revisit with real evidence rather than speculating now with two.

## 2. GitHub Enterprise Server: single `instance_url`, plus a reachability preflight

**Recommendation: follow GitLab's single `instance_url` field exactly. Do not adopt Dokploy's external-URL/internal-URL split.**

The split exists for cases where the browser's reachable address differs from the control plane's address on an internal instance. It trades that case for a second field every operator must reason about. It also has a live bug from this complexity (Dokploy/dokploy#3848: self-hosted OAuth callback ECONNREFUSED).

A cheaper mitigation is a preflight: on save, the control plane calls the instance's version endpoint (`/api/v3/meta` for GHES). If it cannot reach it, return a specific, actionable 400, turning silent runtime OAuth failure into an immediate save error. No second field needed.

**Concrete changes:**

**Store**

New migration adding `instance_url TEXT NOT NULL DEFAULT 'https://github.com'` to `github_app_connections`. The default makes every existing row correct with no backfill.

**`internal/githubapp/client.go`**

Add `APIBaseURL(instanceURL string) string`:
- Returns `https://api.github.com` when `instanceURL` is `https://github.com` or empty
- Otherwise returns `strings.TrimRight(instanceURL, "/") + "/api/v3"`

GHES's REST root is `/api/v3` under the instance host, not a separate `api.` hostname.

Thread `instanceURL` as the first argument of every method, mirroring `gitlabapp.Client`. Widen the `GitHubAppClient` interface in `internal/api/github_app.go` to match.

**`internal/githubapp/manifest.go`**

`BuildManifest` needs no signature change. Every URL it emits derives from our own `baseURL`, not GitHub's.

Changes:
- `writeGitHubAppManifestForm`: hardcoded `https://github.com/settings/apps/new` becomes `<instance>/settings/apps/new`, or `<instance>/organizations/<org>/settings/apps/new` for org-owned apps.
- `handleGitHubAppCallback`: hardcoded redirect to `https://github.com/apps/<slug>/installations/new` becomes `<instance>/apps/<slug>/installations/new`.

**Manual connect**

`github_app_manual.go` gains the same `instance_url` field, defaulting to `https://github.com`.

**Out of scope**

No `insecure_skip_verify` or custom CA toggle. GHES instances with private CAs are supported by adding that CA to the host trust store (documented), not by disabling certificate verification for credential-bearing connections.

## 3. Bitbucket: Cloud now, Server not scheduled

**Recommendation: Bitbucket Cloud only. Bitbucket Server / Data Center is explicitly not on the roadmap, and this is not a "fast follow."**

Why not Server/Data Center:

1. Bitbucket Server has no OAuth-consumer equivalent. Integration is via Application Links or HTTP access tokens (paste-a-token flow).

2. **A paste-a-token flow already ships here.** `PUT /api/v1/apps/{name}/git-source` takes a repo URL plus optional deploy token and returns a webhook secret. `handleGitPushWebhook` verifies GitHub-style HMAC. A Data Center user can connect a repo today by pasting the clone URL and HTTP access token, then adding the webhook by hand.

3. Atlassian ended Bitbucket Server sales and support in Feb 2024. Remaining installs are Data Center. Building first-class integration against a shrinking, self-hosted-only, token-paste target is poor use of the next work.

Document point 2 so "no Bitbucket Server support" reads as a documented path, not a gap.

### Minimum viable Bitbucket Cloud, matching the bar GitHub and GitLab already meet

Deliver: status endpoint, connect and disconnect, credential storage via `internal/secrets`, repo listing behind `AbilityReadSensitive`, branch listing, use-as-source landing in `store.GitSource`, automatic push-webhook registration, and webhook receiver that verifies and triggers deploy.

GitLab meets all of it. GitHub currently meets all but webhook registration.

**`internal/bitbucketapp`**

Structured like `internal/gitlabapp`. OAuth code exchange and refresh against `https://bitbucket.org/site/oauth2/access_token`, plus API below. No instance URL parameter (Cloud only, fixed `api.bitbucket.org/2.0`).

**Operator-registered OAuth consumer**

`key` (client id) plus `secret`. Connect dialog must tell operator which permissions to tick on the consumer (Account: read, Repositories: read, Webhooks: read and write). Bitbucket scopes are configured on the consumer, not requested per authorization.

**Token refresh is mandatory**

Bitbucket access tokens live 2 hours. Reuse the `gitlabAccessToken` pattern.

**Repo listing**

`GET /2.0/repositories?role=member`. Pagination is cursor-based via `next` URL in response body, a third shape distinct from existing clients' page-counter loops. Cap follow count like `listPageCap` does.

**Branch listing**

`GET /2.0/repositories/{workspace}/{repo_slug}/refs/branches`.

**Webhook registration**

`POST /2.0/repositories/{workspace}/{repo_slug}/hooks`, event `repo:push`, `active: true`, with secret. If secret cannot be set, fail use-as-source with the same "connected, but webhook registration failed; add manually" 502 that GitLab returns. Never register an unauthenticated hook.

**Push payload adapter**

`internal/webhook.ParsePushEvent` requires top-level `ref` and `after` (GitHub and GitLab both send these). Bitbucket sends `push.changes[].new.name` (bare branch name, not `refs/heads/`) and `push.changes[].new.target.hash`. Add provider-aware parse selected by `X-Event-Key: repo:push`, normalizing to `PushEvent`. Table-driven test with fixtures for all three providers, including Bitbucket branch-delete where `new` is null.

**Webhook auth**

Third branch in `verifyGitPushWebhookAuth` for `X-Hub-Signature`.

**Frontend**

`BitbucketAppConnectionCard.tsx`, `queries/bitbucketApp.ts`, `types/bitbucketApp.ts`, repo picker reusing unified `/api/v1/git-providers/{kind}/repos` route.

**Out of scope for first slice**

Bitbucket Server / Data Center, app passwords as alternative credential, workspace-restriction UI, pull-request preview environments, auto-populating clone deploy token from OAuth token.

## 4. Manifest preview and confirm step

`GitHubAppConnectionCard.tsx` currently redirects directly to `/api/v1/github-app/register/start` with no preview. Everything the operator grants lives in a warning paragraph next to the button.

GitHub's own confirmation page only lets operators edit the App name. Callback URL, webhook URL, permissions, and events must all be correct before redirect. That demands a preview step.

**Backend: `GET /api/v1/github-app/register/preview`**

Requires `AbilityRoot`. Returns JSON:

```json
{
  "instance_url": "", "app_name": "", "homepage_url": "",
  "callback_url": "", "setup_url": "", "webhook_url": "",
  "webhook_active": false, "permissions": {"contents": "read"},
  "events": [], "public": false, "request_oauth_on_install": false,
  "owner": "user", "organization": ""
}
```

Built by calling `githubapp.BuildManifest` with same inputs as `handleStartGitHubAppRegistration`, so preview and real POST cannot drift. The preview endpoint must not call `pendingState.begin()`. State is minted only when the operator confirms and hits `register/start`.

`register/start` gains two optional, validated query parameters:
- `name`: the one field GitHub lets you change
- `org`: org login, changing form action to `<instance>/organizations/<org>/settings/apps/new`

**Frontend: `GitHubAppManifestPreviewDialog`**

Opened by the existing "Add GitHub App" button:

- Editable App name, prefilled from `app_name`.
- Owner selector: personal account or organization. When organization is chosen, show login input.
- Read-only monospace list of four URLs (homepage, callback, setup, webhook), each labelled with what GitHub uses it for.
- Permissions as `contents: read` style rows, plus events list. Add "webhooks are declared but inactive" note when `webhook_active` is false.
- Reachability warning moved from card body.
- Confirm navigates with `?name=&org=`. Cancel does nothing.

**Should this generalize?**

The manifest preview stays GitHub-specific. GitLab and Bitbucket create nothing remotely on the operator's behalf, so there is no surprise to prevent.

One piece does generalize and fixes a live bug: `GitLabAppConnectionCard.tsx`'s `ConfigureDialog` tells operators to register redirect URI `${window.location.origin}/api/v1/gitlab-app/callback`, but the backend builds its redirect URI from `IngressSettings.PrimaryDomain`. Operators viewing the dashboard over an IP, tunnel, or secondary hostname are told to register a URI that will not match, causing OAuth failure.

Extract `<CallbackURLNotice baseURL={status.base_url} path="/api/v1/gitlab-app/callback" />`: render the server-derived URL with a copy button and explicit "set a primary domain first" state. Use in all three cards and inside the GitHub preview dialog. Both status endpoints already return `base_url`.

## 5. Frontend: extract four primitives, keep three cards

**Recommendation: no generic `ProviderConnectionCard`, no `useProviderConnection` hook. Extract four small primitives and let each card keep its own body.**

The two existing cards are 465 and 322 lines. What is actually identical is the chrome, not the logic. State machines genuinely differ:
- GitHub: not-connected/connected/installed with two competing entry actions on one row.
- GitLab: strict not-connected/configured/authorized sequence.
- Bitbucket: matches GitLab but adds workspace concept.

A config-driven generic card would need a discriminated union over three state machines plus per-provider render slots. That is more code than three cards and trends toward the 500-line cap.

Extract these, because they are byte-identical (except strings) and Bitbucket would make a third copy:

1. `<DisconnectProviderDialog>`: takes `{ triggerLabel, title, description, isPending, onConfirm }`.
2. `<ProviderCardHeader>`: icon tile, title, description block.
3. `<ConnectionStatusRow>`: "connected with badge / not connected" left column, actions right column.
4. `<CallbackURLNotice>`: from section 4.

**Also recommended**

Collapse `/settings/github-app` and `/settings/gitlab-app` into a single `/settings/git-providers` route rendering all three cards. Keep the two existing routes as redirects. Three sidebar entries for three providers gets noisy at four.

## Sequencing

Each step is one complete, coherent PR.

1. **Groundwork, no user-visible change.**
   - Dedupe `pendingState`
   - Rename `githubAppBaseURL` to `controlPlaneBaseURL`
   - Fix GitLab callback URI mismatch bug
   - Add Bitbucket branch to `verifyGitPushWebhookAuth` (table-driven test)
   - Add provider-aware push-payload parsing with fixtures for all three providers

2. **`internal/gitprovider` plus unified repo routes**
   - New package, GitHub and GitLab adapters
   - `/api/v1/git-providers/{kind}/...` routes
   - Old routes aliased

3. **GitHub Enterprise Server**
   - Migration, `APIBaseURL`
   - `instanceURL` threaded through client
   - Form action and install redirect derived from instance
   - Manual-connect field, reachability preflight
   - Frontend instance URL field

4. **Manifest preview and confirm**
   - Preview endpoint, `name`/`org` parameters
   - `GitHubAppManifestPreviewDialog`
   - Four extracted frontend primitives
   - `<CallbackURLNotice>` in GitLab card
   - `/settings/git-providers` route

5. **Bitbucket Cloud**
   - `internal/bitbucketapp`
   - Connection table and migration
   - Connect/callback/disconnect/status handlers
   - `gitprovider.Source` adapter
   - Connection card, queries, types
   - Webhook registration on use-as-source

Steps 3, 4, and 5 are independent once 1 and 2 land. They can run in parallel across sessions against a frozen contract.

## Decisions recorded elsewhere

This is a design note, not an ADR. Two decisions here are ADR-shaped and should get one at the next free number: "post-auth-only git provider abstraction, no unified connect handshake," and "single instance URL for self-hosted providers, no internal/external split."

## Known bugs found while researching this design

1. `web/src/components/GitLabAppConnectionCard.tsx` tells the operator to register redirect URI `${window.location.origin}/api/v1/gitlab-app/callback`, but the backend builds its redirect URI from `IngressSettings.PrimaryDomain`. Viewing the dashboard over an IP, tunnel, or secondary hostname yields a mismatched redirect URI and OAuth fails.
2. `internal/api/github_app.go`'s `githubAppRegistrationState` and `internal/api/pending_state.go`'s `pendingState` are the same type implemented twice.
