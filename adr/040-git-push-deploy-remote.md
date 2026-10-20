# ADR 040: A git push deploy remote served in-process

Status: Accepted (beta, off by default, 2026-10-19)

Date: 2026-10-19

## Context

Deploys come from a connected git provider through webhooks. There is no way to
type `git push` at your own server and get a running app, which is the first
thing Dokku users expect and the biggest first-deploy simplicity gap against it.
The platform already has the pieces a push needs: a bounded source cache
(`internal/sourcecache`) that checks out an exact commit by hash with no network
call, and the manual build trigger that records a deploy attempt, builds with
the pipeline and deploys.

The constraints are the project's own: no new listening port, no exec of the
system `git` or any shell (`test/execgate/allowlist.txt` does not grow), no new
network surface that needs host keys, and secrets only stored as hashes.

## Decision

1. **Smart HTTP, served in-process.** Two routes on the existing control-plane
   HTTP server (so on the existing HTTPS ingress path):
   `GET /api/v1/git/{app}.git/info/refs?service=git-receive-pack` and
   `POST /api/v1/git/{app}.git/git-receive-pack`. The protocol is implemented
   with go-git's `plumbing/protocol/packp` (advertisement, update request,
   report-status, side-band-64k) and `plumbing/format/packfile` (writes the
   received pack into a bare repository). go-git's own `transport/server`
   package was read and rejected for the receive side: it updates refs
   with no hook between "objects arrived" and "ref moved", and this feature
   needs to deploy first and move the ref only on success. Only the pieces
   listed above are used; nothing runs a process.
2. **Storage is the source cache.** Pushed objects land in a bare mirror the
   source cache keeps under a synthetic URL (`push://local/{app}`). The deploy
   then checks the commit out by full hash, offline, through the same
   `sourcecache.Checkout` every other deploy uses, and the image is tagged by
   that commit like any manual build, so rollback targets work unchanged. The
   mirror is also what makes the second push incremental (its refs are
   advertised). If the cache evicts or resets it, the next push simply sends
   everything again.
3. **Auth: HTTP basic with a per-app push token.** The user name is ignored, the
   password is the token (`gp_` plus 48 hex characters from `crypto/rand`). Only
   its SHA-256 is stored (`git_push_remotes.token_hash`), compared in constant
   time through `internal/tokenhash`. The token grants one thing: pushing to
   that app. It is not an API token and carries no abilities, so a leaked token
   cannot read, list or manage anything. Creating, rotating (one call, the old
   token stops at once), changing the branch and revoking need
   `AbilityWriteSensitive` on the app and are audited by the ability
   middleware. The token is returned once and never logged or echoed (log lines
   carry the app name and client address only; build output is passed through
   `secretmask`).
4. **Uniform failures and rate limits.** Unknown app, no remote, revoked remote
   and wrong token all answer the same 401, and a missing remote still performs
   one hash compare. Requests are limited per client address and app
   (`APP_GIT_PUSH_RATE_PER_MINUTE`, default 30), and repeated bad tokens back
   off exponentially with the login limiter. Every push outcome writes an audit
   entry and updates the remote's last push (commit and result).
5. **Branch rule.** A push to the configured branch (default `main`, changed
   with `PUT /api/v1/apps/{name}/git-remote`) deploys that exact commit. Any
   other branch, a tag, a delete, or a multi-ref push is rejected with a
   `remote:` message that names the deploy branch. Push-created preview
   environments are not offered: previews are tied to pull requests on a
   provider, which a bare push does not have.
6. **Deploy semantics.** The push is accepted only after the objects are stored,
   the commit and its tree resolve, and the ref still matches what was
   advertised. The build then runs through the manual-build path (deploy
   attempt row with source `git_push`, freeze and one-deploy-per-app checks) and
   its output streams back on the side-band channel, so the client prints it as
   `remote:` lines, including a line with the pushed commit. The branch ref
   moves only when the deploy succeeds. A build failure therefore returns
   `[remote rejected] ... deploy failed`, the previous release keeps running
   (the pipeline never changes the desired state on a failed build), and the
   same `git push` can be repeated unchanged after the fix.
