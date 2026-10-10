# IAM: what exists and what the authoring tools add

This page records how IAM policies work today and what the policy builder,
simulator and analyzer layer on top. The evaluator itself is unchanged.

## What exists

- A policy is a JSON document of statements: `Effect` (Allow or Deny),
  `Action` (ability strings or `*`), `Resource` (identifiers or `*`).
  Policies live in `iam_policies`; attachments link a policy to a `user` or
  `token` principal in `iam_policy_attachments`.
- Abilities are a flat list on every user and token: `read`, `read:sensitive`,
  `write`, `write:sensitive`, `deploy`, `root`. `root` implies all of them.
- Resources are `app:NAME`, `database:NAME`, `environment:ID` and
  `environment-kind:KIND`. A trailing `*` is a prefix match. An app or
  database also carries the environment resources of the environment it is
  tagged with.
- Evaluation (`authorizeResource` in `internal/api/iam.go`): an explicit Deny
  in any attached policy always wins; otherwise the principal's flat
  abilities decide; otherwise an explicit Allow can grant the ability on that
  resource. A malformed stored document is inert. A principal with no
  policies behaves exactly like its flat abilities.
- Route gates that name a resource (`requireAbilityForResource`) call the
  evaluator. Routes gated by `requireAbility` consult flat abilities only.
- Every route above `read` is audit logged by the shared gate, which covers
  policy create, update, delete, attach and detach.
- Statements carry no conditions today. Environment kind is the one
  condition-like check, and it is expressed as a resource.

## What the authoring tools add

- A catalog of abilities (group, plain description, risk) and a resource
  picker with live match counts, both read only.
- A simulator that runs the real `authorizeResource` path (including
  environment visibility) and lists every statement that matched, plus a
  grouped effective permissions view.
- An analyzer: static checks over stored documents, attachments and
  principals, each with a severity and a one line fix.
- Preview before change (attach, detach, update) as a before and after
  effective permission diff, a version history for policy documents, and a
  guard that refuses a change that would leave no operational root
  principal.
- Parameterized templates and field level validation errors.

## Guarantees and limits

- No evaluation result changes. `internal/api/iam_evaluator_pin_test.go`
  pins the existing behavior; the simulator is tested to agree with
  `authorizeResource` on every case.
- Source IP, time window and MFA conditions are not supported by the
  evaluator, so the builder does not offer them. A `Condition` key written by
  hand is ignored by the evaluator and the analyzer flags it.
- There is no project or tag resource in the evaluator. The builder expands a
  project into its environments when a policy is saved.
- Usage data is per token (`last_used_at`), not per policy, so previews state
  that and do not claim a policy was used.
