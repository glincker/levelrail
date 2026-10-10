---
title: glinr-bot
description: "How the repo's GitHub App bot evaluates pull requests against a policy file, comments its reasoning and, in enforce mode, approves and arms auto-merge."
---

# glinr-bot

A GitHub App that reads a policy file, checks each pull request against it, and
says why. In shadow mode it only comments. In enforce mode it can also approve
and arm auto-merge, and GitHub still waits for the required checks.

## How it works

- Trigger: `pull_request_target`, so the app secrets are available to
  Dependabot and same-repo PRs.
- Safety: the job checks out the **base** branch only. The policy and the
  evaluator come from the base, so a PR cannot edit its own rules, and nothing
  from the PR is built or executed. It reads the PR through the API.
- Output: one comment per PR, updated in place, with a table of gates.

## The policy file

`.github/glinr-bot.yml`:

| Key | Meaning |
|---|---|
| `mode` | `shadow` comments only, `enforce` may approve and merge |
| `deny_paths` | globs that always send the PR to a person |
| `block_labels` | labels that always send the PR to a person |
| `rules[].authors` | exact logins allowed |
| `rules[].paths_only` | every changed file must match one glob |
| `rules[].max_files`, `max_changed_lines` | size limits |
| `rules[].update_types` | `patch`, `minor` or `major`, read from "from X to Y" in the title |
| `rules[].title_prefix` | required title start |
| `rules[].actions` | `comment`, `approve`, `automerge` |

Global gates apply before any rule: not a draft, not from a fork, touches no
denied path, no blocking label. The first rule whose gates all pass wins.

## Rollout pattern for other repos

1. Install the app on the repo and set the `RELEASE_APP_CLIENT_ID` variable and
   `RELEASE_APP_PRIVATE_KEY` secret.
2. Copy `.github/workflows/glinr-bot.yml`, `.github/glinr-bot.yml` and
   `scripts/glinr-bot/`.
3. Start with `mode: shadow` and read the comments for a week.
4. Move the lowest-risk rule to enforce first, for example docs only.

## What it does not do

It does not review code quality, does not merge past failing required checks,
and never acts on a fork or on a path in `deny_paths`.
