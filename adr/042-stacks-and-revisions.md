# ADR 042: Stacks and revisions for platform as code

Status: Accepted (experimental in v0.8.6)

Date: 2026-10-25

## Context

The apply engine already exists. `internal/iac` parses multi-document YAML
(`version: 1`, nine kinds), validates it against the resource schema, plans
field diffs with a plan hash and applies through the ordinary REST API
(`POST /api/v1/apply/plan`, `POST /api/v1/apply`). The server re-enters its own
handler for each item (`internal/api/iac.go:loopbackDoer`), so every ability,
IAM and per-resource check still applies. The whole surface sits behind
`APP_EXPERIMENTAL=iac`.

What is missing is everything a GitOps loop needs around that engine:

- **No stack object.** "Source" is a 48 character string whose only record is a
  `managed-by` tag on apps. Nothing lists stacks, keeps history or owns
  non-app resources.
- **No history.** An apply leaves only per-request audit rows. There is no
  revision to diff against, roll back to or pin.
- **No concurrency control.** The plan hash check runs once before execution,
  so two applies can interleave for the whole run.
- **Permissions stop at the ability level.** No identity can say "plan and
  apply stack `shop-prod`, nothing else". Prune is gated by `write` plus a
  client flag. CI OIDC mints deploy tokens for one service only.
- **Drift** is "run `diff` on a cron". Nothing on the server watches for it.

## Decision

### 1. A stack is a first-class, server-side object

A stack has a unique name, a project scope, an optional environment scope and
server-side settings: `allow_prune`, `drift_policy`, `pinned_revision` and
`require_approval`. State lives in the control plane store. There is no state
file and the client never holds authoritative state.

A file set may carry one optional `kind: Stack` document:

```yaml
version: 1
kind: Stack
metadata: { name: shop-prod }
spec:
  project: shop        # every document must live in this project
  environment: prod    # optional narrower scope
  prune: false         # a request; the server-side allow_prune must also be on
  drift: warn          # warn | lock
```

A file set without a `Stack` document behaves exactly as today, including
`--source`. No resource kinds are added in v0.8.6. Stack names follow the app
name shape (`^[a-z][a-z0-9-]*$`, at most 48 characters) so a stack name is
always a valid legacy source.

### 2. Revisions are rendered documents, never secrets

Every stack apply records a numbered revision: status, plan hash, plan, the
rendered documents (after `${{ env.X }}` substitution), actor, token id, git
repo, ref and sha, CI run URL, timestamps and per-item results. Numbers start
at 1 and increase by one per stack.

Rendering is safe to persist because secret-looking literals are refused before
substitution (`looksSecret`) and secret values only ever travel in the request
`Secrets` map, by name. The stack handlers never persist `Secrets`, and a test
asserts that no stored row and no log line contains a secret value. Revisions
store names, never values.

Statuses: `pending_approval`, `applying`, `applied`, `partial`, `failed`,
`superseded`. `applied`, `partial`, `failed` and `superseded` are terminal.
Only the newest applied revision is current; applying a newer one marks the old
one `superseded`. A `partial` revision keeps per-item results so the next apply
resumes. There are no cross-resource transactions, and the docs say so.

Every stack apply carries an `Idempotency-Key`, so a retried CI job does not
create two revisions.

### 3. Single ownership through a ledger

The ledger maps `(kind, key)` to the one stack that owns it, with the live id
and the revision that first claimed it. `UNIQUE(kind, key)` makes two stacks
claiming one resource a store error, which the planner reports as a plan error.
A stack may reference a resource another stack owns (read-only use by name);
changing it is a plan error.

Once the ledger exists, prune deletes only what the ledger says this stack
created, never by tag alone. Databases are never pruned, only forgotten
(released from the ledger). A prune above the mass-prune threshold
(`APP_STACK_PRUNE_MAX`, or more than half the stack) needs an explicit
`--allow-mass-prune`. Existing resources join a stack through `adopt`, which
requires the plan to be a no-op for them.

Wave 1 still uses the `managed-by` tag for ownership. The ledger takes over in
the hardening wave.

### 4. Locks are a lease

Apply, rollback and prune require holding the stack lease (holder, revision,
expiry). Lease length comes from `APP_STACK_LOCK_TTL`; an apply renews it every
TTL divided by `APP_STACK_LOCK_RENEW_FRACTION` (default 3) and fails if it is lost. The plan hash is
re-checked under the lease, which closes the race left by the current
check-then-run order. An expired lease may be taken over. A stale lease can be
force-released with `unlock`, which is a human action.

### 5. Rollback is a forward re-apply

Rolling back to revision n re-plans n's rendered documents against live state
and applies them as a **new** revision with `rollback_of: n`. History is never
rewritten. Rollback does not restore secret values or database contents, and
the plan states that in a warning.

### 6. Pin

While `pinned_revision` is set, any apply that is not a rollback to the pinned
revision gets a 409. A pin freezes a stack during an incident: pin, act by
hand, fix git, unpin. The next plan shows the drift.

### 7. Drift policy: warn or lock

- `warn` (default): a drift check marks the stack drifted and the plan shows
  the change lines. Dashboard and CLI edits to stack-owned fields still work,
  with a warning that the next apply reverts them.
- `lock`: writes to stack-owned fields from outside a stack apply get a 409
  ("managed by stack X, apply a change or unlock").

