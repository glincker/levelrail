# Git integrations: GitHub, GitLab, Bitbucket, and preview environments

Connect a git provider once at the control-plane level, then point any number of apps at repos it can see.

**Relevant packages:**

- Backend: `internal/api/github_app*.go`, `gitlab_app*.go`, `bitbucket_app*.go`, `git_webhook.go`, `webhook_deliveries.go`, `preview_environments*.go`
- Webhooks: `internal/webhook`

## Provider connections vs. git sources

### What's the difference?

An **app's git source** (`GET/PUT/DELETE /api/v1/apps/{name}/git-source`) is a per-app record:
- One repo URL
- One branch
- One build config
- One webhook secret

A **provider connection** (GitHub App, GitLab OAuth Application, Bitbucket OAuth consumer) is a control-plane-wide credential that can see many repos across many orgs or workspaces.

### Why separate them

Keeping them separate means:
- Connect GitHub once
- Wire up ten apps against ten different repos it can see
- Each has its own webhook secret and independent enable/disable state
- No re-pasting personal tokens into every app's git source by hand

### Provider differences

The three providers are not equivalent, and the code reflects that:

| Provider | Auth method | Token lifetime | PR comments/statuses |
| --- | --- | --- | --- |
| **GitHub** | Real GitHub App with manifest flow | Short-lived per API call | Yes |
| **GitLab** | Hand-registered OAuth Application | Long-lived | No |
| **Bitbucket** | Hand-registered OAuth consumer | Long-lived | No |

GitHub's manifest flow creates the App automatically on GitHub's side. GitLab and Bitbucket require manual App/consumer creation in their own settings first, then pasting credentials in.

### Checking capabilities

`GET /api/v1/git-providers` answers "what can I do with each provider?" in one round trip:
- Connected?
- Can list branches?
- Can register a webhook?
- Can authenticate a clone?

The dashboard's repo picker (`web/src/components/GitRepoSourcePicker.tsx`) uses this instead of calling three separate status endpoints. This avoids hitting an error boundary for non-root, deploy-scoped users opening the "create app from git" wizard.

## Connecting a provider (dashboard only, on purpose)

Every provider's connect flow ends with a real browser redirect to github.com, your GitLab instance, or bitbucket.org. It cannot be driven by a script or CLI call. This is deliberate, not a gap.

**Why dashboard-only:**
- GitHub's manifest flow requires an actual browser form POST
- GitLab's and Bitbucket's OAuth2 flows require operator approval in their own browser session

### GitHub App

Navigate to `/settings/github-app`.

**Automated (manifest flow):**
1. Start: `GET /api/v1/github-app/register/start`
2. Preview: `GET /api/v1/github-app/register/preview` (see App name, permissions, webhook URL before your browser leaves)
3. Your browser redirects to github.com to create the App
4. Credentials returned automatically

**Manual (hand-created App on github.com):**

Create the App at `github.com/settings/apps` yourself, then connect via `PUT /api/v1/github-app/manual`:
- Paste: `app_id`, `client_id`, `client_secret`, `webhook_secret`, private key PEM

Manual exists for control planes with no publicly reachable primary domain yet (manifest flow needs a real callback URL).

### GitLab App

Navigate to `/settings/gitlab-app`.

1. Paste your OAuth Application's credentials via `PUT /api/v1/gitlab-app`:
   - `instance_url`
   - `client_id`
   - `client_secret`