7. **Limits.** Per push body: `APP_GIT_PUSH_MAX_BYTES` (default 256 MiB,
   counted after gzip decoding, answered 413 beyond it). Per app mirror quota:
   `APP_GIT_PUSH_QUOTA_BYTES` (default 1 GiB); an over-quota mirror is reset (it
   is only a cache, the deployed image is unaffected) and the push is asked to
   retry. One push per app at a time (409 otherwise). The wait for a deploy is
   bounded by `APP_GIT_PUSH_TIMEOUT` (default 30 minutes); the build itself keeps
   running past it and stays visible in the dashboard. The upload route is exempt
   from the 16 MiB API body backstop and bounds its own body after auth.
8. **Opt-in beta.** Feature `git-push` in the beta features list, off by default,
   switchable at runtime. While off, the push routes answer 404 and the
   management routes are gated like any other beta feature.
9. **Surfaces together.** API (`/api/v1/apps/{name}/git-remote`), CLI
   (`git remote add|show|rotate|branch|revoke <app>`, which prints the exact
   `git remote add` line) and a card on the app's Source tab with copy, rotate
   and revoke.

## What this does not do

- No git hosting: no web view of the repository, no browsing, no history, no
  pull requests, no `git clone` or `git fetch` (the upload-pack service answers
  403).
- No Git LFS. Large binaries go through the size limit or a registry image.
- No SSH. Tokens in the remote URL are stored by git in the repository's config
  in plain text on the developer machine, like any HTTPS token remote; a
  credential helper keeps them out of the config.
- No push to deploy for compose apps, and no per-branch environments.
- No signature or protected branch policy on pushes: whoever holds the token can
  deploy that app.
- Pack contents are not scanned. A hostile pack inside the size limit can still
  be expensive to unpack or check out; the token holder is already trusted to
  run arbitrary builds, so this adds no new privilege, only a new path to the
  same one.

## Rejected alternatives

- **An SSH server (the Dokku model).** A new listening port, host key
  generation and rotation, `authorized_keys` management and a second
  authentication system, on every control plane, for a beta feature. Rejected
  by default. It can be revisited if key-based pushes are requested, behind a
  separate ADR.
- **`git http-backend` or `git receive-pack` through exec.** The hooks and
  protocol would come for free, but it adds an exec surface that takes
  user-influenced input (repository names, ref names, pack data) and needs the
  `git` binary on the control plane host. Rejected: the exec allowlist stays
  closed.
- **go-git `transport/server` for the receive side.** Its source shows it stores the
  pack and moves refs in one step with no place to run the deploy in between,
  and it reports the unpack and the ref update as one status. Replaced by the
  packp and packfile primitives it is built from.
- **Reusing the general API token with a push ability.** Would put a bearer
  credential with a wider blast radius into `.git/config` files. A dedicated
  per-app credential that can do exactly one thing is safer.
- **Deploy asynchronously and report success on accept.** Simpler, but the
  client would see success for a build that fails a minute later, and a failed
  push could not be retried by repeating the command.
- **A provider-less signed webhook as the first slice.** Kept as the fallback if
  in-process receive-pack had proven unsafe. The prototype and the tests with
  both go-git and the real `git` client (chunked large pushes and the probe
  request included) showed it works, so the webhook path was not built.

## Consequences

- The control plane binary gains a git wire-protocol endpoint, tested against the
  real client in a live test (`internal/api/git_push_live_test.go`, skipped under
  `-short`).
- The source cache directory must be configured (it is on every normal
  install); otherwise the routes answer 501 with a clear message.
- Pushes are bounded by memory and disk the same way other uploads are; the quota
  and body limit are the knobs, and both are environment variables.
