# Repo Presence, README, Docs, and Marketing Plan Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give the Levelrail GitHub repo real SEO/discovery presence (topics,
Discussions), rewrite the README with an accurate competitor comparison,
stand up a real `/docs` tree, and write a private post-beta marketing plan,
all grounded in facts (code, `TASKS.md`, existing `docs-local` research)
rather than invented claims.

**Architecture:** One prep task builds a feature/status inventory that acts
as the shared fact base. Repo-settings is a standalone task run directly in
the main session (touches live, shared GitHub state, needs explicit user
confirmation per command, not something to hand to an unattended subagent).
Everything else is documentation-only, no app code touched, and fans out
once the fact base and the comparison research exist.

**Tech Stack:** Markdown, `gh` CLI for repo settings. No build step.

**Note on task style:** this is a documentation plan, not a code/TDD plan.
"Tests" are replaced with concrete verification commands (grep checks, link
checks) and every content task specifies exact required sections and the
exact source files/facts each section must be grounded in, so no step is
"add appropriate content" with nothing underneath it.

---

## Dependency order

```
Task 1 (feature/status inventory) ─┬─> Task 3 (docs/comparison.md) ─> Task 4 (README.md)
                                    ├─> Task 5 (docs/getting-started.md)
                                    ├─> Task 6 (docs/architecture.md)
                                    ├─> Task 7 (docs/app-spec-reference.md)
                                    ├─> Task 8 (docs/roadmap.md)
                                    └─> Task 9 (docs-local/strategy/marketing-plan.md, also needs Task 3)

Task 2 (GitHub repo settings) — independent, run any time, main session only.
```

Tasks 5, 6, 7, 8 are independent of each other and of Task 4 once Task 1 is
done — good candidates for parallel subagents (matches this repo's own
CLAUDE.md guidance: "Docs" is listed as good parallel work). Task 9 needs
both Task 1 and Task 3 finished first.

---

### Task 1: Feature/status inventory (fact base)

**Files:**
- Create: `docs-local/research/feature-inventory-2026-08-15.md`

- [ ] **Step 1: Enumerate implemented surface area**

Run and record output:
```bash
ls cmd/
ls internal/
find web/src/routes -iname "*.tsx" | sort
```

- [ ] **Step 2: Cross-reference against the phase plan and build log**

Read `CLAUDE.md` section 6 (Phases) for the target/aspirational plan.
Read `TASKS.md` section headers for actual status:
```bash
grep -n "^## " TASKS.md
```
As of this plan's writing, that returns:
```
Phase 0: Research and foundation. COMPLETE (2026-08-11)
Phase 1: Single node, deploy something real. IN PROGRESS
Phase 2: Observability, the actual differentiator. IN PROGRESS (started 2026-08-12)
Phase 3: Multi-node. Breakdown written (2026-08-13)
```
Also read `TASKS-v2.md` (dashboard redesign log) for frontend shell status,
and skim the top ~50 lines of `QUEUE.md` for the current in-flight items
and any founder directives that affect what's safe to claim as "done."

- [ ] **Step 3: Write the inventory doc**

Structure required (fill every section with real findings, no placeholders):
```markdown
# Feature/status inventory (2026-08-15)

Fact base for the repo-presence/docs/marketing initiative. Every claim
in README.md, /docs, and docs-local/strategy/marketing-plan.md about what
Levelrail *currently does* must trace back to this file. Re-derive this
file (don't hand-edit it later) if it goes stale.

## Shipped and working today
[bullet list: capability -> which package/route implements it -> one-line
evidence, e.g. "Alerting (threshold rules, webhook/email/Slack/Discord/
Telegram notification) -- internal/alerting -- exists despite Phase 2
plan listing it as not-yet-started"]

## In progress
[bullet list, cite TASKS.md/TASKS-v2.md section]

## Not started
[bullet list, cite the phase plan section describing it]

## Divergences from CLAUDE.md's phase plan
[Where the actual build has moved ahead of or differs from the phase
list in CLAUDE.md section 6 -- this is the list that keeps roadmap.md
honest instead of just restating the aspirational plan]
```

- [ ] **Step 4: Verify no invented facts**