2. Authorize via `GET /api/v1/gitlab-app/connect` (redirects to your instance's `/oauth/authorize`)

### Bitbucket

Navigate to `/settings/bitbucket-app`.

1. Paste your OAuth consumer's credentials via `PUT /api/v1/bitbucket-app`:
   - `key`
   - `secret`
2. Authorize the same way (`GET /api/v1/bitbucket-app/connect`)

### Prerequisites (all three)

**Master key:** All three need a master key configured on the control plane (the same one envelope-encrypts every secret). Routes that need it return `501` when absent, rather than crashing.

**GitHub's manifest flow:** Additionally needs a primary domain set in ingress settings first. GitHub needs a real, reachable callback URL. A `409` explains exactly that if you try before setting one.

### Disconnecting

`DELETE /api/v1/github-app`, `/gitlab-app`, or `/bitbucket-app` deletes the connection row and secrets locally only.

It does not reach out to revoke anything or delete the App/Application/consumer on the provider's own side. To remove the App from GitHub, delete it from `github.com/settings/apps` yourself.

## Browsing and using repos from the CLI

Once a provider is connected (dashboard only), the CLI drives direct HTTP for everything else: listing repos, listing branches, and connecting a repo as an app's git source (which also registers the webhook in the same call).

### Quick reference

**GitHub:**
```bash
levelrail-cli github-app repos
levelrail-cli github-app branches <owner> <repo>
levelrail-cli github-app use-as-source <owner> <repo> --app-name my-app --branch main
```

**GitLab:**
```bash
levelrail-cli gitlab-app projects
levelrail-cli gitlab-app branches <project-id>
levelrail-cli gitlab-app use-as-source <project-id> --app-name my-app
```

**Bitbucket:**
```bash
levelrail-cli bitbucket-app repos
levelrail-cli bitbucket-app branches <workspace> <repo-slug>
levelrail-cli bitbucket-app use-as-source <workspace> <repo-slug> --app-name my-app
```

### Example: connecting a GitHub repo end to end

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
webhook_url:         /api/v1/webhooks/github/storefront
webhook_registered:  true
```

### Optional flags for use-as-source

`use-as-source` accepts the same optional build configuration as `PUT /api/v1/apps/{name}/git-source`:

- `--branch`: defaults to the repo's default branch
- `--build-type`: `dockerfile`, `railpack`, or `static` (default: `dockerfile`)
- `--build-path`: path to the build file within the repo

GitLab uses numeric project IDs instead of owner/repo pairs. So `gitlab-app branches` and `use-as-source` take one positional argument instead of two.

### Graceful degradation

**GitHub:** `use-as-source` degrades gracefully if the installation predates `repository_hooks:write`. It connects the repo and reports `webhook_registered: false` with a `webhook_error` explaining the permission gap, rather than failing entirely.

**GitLab & Bitbucket:** `use-as-source` fails the whole request (`502`) if webhook registration fails, since neither has a partial-success fallback.

### No CLI for git-providers status

There is no `git-providers` CLI command. `GET /api/v1/git-providers` exists to feed the dashboard's aggregated repo-picker UI in one call. The CLI covers all operator use cases via the three provider-specific commands above.

## The webhook receiver and delivery history

### Single webhook endpoint for all providers

Push and pull-request webhooks from all three providers land on the same URL:

```
POST /api/v1/webhooks/github/{name}
```

The path says "github" for backward compatibility with already-configured GitHub webhooks. GitLab and Bitbucket register at this exact same path. The provider is detected from headers alone (`X-Gitlab-Event`, `X-GitHub-Event`, or `X-Event-Key`), not the URL.

### Authentication

This route is deliberately unauthenticated in the `requireAbility` sense. No provider can present a session or API token. Instead, the per-app secret (generated when the git source was connected) is checked against the request's signature.

| Provider | Signature method |
| --- | --- |
| GitHub | HMAC-SHA256, header: `X-Hub-Signature-256`, format: `sha256=<hex>` |
| Bitbucket | HMAC-SHA256, header: `X-Hub-Signature` (older, unsuffixed), format: `sha256=<hex>` |
| GitLab | Secret sent verbatim, header: `X-Gitlab-Token`, checked with constant-time comparison |

### Event routing

**Push events:** Accepted (`200`) but ignored if the branch is not the git source's configured target branch. Rejected branches return early, not processed.

**Pull-request events:** Routed separately from pushes.
- `opened` / `synchronize`: Deploys or redeploys a preview environment (gated on `PreviewEnabled`)
- `closed`: Tears down the preview, regardless of merge status

### Delivery history and replay

Every inbound request is recorded, verified or not, whether it matched a git source or not. Bad signatures, unconnected sources, and deploy failures all show up, not just successful deploys.

List deliveries:
```bash
levelrail-cli apps webhook-deliveries list storefront
ID                    PROVIDER  EVENT        SIGNATURE  MATCHED  STATUS  RECEIVED
whd_9f2a...           github    push         true       true     200     2026-09-10T14:02:11Z
whd_8e11...           github    push         false      true     401     2026-09-10T13:58:03Z
```

Replay a failed delivery once the issue is fixed:
```bash
levelrail-cli apps webhook-deliveries replay storefront whd_8e11...
```

Replay re-runs the exact same logic (`processGitPushWebhookPayload`) against the stored payload and header fields. This can trigger a real build and deploy. It does not re-verify the original signature (the caller already authenticated at `AbilityDeploy` tier). It does not write a second delivery row (the replay is already in the audit log).

### Dashboard

Each app's **Source** tab (`/apps/{name}/source`) shows a "Recent webhook deliveries" panel, alongside the git source card and preview environments card.

## Preview environments per pull request

Opt-in, per app, off by default. Once enabled, opening a pull request against the connected repo's target branch deploys an independent copy of the app under `<app-name>-pr-<number>`.

### Lifecycle

- **New PR:** Deploys a preview copy automatically
- **New commits on PR:** Redeploys in place
- **PR closed or merged:** Tears down automatically

### Multi-service previews

A single app with a persisted multi-service `services:` map fans a preview out the same way `POST /api/v1/apps/{name}/deploy-spec` does for a real deploy. A preview is never a second, drifted deploy path.

### Enable/disable

**CLI:**
```bash
levelrail-cli apps previews enable storefront
levelrail-cli apps previews disable storefront
```

**Dashboard:** The **Enabled** toggle on the app's Source tab (next to the git source card).

### Domains

When a control-plane primary domain is configured, the preview gets `pr-<number>.<app-name>.<primary-domain>`.

A domain collision doesn't fail the preview. It just deploys without a domain. The `status_reason` field explains why.

For multi-service previews, only one fanned-out service (the one that carried a domain in production) claims the preview's subdomain. The rest are internal-network-only (same as a worker or database service in production).

```bash
$ levelrail-cli apps previews list storefront
PR  PREVIEW APP           BRANCH        STATUS  DOMAIN                              UPDATED               STALE  EPHEMERAL DATABASES
42  storefront-pr-42      feat/cart-ux  active  pr-42.storefront.example.com        2026-09-11T09:14:22Z  no     -
```

### PR comments and commit statuses

GitHub only. A second, independent toggle.

**Enable/disable:**
```bash
levelrail-cli apps previews pr-status enable storefront
levelrail-cli apps previews pr-status disable storefront
```

**Behavior:** When on, every preview deploy posts a `levelrail/preview` commit status on the PR's head commit:
- `pending`: Building
- `success`: Live with preview URL
- `failure`: Deploy failed (truncated to GitHub's 140-character limit)

A successful deploy or teardown also posts a PR comment.

**Implementation:** Uses the same GitHub App installation token as other repo/branch calls. For GitLab or Bitbucket sources, or for GitHub repos predating the App having a usable installation, this is a no-op. It's silently skipped and logged, never failing the preview deploy.

### Per-preview env overrides

By default a preview inherits the parent app's env vars wholesale (`Env`, `SecretEnv`, and `VaultEnv` all copy unchanged).

Declare a preview-specific value for one key on the parent app, and every preview created afterward gets that value, while other env vars still inherit normally.

**CLI:**
```bash
levelrail-cli apps preview-env set storefront DATABASE_URL --value postgres://preview-only/db
levelrail-cli apps preview-env clear storefront DATABASE_URL
```

**Dashboard:** **Preview env overrides** card on the app's Environment tab

**Behavior:**
- Overrides take effect the next time a preview is created (new PR opened or commits pushed)
- Never touch the parent app's own running deploy
- Parent redeploys never wipe them
- Single-service preview path only (multi-service `services:` fan-out does not support them yet)

### Manual teardown and the TTL sweep

**Manual teardown:**
```bash
levelrail-cli apps previews teardown storefront 42
```

This uses the same deletion path (`teardownPreviewRecord`) the automatic pull-request-closed webhook uses. Use it for stuck builds or to retry after a partially-failed automatic teardown.

Also available: "Tear down" button next to each preview row in the dashboard.

**TTL sweep (automatic fallback):**

The TTL sweep handles the failure mode that `webhook-deliveries` is designed to expose: a pull-request-closed delivery that never arrived.

Any preview whose last update is older than `APP_PREVIEW_TTL` (Go duration string, default 7 days) gets torn down automatically by a background loop.

Trigger it immediately instead of waiting:
```bash
levelrail-cli apps previews sweep
```

Or click the "Sweep stale previews" button (appears once at least one preview is actually stale).

**Stale detection:** `GET /api/v1/apps/{name}/previews` returns a `stale` field on each preview indicating whether the next sweep tick would catch it. Check this ahead of time to see what's coming.

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
| `PUT` | `/api/v1/apps/{name}/preview-env/{key}` | `write` |
| `DELETE` | `/api/v1/apps/{name}/preview-env/{key}` | `write` |

### Permission tiers

**`root` tier:** `register/start`, `register/preview`, `callback` (all providers), `installed`

These read or write credential material itself (same tier as `PUT /api/v1/settings/ingress`).

**`read:sensitive` / `write:sensitive` tier:** Repo/branch/project listing and use-as-source endpoints

These disclose real private-repo names but never the connection's own credentials. They match how `GET .../backups/{id}/download` is already gated.

## CLI

### Provider connections (view repos/branches only)

**GitHub:**
```bash
levelrail-cli github-app repos [flags]
levelrail-cli github-app branches <owner> <repo> [flags]
levelrail-cli github-app use-as-source <owner> <repo> --app-name NAME [--branch BRANCH] [--build-type TYPE] [--build-path PATH]
```

**GitLab:**
```bash
levelrail-cli gitlab-app projects [flags]
levelrail-cli gitlab-app branches <project-id> [flags]
levelrail-cli gitlab-app use-as-source <project-id> --app-name NAME [--branch BRANCH] [--build-type TYPE] [--build-path PATH]
```

**Bitbucket:**
```bash
levelrail-cli bitbucket-app repos [flags]
levelrail-cli bitbucket-app branches <workspace> <repo-slug> [flags]
levelrail-cli bitbucket-app use-as-source <workspace> <repo-slug> --app-name NAME [--branch BRANCH] [--build-type TYPE] [--build-path PATH]
```

### Webhook deliveries

```bash
levelrail-cli apps webhook-deliveries list <app-name> [--limit N] [--before RFC3339]
levelrail-cli apps webhook-deliveries replay <app-name> <delivery-id>
```

### Preview environments

```bash
levelrail-cli apps previews list <app-name>
levelrail-cli apps previews enable <app-name>
levelrail-cli apps previews disable <app-name>
levelrail-cli apps previews pr-status enable <app-name>
levelrail-cli apps previews pr-status disable <app-name>
levelrail-cli apps previews teardown <app-name> <pr-number>
levelrail-cli apps previews sweep
```

### Preview env overrides

```bash
levelrail-cli apps preview-env set <app-name> <key> --value VALUE
levelrail-cli apps preview-env clear <app-name> <key>
```

::: warning
Connecting a provider is dashboard-only. `github-app`/`gitlab-app`/`bitbucket-app` with no subcommand only prints usage. This is intentional: it requires a real browser redirect through the provider's manifest or OAuth flow, not something a scriptable client can drive.
:::

## Not built yet (deliberate follow-ups)

### PR comments and statuses

- **No PR comments or commit statuses for GitLab or Bitbucket.** Only GitHub posts them (`preview_environments_github.go`). A GitLab or Bitbucket source with `post_pr_comments` enabled has nothing to post to yet.

### Deploy token reuse

- **No deploy token reuse for private GitLab or Bitbucket repos.** `use-as-source` connects the repo and registers the webhook, but doesn't carry the short-lived OAuth token forward as the git source's deploy token. A private project only builds once an operator separately pastes a personal access token into the app's git source card. GitHub's `use-as-source` doesn't have this gap (installation token is minted fresh on every call).

### Authenticated clones

- **No authenticated clone path for GitLab or Bitbucket.** `GET /api/v1/git-providers`'s `can_auth_clone` is hard-coded `false` for both. Only GitHub's installation token doubles as clone credentials.

- **GitHub Enterprise Server clone auth assumes github.com.** A connection to a GHES instance is fully supported for the App itself, but the `isGitHubHTTPSRepoURL` check behind the authenticated-clone path only recognizes `github.com` URLs. This is a separately tracked gap.

### Revocation

- **No revoke-on-disconnect.** Disconnecting any of the three providers deletes the local connection row and secrets only. It never calls the provider to revoke the token, uninstall the App, or delete the OAuth Application/consumer on its own side.
