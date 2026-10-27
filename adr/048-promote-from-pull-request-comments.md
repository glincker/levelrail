# ADR 048: Promote from pull request comments

Status: Proposed

Date: 2026-10-27

## Context

Pull request comment commands (`docs/pr-comment-commands.md`) let a GitHub
user with write access run `status`, `redeploy`, `rollback`, `preview
redeploy` and `help` for the app whose webhook received the comment. The
handler verifies the webhook signature, ignores edits, bots and redelivered
comment ids, applies per-user and per-pull-request limits, confirms write or
admin permission with the GitHub App token (fail closed), and runs the same
freeze, running-deploy, protected environment and policy gates as the
dashboard. Refusals are audited as `pr_comment_command.refused`.

Users ask for `/levelrail promote <env>`. Promotion today is
`handlePromoteApp` (`internal/api/promote.go`). Into a protected environment
it never promotes directly: it calls `requestDeployApproval`
(`internal/api/deploy_approvals.go`), which saves a pending
`deploy_approvals` row. A different privileged user then decides it.
`loadDecidableApprovalAs` refuses a decider whose `(type, id)` equals the
requester's.

Two facts shape this decision:

1. `requestDeployApproval` takes an `http.ResponseWriter` and an
   `*http.Request`. It reads the requester from the session or token
   (`currentActor`) and writes its own error responses. A webhook has no
   such actor.
2. The self-approval check compares requester and decider by `(type, id)`.
   If a GitHub login were recorded under a type of its own, the same person
   could request from a comment and approve from the dashboard, and the
   check would not notice.

## Questions and answers

### How a GitHub login maps to a requester identity

Options:

- **A. Linked identity only.** Promote is accepted only when the GitHub
  login is linked to a Levelrail user (an OAuth link or an admin-set
  mapping). The approval row records that user as the requester, so the
  existing self-approval check works unchanged. An unlinked login is
  refused with reason `unlinked_github_login`.
- **B. A dedicated `github` requester type.** The row records
  `RequestedByType: "github"`, `RequestedBy: <numeric GitHub user id>`,
  `RequestedByName: github:<login>`. It needs its own limits and its own
  self-approval rule, because a `github` requester and a `session` decider
  never compare equal.
- **C. Both.** Use the linked user when there is one, fall back to B.

The numeric GitHub user id, not the login, is the stable key. Logins can be
renamed and then claimed by someone else.

### Why a comment must never satisfy an approval

An approval is a second, independent human decision made in an
authenticated Levelrail session or a verified chat action. A comment proves
only that a GitHub account with write access typed some text, under
GitHub's authentication rather than ours. Write access on a repository is
often wider than the set of people allowed to change production. If a
comment could approve, then two GitHub accounts, or one person with two
accounts, could request and approve with nothing in Levelrail ever checking
who they are. So comments can only **create** a pending approval. There is
no `/levelrail approve` verb. Edited comments stay ignored, so a comment
cannot be changed into something else after a reviewer has read it.

### Protected environments and required approvers

- A promote into an unprotected environment still goes through an
  approval when it comes from a comment. Unlike a dashboard promote, a
  comment-triggered promote never takes the direct path. This keeps a
  single rule: comments request, people decide.
- A protected environment's existing approver rules apply unchanged. The
  decider must not be the requester. Under option A that is the existing
  check. Under option B the check must also refuse a decider whose linked
  GitHub id equals the requester's id.
- Freeze, policy and image signature checks run when the request is made
  and again when it is decided, as for a dashboard promote. A comment can
  never set `force`, `confirm` or a freeze override.
- The target is resolved only from the app's own project: `<env>` is an
  environment name in that project, and exactly one tagged app must exist
  there. Anything else is refused, never guessed.

### Replay and spoofing

- Forged deliveries: rejected by the per-app HMAC signature, as today.
- Redelivery and operator replay: delivery id dedupe plus the per-app
  comment id claim, as today. Only one approval row can come from a comment
  id. The comment id is stored on the row for traceability.
- Login spoofing: the login in the payload is only a lookup key. Permission
  is confirmed with the GitHub API. Under option A the link is checked
  against the numeric id from the payload, which GitHub signs inside the
  delivery.
- Stale permission: a promote skips the permission cache and always asks
  GitHub live.
- Flooding: per-user and per-pull-request limits, plus a cap on open
  comment-sourced approvals per app (from env), so a comment storm cannot
  bury approvers in requests.
- Pull request content: `<env>` is matched against a fixed set of names.
  It is never interpolated into a reply, a log line or a path without
  sanitizing.

## Decision (recommended)

Option A, linked identity only, for the first version. It reuses the
existing requester model and self-approval check with no new trust type,
and an unlinked login gets an audited refusal and no reply. Option B stays
open as a later extension if unlinked contributors need it.

Rejected for now: B, because it adds a requester type that the
self-approval check cannot compare with a dashboard decider without a link
anyway. C, because it carries B's gap. A direct promote from a comment into
an unprotected environment, because it would be the only path where a
comment changes another environment with no human in Levelrail involved.
An `approve` verb, for the reasons above.

## Code touch points

- `internal/api/deploy_approvals.go`: split `requestDeployApproval` into an
  HTTP wrapper and a core `createDeployApproval(ctx, requester, ...)
  (store.DeployApproval, error)` that takes the requester as a value.
  `handleTriggerDeploy` and `handlePromoteApp` keep calling the wrapper.
- `internal/api/promote.go`: lift the checks out of `resolvePromotion` and
  `handlePromoteApp` (blockers, signature, environment lookup) into
  functions that take a context and return errors instead of writing
  responses.
- `internal/api/pr_comment_commands.go`: a `promote` case that resolves the
  linked user, runs the lifted checks, and always calls
  `createDeployApproval`. The reply links to the approval in the dashboard.
- `internal/prcommand`: a `promote <env>` form with an environment name
  validated against a strict charset and length.
- `internal/store`: comment id and pull request number on the approval row
  (a migration), and the GitHub link lookup for option A.
- Audit: accepted requests as `pr_comment_command.promote_requested`,
  refusals under the existing `pr_comment_command.refused` action.

## Consequences

- Promote from a comment is never faster than the approval it creates. That
  is deliberate.
- Option A needs a GitHub identity link, which does not exist yet. Until
  it lands, promote from comments is not offered.
- The HTTP-free approval core also helps chat and MCP callers that today
  have to work around `requestDeployApproval`'s HTTP signature.