Read the finished file back and confirm every bullet cites a real path or
a real `TASKS.md`/`TASKS-v2.md` section header. Delete any bullet that
doesn't.

- [ ] **Step 5: Commit**

```bash
git add docs-local/research/feature-inventory-2026-08-15.md
git commit -m "docs: add feature/status inventory as fact base for repo-presence work"
```
Note: `docs-local/` is gitignored per this repo's convention (CLAUDE.md
8a) -- confirm with `git status` that this file is *not* staged/tracked
if `.gitignore` excludes it. If it's ignored, skip the commit for this
file only and say so; the file still exists on disk for later tasks to
read.

---

### Task 2: GitHub repo settings

**Run in the main session, not a detached subagent.** This changes live,
public-facing settings on `glincker/levelrail`. Confirm the exact commands
with the user before running each one (per this project's rule on
shared-state actions).

- [ ] **Step 1: Add topics**

```bash
gh repo edit glincker/levelrail \
  --add-topic self-hosted \
  --add-topic paas \
  --add-topic docker \
  --add-topic deployment \
  --add-topic devops \
  --add-topic golang \
  --add-topic coolify-alternative \
  --add-topic dokploy-alternative \
  --add-topic self-hosted-paas \
  --add-topic buildkit \
  --add-topic caddy \
  --add-topic wireguard \
  --add-topic ci-cd
```

- [ ] **Step 2: Verify topics landed**

```bash
gh repo view glincker/levelrail --json repositoryTopics
```
Expected: all 13 topics listed.

- [ ] **Step 3: Enable Discussions**

```bash
gh api -X PATCH repos/glincker/levelrail -f has_discussions=true
```

- [ ] **Step 4: Verify Discussions is on**

```bash
gh repo view glincker/levelrail --json hasDiscussionsEnabled
```
Expected: `"hasDiscussionsEnabled":true`.

- [ ] **Step 5: Seed default Discussion categories and a welcome post**

GitHub creates default categories (General, Ideas, Q&A, Show and tell,
Announcements) automatically on first enable. Confirm via the web UI or:
```bash
gh api graphql -f query='
{
  repository(owner: "glincker", name: "levelrail") {
    discussionCategories(first: 10) { nodes { name } }
  }
}'
```
Then create one pinned welcome post (do this through the GitHub web UI,
`gh` has no discussion-create command as of this writing) in Announcements:
title "Welcome to Levelrail" -- short paragraph pointing at README's Status
section and the new `/docs` tree, inviting Ideas/Q&A. Pin it.

- [ ] **Step 6: No commit** -- this task only changes repo settings, not files.

---

### Task 3: `docs/comparison.md`

**Files:**
- Create: `docs/comparison.md`
- Depends on: Task 1 output, plus reading:
  `docs-local/research/prior-art-coolify.md`,
  `docs-local/research/prior-art-dokploy.md`,
  `docs-local/research/prior-art-caprover.md`,
  `docs-local/research/prior-art-dokku.md`,
  `docs-local/research/prior-art-kamal.md`

- [ ] **Step 1: Extract comparison facts per competitor**

For each of the 5 `prior-art-*.md` files, pull out: node control method,
orchestration engine, observability approach, ingress, license, build
engine, and (if stated) idle resource footprint and most-complained-about
GitHub issue theme. Do not paraphrase from memory of these tools --
every fact must come from the prior-art file's text.

- [ ] **Step 2: Write the comparison doc**

Structure required:
```markdown
# Comparison

Positioning, not a ranking. All of these projects are worth using --
this page exists to make the actual technical differences legible, sourced
from direct research (see `docs-local/research/prior-art-*.md` for the
full per-project writeups this table is drawn from, at the level of detail
this project's own research already produced).

## At a glance

| Project | Node control | Orchestration | Observability | Ingress | License | Build engine |
| --- | --- | --- | --- | --- | --- | --- |
[one row per project, including Levelrail, all cells sourced per Step 1]

## Levelrail vs Coolify
[2-4 sentences, specific, sourced from prior-art-coolify.md]

## Levelrail vs Dokploy
[same pattern]

## Levelrail vs CapRover
[same pattern]

## Levelrail vs Dokku
[same pattern]

## Levelrail vs Kamal
[same pattern -- Kamal is the smallest/most instructive per CLAUDE.md
Phase 0 notes, call out that it isn't a control plane at all]

## What Levelrail doesn't do (yet)
[honest gaps, sourced from Task 1's inventory -- e.g. multi-node,
WireGuard mesh, if still not-started as of Task 1's findings]
```

