---
description: How to commit, describe and label changes so the release history stays readable as the repo grows.
---

# Change tracking

How a change travels from commit to release notes, and where its status is recorded. Short rules, so reviewers and release notes stay readable as the repo grows.

## Commits

- One clubbed commit per theme. A feature that spans API, CLI, dashboard and docs is one commit, even at 50 or more files. Do not split a feature into 1 to 3 file commits.
- Subject: `type(scope): outcome`, under 72 characters. Types are feat, fix, perf, refactor, docs, test and chore. Describe the result, not the activity.
- Body: at most 3 short lines of why. Optional trailers `Task: 118, 123` and `ADR: 074`.
- No AI attribution and no co-author trailers.

```
feat(apps): add preview environments per pull request

Previews reuse the build center and expire on a setting.
Task: 118, 123
ADR: 074
```

## Pull request description

Fill in the PR template. Keep the bullets specific: name the verb, the route, the setting key, the flag, the table or the UI page.

For a release PR, build the description from the merged PR sections:

1. Group by capability, not by commit order.
2. List every migration number, forward only.
3. List every new runtime setting key with its default.
4. List every flag with its status label.
5. Collect behaviour changes and upgrade notes in one place.
6. Collect the "Not verified / needs a real host" items so unproven checks are visible.

## Where tracking lives

| What | Where |
| --- | --- |
| Maturity label and test evidence per feature | [feature-status.md](/feature-status) |
| Known gaps and limits | [known-limits.md](/known-limits) |
| What is done and planned | [roadmap.md](/roadmap) |
| Design decisions | ADRs, referenced by number in the commit trailer |

Update the matching row in the same commit that changes the behaviour. A claim without a row or evidence does not ship as stable.

## Generated files

Regenerate, never hand merge. On a conflict in a generated file (API reference, OpenAPI, route tree, locale catalogs), take either side, then run the repo generator and commit the result.

## Labels and evidence

| Label | Meaning | Evidence needed |
| --- | --- | --- |
| experimental | Behind a flag, may change or be removed | Unit tests; docs say it is gated |
| beta | Works end to end, limited real-world use | Integration test through the real code path; listed gaps in known-limits |
| stable | Safe to rely on | Tests plus a run on a real host, upgrade path tested, authz deny tests, no open gaps that lose data |

Move a label up only with the evidence in the same change, and link the test or run in the feature-status row.

## Runtime settings, not env

New tunables are runtime settings: reachable from the API, CLI and dashboard, applied live where possible. Environment variables only seed defaults and never lock a value. Thresholds are never hardcoded. Record the key, default and whether it applies live or needs a restart in the PR.

## Reviewing a change

A reviewer should be able to answer from the PR alone: what changed, which surfaces got it, what migrates, what is new to configure, what is unproven, and how to roll back.
