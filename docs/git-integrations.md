# Git integrations: GitHub, GitLab, Bitbucket, and preview environments

Connect a git provider once at the control-plane level, then point any
number of apps at repos it can see. Packages: `internal/api/github_app*.go`,
`gitlab_app*.go`, `bitbucket_app*.go`, `internal/api/git_webhook.go`,
`internal/api/webhook_deliveries.go`, `internal/api/preview_environments*.go`,
`internal/webhook`.

## Why a provider connection is separate from a git source

An app's git source (`GET/PUT/DELETE /api/v1/apps/{name}/git-source`,
`docs/` elsewhere) is a per-app record: one repo URL, one branch, one
build config, one webhook secret. A provider connection (GitHub App,
GitLab OAuth Application, Bitbucket OAuth consumer) is a control-plane-wide
credential that can see many repos across many orgs or workspaces. Keeping
them separate means connecting GitHub once and then wiring up ten apps
against ten different repos it can see, each with its own webhook secret
and its own independent enable/disable state, rather than re-pasting a
personal access token into every app's git source by hand.

The three providers are not equivalent in what they offer, and the code
does not pretend otherwise:

- **GitHub** uses a real GitHub App: a manifest-based registration flow
  that creates the App on GitHub's side and hands back credentials
  automatically, short-lived installation tokens minted per API call
  instead of a long-lived personal token, and the only provider that can
  post PR comments and commit statuses today (`preview_environments_github.go`).
- **GitLab** and **Bitbucket** use a hand-registered OAuth
  Application/consumer: the operator creates it in their own GitLab or
  Bitbucket instance's settings first, then pastes the resulting
  client ID/secret (GitLab) or key/secret (Bitbucket) in. Neither has a
  GitHub-style manifest API that could create the Application
  automatically, so this step is unavoidably manual.

`GET /api/v1/git-providers` is the one call that answers "what can I do
with each provider right now" in a single round trip: connected, whether
it can list branches, whether it can register a webhook, whether it can
authenticate a clone. The dashboard's repo picker
(`web/src/components/GitRepoSourcePicker.tsx`) reads this instead of
calling the three providers' own status endpoints separately, because
those three sit at `AbilityRoot` (see below) and a non-root, deploy-scoped
user opening the "create app from git" wizard would otherwise hit an
error boundary before ever seeing a repo list.

## Connecting a provider (dashboard only, on purpose)

Every provider's connect flow ends in a real browser redirect to
github.com, your GitLab instance, or bitbucket.org, so it cannot be driven
by a script or a CLI call. This is deliberate, not a gap: GitHub's
manifest flow requires an actual browser form POST
(`writeGitHubAppManifestForm`, `github_app_register.go`), and GitLab's and
Bitbucket's OAuth2 authorization-code flows require the operator to
approve the request in their own browser session. Each provider gets its
own settings page:

- `/settings/github-app`: register via manifest (the default,
  `GET /api/v1/github-app/register/start`, with a preview step at
  `GET /api/v1/github-app/register/preview` so you see the App's name,
  permissions, and webhook URL before your browser leaves the page), or
  connect manually (`PUT /api/v1/github-app/manual`) with an App you
  created by hand on github.com, pasting in `app_id`, `client_id`,
  `client_secret`, `webhook_secret`, and the private key PEM yourself.
  Manual connect exists for a control plane with no publicly reachable
  primary domain yet, since the manifest flow needs one for its callback
  URL.