- [ ] **Step 3: Verify sourcing**

Grep the doc for any competitor claim and confirm it appears in the
corresponding `prior-art-*.md` file. Fix or cut anything that doesn't.

- [ ] **Step 4: Verify no em dashes**

```bash
grep -n "—\|–" docs/comparison.md
```
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add docs/comparison.md
git commit -m "docs: add sourced competitor comparison page"
```

---

### Task 4: README.md overhaul

**Files:**
- Modify: `README.md`
- Depends on: Task 3 (`docs/comparison.md`), Task 1 (inventory)

- [ ] **Step 1: Add badge row**

Insert directly under the `# Levelrail` heading, above the existing
License badge line (replace that single-badge line with this row):
```markdown
[![CI](https://github.com/glincker/levelrail/actions/workflows/ci.yml/badge.svg)](https://github.com/glincker/levelrail/actions/workflows/ci.yml)
[![License: Apache 2.0](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](LICENSE)
[![Go Report Card](https://goreportcard.com/badge/github.com/glincker/levelrail)](https://goreportcard.com/report/github.com/glincker/levelrail)
[![Go Version](https://img.shields.io/github/go-mod/go-version/glincker/levelrail)](go.mod)
```

- [ ] **Step 2: Replace the "How it compares" table**

Replace the existing table (README.md lines ~96-104 as of this plan's
writing -- re-locate by searching for `## How it compares`) with a trimmed
version of `docs/comparison.md`'s "At a glance" table (same columns Task 3
produced, or a subset if the row gets too wide for a table cell), and add
directly below the table:
```markdown
Full writeup with per-project detail: [docs/comparison.md](docs/comparison.md).
```

- [ ] **Step 3: Add a links section**

Add a new section after "Contributing" and before "License":
```markdown
## Docs and community

- [docs/](docs/) -- getting started, architecture, app spec reference, roadmap
- [GitHub Discussions](https://github.com/glincker/levelrail/discussions) -- questions, ideas, show and tell
```

- [ ] **Step 4: Verify no em dashes and no accidental overclaiming**

```bash
grep -n "—\|–" README.md
```
Expected: no output. Then re-read the "Status" section and confirm nothing
added in Steps 1-3 contradicts its beta/no-stable-release framing.

- [ ] **Step 5: Commit**

```bash
git add README.md
git commit -m "docs: add badges, sourced comparison table, docs/community links to README"
```

---

### Task 5: `docs/getting-started.md`

**Files:**
- Create: `docs/getting-started.md`
- Depends on: Task 1 (inventory)

- [ ] **Step 1: Confirm current build/run steps still match README**

Re-read README.md's "Building and running locally" section and
`web/README.md` for anything the README doesn't already cover (env vars,
ports, first-run setup). Also check for a `dev-fixtures.yml` (exists at
repo root) and whether it's referenced anywhere as a "deploy a sample app"
path.

- [ ] **Step 2: Write the doc**

Structure required:
```markdown
# Getting started

## Requirements
[from README: Go 1.25+, Docker -- plus anything Step 1 found that
README omits, e.g. Node/npm version for the frontend]

## Build and run
[the two `go build` commands from README, plus the web/ npm commands,
reproduced here so this page stands alone -- don't just say "see README"]

## Deploy your first app
[walk through app.yaml per the schema in CLAUDE.md 4.9 and
docs/app-spec-reference.md once Task 7 exists -- if dev-fixtures.yml or
test/fixtures is a usable example, point at the real path, don't invent
a fake example]

## Where to go next
[link docs/architecture.md, docs/app-spec-reference.md, docs/comparison.md]
```

- [ ] **Step 3: Verify every command in the doc actually exists**

For every fenced command block, confirm the referenced file/flag exists,
e.g.:
```bash
test -f test/fixtures -o -d test/fixtures && echo exists
ls dev-fixtures.yml
```

- [ ] **Step 4: Verify no em dashes**