Lock enforcement (v0.8.6): a guard in front of the router looks up the ledger
owner of the resource a POST, PUT, PATCH or DELETE targets (apps, databases,
domains, projects, environments). If the owner has policy `lock` and the
request is not part of that stack's own apply, the answer is 409 with code
`stack_locked`. Reads, and operations that change no declared field (start,
stop, restart, exec, builds, backups, cache purge, dry-run and check calls),
pass. Every other route on a locked resource is refused, including replicas
(a PUT of the app, and `autoscale`), image deploys, env, secrets, domains and
delete. A root caller may send `X-Levelrail-Override-Stack-Lock: <stack>`; the
override is audited as `stack.lock_override` and is never honored for CI or
stack-scoped tokens. If the ledger cannot be read the request is allowed and a
warning is logged; forgetting or destroying the stack unlocks its resources.

Additive maps (env, labels, tags, domains) keep showing live extras as `keep`.
Operational actions (restart, redeploy the same image, logs, exec) never count
as drift. When `autoscale` is declared, `replicas` is never drift. An
auto-revert mode is not offered in v0.8.6.

### 8. Permissions map onto existing abilities

No new global ability strings are added.

| Verb | Needs |
| --- | --- |
| list, get, plan, revisions, diff, drift, export | `read` (env values in export still need `read:sensitive`) |
| apply, rollback, adopt, pin | `write` on `stack:<name>` |
| apply with prune | `write` on `stack:<name>`, the manifest asks for prune, and the stack's `allow_prune` is on |
| destroy, unlock, change stack settings | `root`, or a team admin or owner of the stack's project. Never a CI token. |
| approve | `write` on `stack:<name>`, a different principal from the applier, before `APP_STACK_APPROVAL_TTL` expires |

A manifest cannot grant itself prune. A CI token may prune only when the
stack's server-side `allow_prune` is on. Destroy is never allowed from CI.

### 9. Stack-scoped tokens through IAM during an apply

Loopback sub-requests check `app:web`, `database:x` and so on, not
`stack:shop-prod`, so a token scoped to a stack would be denied on every item.
Instead a token scope gains a stack id. In IAM evaluation a stack-scoped
principal is allowed on resource R only when:

1. the request carries the stack apply context, a server-side context value set
   by the stack handler (never a client header), and
2. R is in that stack's ledger, or is being created inside the stack's project
   or environment scope.

Outside an apply context such a token reaches only `/api/v1/stacks/{name}/*`.
Explicit Deny statements still win. The same context carries the revision
number, so audit rows of loopback sub-requests record `stack_revision`.

The CI OIDC exchange gains stack trusts: `(repository, ref pattern, GitHub
environment)` maps to a stack and a set of verbs. A typical setup lets
`pull_request` refs plan and `refs/heads/main` with environment `prod` apply.
The exchange mints a short-lived stack-scoped token, so no long-lived write
token sits in CI secrets. Fork pull requests get no token and the plan job
skips with a notice.

### 10. Approvals

With `require_approval`, or when the stack targets a protected environment,
apply stores a `pending_approval` revision with the frozen plan hash. Approval
runs it only if the live plan still hashes the same, otherwise it fails with
`ErrPlanChanged`. The shape and TTL handling copy deploy approvals, with the TTL
from `APP_STACK_APPROVAL_TTL`.

### 11. Audit

Each stack verb writes one audit entry (`stack.apply`, `stack.rollback`,
`stack.prune`, `stack.destroy`, `stack.approve`, `stack.pin`, `stack.unlock`)
with `Detail` holding stack, revision, plan hash, git sha and counts. These
fields already sit in the audit hash chain, so its schema does not change.

### 12. Experimental flag and graduation bar

Stacks ship behind `APP_EXPERIMENTAL=iac` in v0.8.6, sharing the flag with the
stackless apply path. They graduate when all of these hold:

- the Wave 1 loop (PR plan comment, merge apply to revision 1, a second
  revision, rollback creating revision 3 with revision 1's documents) passes on
  a real VPS,
- two full tutorial runs complete without a stack bug left open,
- the secret persistence test and the lease contention test (two concurrent
  applies, exactly one wins) are green in CI.

## Contract types

`internal/iac/stack_types.go` holds the shared types (stack, stack spec,
revision and status, ledger entry, lease, drift policy, delete mode, the apply
scope context key) and their validation helpers. It has no store or API code.
The store and the stack API build on it, so the names in this ADR and in the
code stay the same.

## Rejected alternatives

- **A client-held state file.** It needs locking and secret handling on every
  client, and the server already knows live state. Rejected.
- **Ownership by tag only.** Tags cover apps only, can be edited by hand and
  give no history. Kept for back compat, replaced by the ledger.
- **Rollback by restoring stored rows.** It would bypass the API and its
  authorization and could not undo external effects. A forward re-apply goes
  through the same path as any apply.
- **A new global `stack` ability.** It widens a deliberately small role model.
  Resource-scoped `write` on `stack:<name>` covers it.
- **Trusting a client header for the apply context.** Any token could claim it.
  The context is set only by the server-side stack handler.

## Consequences

- One migration adds stacks, revisions, the ledger and leases. Later slices add
  the token stack scope and CI stack trusts.
- The stackless `POST /api/v1/apply` path keeps working unchanged.
- Rendered documents are stored, so a stack's history is readable by anyone
  with `read` on it. Env values remain hidden in plans and diffs.
- Secrets and database contents are never versioned. Rollback docs say so.

## What we do not do in v0.8.6

Cross-stack outputs, templating or overlays, auto-revert drift, server-side git
sync, destroy of databases, and generated SDKs.