- `/settings/gitlab-app`: paste in an OAuth Application's
  `instance_url`/`client_id`/`client_secret` (`PUT /api/v1/gitlab-app`),
  then authorize it (`GET /api/v1/gitlab-app/connect`, a plain redirect to
  the instance's own `/oauth/authorize`).
- `/settings/bitbucket-app`: paste in an OAuth consumer's `key`/`secret`
  (`PUT /api/v1/bitbucket-app`), then authorize the same way
  (`GET /api/v1/bitbucket-app/connect`).

All three need a master key configured on the control plane (the same one
envelope-encrypts every secret): every route that needs it returns `501`
when it's absent, rather than crashing. GitHub's automated flow
additionally needs a primary domain set in ingress settings first, since
GitHub needs a real, reachable callback URL to redirect to; a `409`
explains exactly that if you try before setting one.

Disconnecting (`DELETE /api/v1/github-app`, `/gitlab-app`, `/bitbucket-app`)
is local only: it deletes the connection row and the stored secrets on
this control plane, but does not reach out to revoke anything or delete
the App/Application/consumer on the provider's own side. If you want the
App itself gone from GitHub, delete it from
`github.com/settings/apps` yourself.

## Browsing and using repos from the CLI

Once a provider is connected (dashboard only, above), everything past
that point is real HTTP the CLI drives directly: listing repos, listing a
repo's branches, and connecting a repo as an app's git source, which also
registers the push webhook in the same call.

```bash
levelrail-cli github-app repos
levelrail-cli github-app branches <owner> <repo>
levelrail-cli github-app use-as-source <owner> <repo> --app-name my-app --branch main

levelrail-cli gitlab-app projects
levelrail-cli gitlab-app branches <project-id>
levelrail-cli gitlab-app use-as-source <project-id> --app-name my-app

levelrail-cli bitbucket-app repos
levelrail-cli bitbucket-app branches <workspace> <repo-slug>
levelrail-cli bitbucket-app use-as-source <workspace> <repo-slug> --app-name my-app
```

Example: connecting a GitHub repo end to end once the App is installed.

```bash
$ levelrail-cli github-app repos
FULL_NAME            PRIVATE  DEFAULT_BRANCH
acme/storefront      true     main
acme/storefront-api   true     main

$ levelrail-cli github-app use-as-source acme storefront --app-name storefront --branch main
app_name:            storefront
repo_url:            https://github.com/acme/storefront.git
branch:               main
build_type:          dockerfile
webhook_url:          /api/v1/webhooks/github/storefront
webhook_registered:  true
```

`use-as-source` accepts the same optional build configuration as
`PUT /api/v1/apps/{name}/git-source` directly: `--branch` (defaults to the
repo's own default branch), `--build-type` (`dockerfile`, `railpack`, or
`static`; defaults to `dockerfile`), and `--build-path`. GitLab's project
is addressed by its numeric ID rather than an owner/repo pair, so
`gitlab-app branches`/`use-as-source` take one positional argument instead
of two.

GitHub's `use-as-source` degrades gracefully if the installation predates
`repository_hooks:write`: it still connects the repo as the git source and
reports `webhook_registered: false` with a `webhook_error` explaining the
permission gap, rather than failing the whole request. GitLab's and
Bitbucket's own `use-as-source` calls do fail the whole request (`502`) if
webhook registration itself fails, since neither has an equivalent
partial-success shape to fall back to.

There is no `git-providers` CLI command: `GET /api/v1/git-providers` exists
specifically to feed the dashboard's aggregated repo-picker UI in one call
and has no standalone operator use the three `<provider>-app repos/projects`
commands above don't already cover directly.

## The webhook receiver and delivery history

Every connected provider's push and pull-request webhooks land on the
same URL, regardless of provider: `POST /api/v1/webhooks/github/{name}`.
The path still says "github" for backward compatibility with
already-configured GitHub webhooks; GitLab's and Bitbucket's own webhook
registration point at this exact same path. Which provider actually sent
a request is detected from headers alone (`X-Gitlab-Event`,
`X-GitHub-Event`, or `X-Event-Key`), not the URL.

This route is deliberately unauthenticated in the `requireAbility` sense:
none of the three providers can present a session or API token. Its own
per-app secret, generated when the git source was connected and checked
against the request's signature, is what stands in for auth:

- GitHub and Bitbucket both sign the raw body with HMAC-SHA256
  (`X-Hub-Signature-256` for GitHub, the older unsuffixed
  `X-Hub-Signature` for Bitbucket, same `sha256=<hex>` wire format).
- GitLab instead sends the configured secret back verbatim
  (`X-Gitlab-Token`), checked with a constant-time comparison.

A push to any branch other than the git source's configured target branch
is accepted (`200`) but ignored, not rejected. A pull-request event is
routed separately from a push: `opened`/`synchronize` deploys or
redeploys a preview environment (below, gated on `PreviewEnabled`),
`closed` tears one down, regardless of whether it merged.

Every inbound request is recorded, verified or not, whether it matched a
connected git source or not: bad signature, no git source connected, and
a downstream deploy failure are all visible, not just successful
deploys.

```bash
$ levelrail-cli apps webhook-deliveries list storefront
ID                    PROVIDER  EVENT        SIGNATURE  MATCHED  STATUS  RECEIVED
whd_9f2a...           github    push         true       true     200     2026-09-10T14:02:11Z
whd_8e11...           github    push         false      true     401     2026-09-10T13:58:03Z
```

A delivery can be replayed once whatever was wrong (a rotated secret, a
downstream bug that's since been fixed) is corrected:

```bash
levelrail-cli apps webhook-deliveries replay storefront whd_8e11...
```

Replay re-runs the exact same processing logic
(`processGitPushWebhookPayload`) against the stored payload and header
fields, which can trigger a real build and deploy: it does not re-verify
the original signature (the caller already authenticated at the
`AbilityDeploy` tier to reach this route), and it does not write a second
delivery row, since the replay itself is already visible in the audit log
every `AbilityDeploy` call produces. In the dashboard, this lives on each
app's **Source** tab (`/apps/{name}/source`) as the "Recent webhook
deliveries" panel, alongside the git source card and the preview
environments card.

## Preview environments per pull request

Opt-in, per app, off by default (`PreviewEnabled` on the git source
record). Once on, opening a pull request against the connected repo's
target branch deploys an independent copy of the app under
`<app-name>-pr-<number>`; pushing new commits to that PR redeploys it in
place; closing or merging the PR tears it down automatically. A single
app with a persisted multi-service `services:` map fans a preview out the
same way `POST /api/v1/apps/{name}/deploy-spec` does for a real deploy,
so a preview is never a second, drifted deploy path.

Turn it on from either surface:

```bash
levelrail-cli apps previews enable storefront
levelrail-cli apps previews disable storefront
```

or the **Enabled** toggle on the app's Source tab (next to the git source
card). A domain, when a control-plane primary domain is configured, is
`pr-<number>.<app-name>.<primary-domain>`; a domain collision doesn't fail
the preview, it just deploys without one (`status_reason` explains why).
For a multi-service preview, only one fanned-out service (the one that
carried a domain in production) ever claims the preview's subdomain; the
rest are internal-network-only, the same visibility a worker or database
service already has in production.

```bash
$ levelrail-cli apps previews list storefront
PR  PREVIEW APP           BRANCH        STATUS  DOMAIN                              UPDATED               STALE  EPHEMERAL DATABASES
42  storefront-pr-42      feat/cart-ux  active  pr-42.storefront.example.com        2026-09-11T09:14:22Z  no     -
```

### PR comments and commit statuses

A second, independent toggle, GitHub only:

```bash
levelrail-cli apps previews pr-status enable storefront
levelrail-cli apps previews pr-status disable storefront
```

When on, every preview deploy posts a `levelrail/preview` commit status on
the PR's head commit (`pending` while building, `success` with the
preview URL once live, `failure` with the error if the deploy failed,
truncated to GitHub's own 140-character description limit), and a
successful deploy or a teardown also posts a PR comment. This uses the
same GitHub App installation token every repo/branch call uses; it is a
no-op for a GitLab or Bitbucket source, or for a GitHub repo that predates
the App having a usable installation, silently skipped and logged, never
failing the preview deploy it would have annotated.

### Manual teardown and the TTL sweep

```bash
levelrail-cli apps previews teardown storefront 42
```

is the same deletion path (`teardownPreviewRecord`) the automatic
pull-request-closed webhook uses, for a stuck build or a retry after a
partially-failed automatic teardown. It's also the "Tear down" button next
to each preview row in the dashboard.

The TTL sweep is the fallback for exactly the failure mode
`webhook-deliveries` exists to make visible: a pull-request-closed
delivery that never arrived. Any preview whose last update is older than
`APP_PREVIEW_TTL` (a Go duration string, default 7 days) gets torn down by
a background loop that runs automatically; `apps previews sweep` (or the
"Sweep stale previews" button, which only appears once at least one
preview actually is stale) triggers it immediately instead of waiting for
the next tick:

```bash
levelrail-cli apps previews sweep
```

`GET /api/v1/apps/{name}/previews`'s own `stale` field on each preview
reports whether the next sweep tick would catch it right now, computed
from the same TTL, so you can see it coming before it happens.

## API reference

| Method | Path | Ability |
| --- | --- | --- |
| `GET` | `/api/v1/git-providers` | `read:sensitive` |
| `GET` | `/api/v1/github-app` | `root` |
| `DELETE` | `/api/v1/github-app` | `root` |
| `PUT` | `/api/v1/github-app/manual` | `root` |
| `GET` | `/api/v1/github-app/register/preview` | `root` |
| `GET` | `/api/v1/github-app/register/start` | `root` |
| `GET` | `/api/v1/github-app/callback` | `root` |
| `GET` | `/api/v1/github-app/installed` | `root` |
| `GET` | `/api/v1/github-app/repos` | `read:sensitive` |
| `GET` | `/api/v1/github-app/repos/{owner}/{repo}/branches` | `read:sensitive` |
| `POST` | `/api/v1/github-app/repos/{owner}/{repo}/use-as-source` | `write:sensitive` |
| `GET` | `/api/v1/gitlab-app` | `root` |
| `PUT` | `/api/v1/gitlab-app` | `root` |
| `DELETE` | `/api/v1/gitlab-app` | `root` |
| `GET` | `/api/v1/gitlab-app/connect` | `root` |
| `GET` | `/api/v1/gitlab-app/callback` | `root` |
| `GET` | `/api/v1/gitlab-app/projects` | `read:sensitive` |
| `GET` | `/api/v1/gitlab-app/projects/{id}/branches` | `read:sensitive` |
| `POST` | `/api/v1/gitlab-app/projects/{id}/use-as-source` | `write:sensitive` |
| `GET` | `/api/v1/bitbucket-app` | `root` |
| `PUT` | `/api/v1/bitbucket-app` | `root` |
| `DELETE` | `/api/v1/bitbucket-app` | `root` |
| `GET` | `/api/v1/bitbucket-app/connect` | `root` |
| `GET` | `/api/v1/bitbucket-app/callback` | `root` |
| `GET` | `/api/v1/bitbucket-app/repos` | `read:sensitive` |
| `GET` | `/api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/branches` | `read:sensitive` |
| `POST` | `/api/v1/bitbucket-app/repos/{workspace}/{repoSlug}/use-as-source` | `write:sensitive` |
| `POST` | `/api/v1/webhooks/github/{name}` | none (HMAC/token verified) |
| `GET` | `/api/v1/apps/{name}/webhook-deliveries` | `read` |
| `POST` | `/api/v1/apps/{name}/webhook-deliveries/{id}/replay` | `deploy` |
| `PUT` | `/api/v1/apps/{name}/preview-settings` | `write:sensitive` |
| `GET` | `/api/v1/apps/{name}/previews` | `read` |
| `POST` | `/api/v1/apps/{name}/previews/{number}/teardown` | `deploy` |
| `POST` | `/api/v1/previews/sweep` | `deploy` |

`register/start`, `register/preview`, `callback` (both providers), and
`installed` sit at `root` because they read or write the credential
material itself, the same tier `PUT /api/v1/settings/ingress` uses; the
repo/branch/project listing and use-as-source endpoints sit one tier down
at `read:sensitive`/`write:sensitive`, matching how `GET .../backups/{id}/download`
is already gated, since they disclose real private-repo names but never
the connection's own credentials.

## CLI

```bash
levelrail-cli github-app repos [flags]
levelrail-cli github-app branches <owner> <repo> [flags]
levelrail-cli github-app use-as-source <owner> <repo> --app-name NAME [--branch BRANCH] [--build-type TYPE] [--build-path PATH]

levelrail-cli gitlab-app projects [flags]
levelrail-cli gitlab-app branches <project-id> [flags]
levelrail-cli gitlab-app use-as-source <project-id> --app-name NAME [--branch BRANCH] [--build-type TYPE] [--build-path PATH]

levelrail-cli bitbucket-app repos [flags]
levelrail-cli bitbucket-app branches <workspace> <repo-slug> [flags]
levelrail-cli bitbucket-app use-as-source <workspace> <repo-slug> --app-name NAME [--branch BRANCH] [--build-type TYPE] [--build-path PATH]

levelrail-cli apps webhook-deliveries list <app-name> [--limit N] [--before RFC3339]
levelrail-cli apps webhook-deliveries replay <app-name> <delivery-id>

levelrail-cli apps previews list <app-name>
levelrail-cli apps previews enable <app-name>
levelrail-cli apps previews disable <app-name>
levelrail-cli apps previews pr-status enable <app-name>
levelrail-cli apps previews pr-status disable <app-name>
levelrail-cli apps previews teardown <app-name> <pr-number>
levelrail-cli apps previews sweep
```

There is no CLI command to connect a provider itself (`github-app`/
`gitlab-app`/`bitbucket-app` with no further subcommand only prints usage):
that's the one part of this feature that stays dashboard-only, since it's
a real browser redirect through the provider's own manifest or OAuth
flow, not something a scriptable client can drive.

## Not built yet (deliberate follow-ups)

- **No PR comments or commit statuses for GitLab or Bitbucket.** Only
  GitHub posts them (`preview_environments_github.go`); a GitLab or
  Bitbucket source with `post_pr_comments` on has nothing to actually post
  to yet.
- **No deploy token reuse for a private GitLab or Bitbucket repo.**
  `use-as-source` for both connects the repo and registers the webhook,
  but does not carry the short-lived OAuth token forward as the git
  source's own deploy token: a private project only builds once an
  operator separately pastes a personal access token into the app's git
  source card. GitHub's own `use-as-source` doesn't have this gap, since
  its installation token is minted fresh on every call anyway.
- **No authenticated clone path for GitLab or Bitbucket.**
  `GET /api/v1/git-providers`'s own `can_auth_clone` is hard-coded `false`
  for both; only GitHub's installation token doubles as clone
  credentials.
- **GitHub Enterprise Server clone auth assumes github.com.** A connection
  to a GHES instance is fully supported for the App itself, but the
  isGitHubHTTPSRepoURL check behind the authenticated-clone path only
  recognizes `github.com` URLs, a separately tracked gap.
- **No revoke-on-disconnect.** Disconnecting any of the three providers
  deletes the local connection row and secrets only; it never calls the
  provider to revoke the token, uninstall the App, or delete the OAuth
  Application/consumer on its own side.