```bash
grep -n "—\|–" docs/getting-started.md
```
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add docs/getting-started.md
git commit -m "docs: add getting-started guide"
```

---

### Task 6: `docs/architecture.md`

**Files:**
- Create: `docs/architecture.md`
- Depends on: Task 1 (inventory)

- [ ] **Step 1: Ground each section in real code**

For each of the architecture pieces below, confirm the package exists and
note one real file to point at:
```bash
ls internal/reconcile internal/docker internal/build internal/ingress \
   internal/store internal/telemetry internal/secrets internal/agent \
   internal/network
```

- [ ] **Step 2: Write the doc**

Structure required (expand README's "Architecture at a glance" section,
don't just repeat it verbatim):
```markdown
# Architecture

## Control plane
[internal/reconcile -- level-triggered reconciler pattern, per CLAUDE.md
4.2, cite that it's never edge-triggered and why (interruption safety)]

## Node agent
[internal/agent, internal/docker -- reverse-dialed gRPC, no inbound ports,
in-process transport in single-node mode per CLAUDE.md 4.3, and confirm
from Task 1's inventory whether multi-node/real gRPC transport has
actually shipped yet or is still in-process only]

## Builds
[internal/build -- BuildKit as a library, not `docker build`, per CLAUDE.md
4.4]

## Ingress
[internal/ingress -- embedded Caddy, per CLAUDE.md 4.5]

## State
[internal/store -- embedded SQLite WAL mode, modernc.org/sqlite, per
CLAUDE.md 4.7]

## Observability
[internal/telemetry -- node-local metrics/logs, federated query, per
CLAUDE.md 4.8 -- and per Task 1's inventory, is federated query itself
live yet, or only single-node today?]

## Secrets
[internal/secrets -- envelope encryption, filippo.io/age, per CLAUDE.md
4.10]

## Frontend
[web/ -- React/Vite/TS/Tailwind, embedded via embed.FS, per CLAUDE.md 4.12]
```
Every bracketed claim about *current* status (not just design intent) must
be checked against Task 1's inventory before writing it as fact.

- [ ] **Step 3: Verify no em dashes**

```bash
grep -n "—\|–" docs/architecture.md
```
Expected: no output.

- [ ] **Step 4: Commit**

```bash
git add docs/architecture.md
git commit -m "docs: add architecture overview"
```

---

### Task 7: `docs/app-spec-reference.md`

**Files:**
- Create: `docs/app-spec-reference.md`
- Depends on: Task 1 (inventory)

- [ ] **Step 1: Find the actual spec parser/validator**

```bash
ls internal/spec
grep -rn "yaml:\"" internal/spec | head -40
```
Use the real struct tags/fields found here as the source of truth, not
just the example in CLAUDE.md 4.9 (which may already be stale relative to
the code -- if they disagree, the code wins, and note the discrepancy).

- [ ] **Step 2: Write the doc**

Structure required:
```markdown
# app.yaml reference

The declarative spec Levelrail reads from your repo.

## Full example
[the CLAUDE.md 4.9 example, corrected against Step 1's findings if fields
differ]

## Field reference
[table: field -> type -> required? -> default -> description, one row per
field actually found in internal/spec's structs -- do not include fields
from the CLAUDE.md example that Step 1 didn't find implemented, list them
separately under "Planned, not yet implemented" instead]

## Validation
[how/when validation runs -- cite the actual validator function/file from
internal/spec]
```

- [ ] **Step 3: Verify field table matches code exactly**

For every row in the field reference table, grep to confirm the field
exists:
```bash
grep -n "<FieldName>" internal/spec/*.go
```

- [ ] **Step 4: Verify no em dashes**

```bash
grep -n "—\|–" docs/app-spec-reference.md
```
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add docs/app-spec-reference.md
git commit -m "docs: add app.yaml reference, verified against internal/spec"
```

---

### Task 8: `docs/roadmap.md`

**Files:**
- Create: `docs/roadmap.md`
- Depends on: Task 1 (inventory) -- this task consumes it directly

- [ ] **Step 1: Pull the "Divergences from CLAUDE.md's phase plan" section**

Read that section of Task 1's inventory doc directly, it's written for
exactly this purpose.

- [ ] **Step 2: Write the doc**

Structure required:
```markdown
# Roadmap

Status as of [date], not the aspirational plan -- see CLAUDE.md in the
repo for the full phase-by-phase design doc if you want the long version.

## Done
[from Task 1's "Shipped and working today"]

## In progress
[from Task 1's "In progress"]

## Not started
[from Task 1's "Not started"]

## Explicitly out of scope
[pull straight from CLAUDE.md section 2, Non-goals -- this section can be
copied faithfully since it's already a settled, accurate list]
```

- [ ] **Step 3: Verify against inventory, not vibes**

Confirm every bullet in Done/In progress/Not started matches a bullet in
Task 1's inventory file. This file should be near-mechanical, not creative
writing.

- [ ] **Step 4: Verify no em dashes**

```bash
grep -n "—\|–" docs/roadmap.md
```
Expected: no output.

- [ ] **Step 5: Commit**

```bash
git add docs/roadmap.md
git commit -m "docs: add roadmap page grounded in feature inventory"
```

---

### Task 9: `docs-local/strategy/marketing-plan.md`

**Files:**
- Create: `docs-local/strategy/marketing-plan.md`
- Depends on: Task 1 (inventory), Task 3 (comparison facts)

- [ ] **Step 1: Confirm this file is gitignored**

```bash
git check-ignore -v docs-local/strategy/marketing-plan.md || echo "NOT IGNORED -- STOP, check .gitignore before writing"
```
If it's not ignored, stop and fix `.gitignore` first (add `docs-local/` or
confirm the existing pattern covers a new subdirectory) rather than risk
committing strategy material. This file must never be `git add`ed.

- [ ] **Step 2: Write the plan**

Structure required:
```markdown
# Marketing plan (post-beta)

Status: not yet triggered. Gated on Phase 5's exit criterion (shadow-run
period complete, per CLAUDE.md section 6). This is a plan to execute
later, nothing here authorizes any outbound action now.

## Positioning
[grounded in docs/comparison.md's per-competitor sections -- the actual
differentiators, not generic SaaS marketing language]

## Launch channels
[Hacker News (Show HN), r/selfhosted, r/homelab -- why each one fits this
audience specifically, not a generic channel list]

## Timing
[tied explicitly to Phase 5's exit criterion and the shadow-run period,
per CLAUDE.md section 6 and section 10]

## Content angles
[the "N apps on one control plane, idle CPU/memory" number from Phase 5's
"Performance" work item is the strongest asset once it exists -- list
other angles the same way, each tied to a real, already-planned piece of
work rather than invented]

## Open questions
[carry forward CLAUDE.md section 9's open decision #5: whether the first
public release is announced at all, or used quietly for GLINCKER apps
first -- this plan should answer that question or explicitly flag it as
still open]
```

- [ ] **Step 3: Verify no em dashes**

```bash
grep -n "—\|–" docs-local/strategy/marketing-plan.md
```
Expected: no output.

- [ ] **Step 4: Confirm still not tracked, do not commit**

```bash
git status --short docs-local/strategy/marketing-plan.md
```
Expected: no output (untracked files matching an ignored pattern don't
show in `git status --short` at all; if it *does* show, stop, that means
it isn't actually ignored).

---

## Final verification (run after all tasks land)

- [ ] **Step 1: Confirm no product-name-leak regression**

```bash
grep -rn "Levelrail" cmd/ internal/ web/src proto/ 2>/dev/null
```
Expected: no output (this plan shouldn't touch those directories at all,
this is a final sanity check that nothing did).

- [ ] **Step 2: Confirm no em dashes anywhere touched**

```bash
grep -rn "—\|–" README.md docs/ 2>/dev/null
```
Expected: no output.

- [ ] **Step 3: Confirm all relative links resolve**

```bash
grep -on "\[[^]]*\](docs/[a-z-]*\.md)" README.md docs/*.md
```
For each match, confirm the referenced file exists under `docs/`.

- [ ] **Step 4: Run this repo's existing verification script**

```bash
scripts/verify.sh
```
This is a docs-only change, so this is a low-risk sanity check that
nothing in the working tree is broken, not an expectation that docs
changes would fail it.
