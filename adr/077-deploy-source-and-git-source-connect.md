# 077. Deploy source model and one git source connect path

Status: proposed (needs owner acceptance)

Date: 2026-11-12

## Context

An app changes when something deploys it: a git push, an upload from the CLI, an image deploy, a registry push or a CI job. Only the git path keeps an app current on its own, and today it is easy to end up with an app that looks connected but is not:

- `levelrail-cli deploy <dir>` uploads the folder and builds once. `up` and `apps create` read the origin remote and build from it once. Neither saves a git source, so later pushes deploy nothing.
- Connecting happens in four places that each do a different subset of the work: `PUT /apps/{name}/git-source`, GitHub App use as source, the connection repo picker and the platform importer. Only two of them register a webhook. The importer did not accept ssh repository URLs at all.
- Repository URLs were parsed by several small helpers that disagreed on ssh, scp style and port handling.

## Decision

1. **One parser.** Every repository URL input goes through `internal/gitprovider/reporef`. It accepts https, http, ssh, git and scp style (`git@host:owner/repo`) forms, strips credentials, keeps GitLab subgroups, and returns host, owner, repo, a provider guess and one canonical clone URL (https, or http when the input was plain http; an ssh port is dropped). Stored git sources use the canonical URL unless the operator explicitly chose ssh with a deploy key.
2. **One connect endpoint.** `POST /api/v1/apps/{name}/git-source/connect` takes a repository reference in any form (the CLI fills it from the origin remote when omitted) and does the whole job in one call: parse, pick the provider connection that covers the host, default the branch to the repository default, run build detection, save the source, register the webhook (the GitHub App webhook when it covers the repository, a per-repository hook otherwise), send a ping, and return the deploy source. It is idempotent: running it again heals a half-wired source. The existing PUT stays for scripted edits. Use as source, the repo picker and the importer call the same internal step instead of their own copies.
3. **Deploy source is a read model.** `GET /api/v1/apps/{name}/deploy-source` answers "what changes this app, and is that healthy" for every kind: `git`, `upload`, `image`, `compose`, `registry_push`, `git_push_remote`, `none`. It is computed on read from rows that already exist plus a small health row on the git source reliability record, by a pure evaluator that returns a state (`healthy`, `degraded`, `broken`, `unconnected`) and findings with fix identifiers. Upload and image deploys record the credential-free origin URL, branch and commit they came from, so an upload app built from a checkout can say "built from this repository but not connected".
4. **Self-heal only what Levelrail owns.** The background audit may re-register, re-enable or re-point Levelrail's own hook. It never disables another system's hook; that is a confirmed, audited one-step fix.
5. **Secrets stay on the server.** A connect response carries no webhook secret when registration succeeded. A secret is shown once, only on the manual path where the provider cannot register the hook.
6. **The client never connects silently.** CLI flows may suggest or offer to connect, but a git source is created only by an explicit command, prompt answer or dashboard action.

## Consequences

- One more endpoint and one more resource, in exchange for a single place where "connected" is defined and tested.
- The parser change is behavior preserving for https input. ssh and scp style input that used to be rejected or stored with a misleading port now maps to the canonical https URL.
- Provider calls happen only at connect time and in the existing poller's audit pass. Reads of the deploy source make no network calls.
- Every new route goes through the request audit middleware and the authz matrix, like the existing git source routes.
