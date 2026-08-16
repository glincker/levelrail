# Repo presence, README, docs, and marketing plan

Status: approved
Date: 2026-08-15

## Purpose

Levelrail's GitHub repo (`glincker/levelrail`, public, 1 star, no topics,
Discussions off) has no SEO/discovery presence and the README, while
decent, understates the project relative to the actual research already
done in `docs-local/research/prior-art-*.md`. This is groundwork so
crawlers, search, and human visitors get an accurate, comparison-grounded
picture of the project while it's still in beta, plus a private plan for
marketing once beta ends. No application code is touched.

## Scope

Four independent deliverables, safe to work in parallel (no shared files
except README.md, which only the README workstream touches):

1. GitHub repo settings (topics, Discussions)
2. README overhaul
3. `/docs` tree (real content)
4. Marketing plan (private, `docs-local/`)

## 1. GitHub repo settings

- Add topics via `gh repo edit --add-topic`: `self-hosted`, `paas`,
  `docker`, `deployment`, `devops`, `golang`, `coolify-alternative`,
  `dokploy-alternative`, `self-hosted-paas`, `buildkit`, `caddy`,
  `wireguard`, `ci-cd`.
- Enable Discussions (`gh api -X PATCH repos/glincker/levelrail
  has_discussions=true`), categories: Announcements, Q&A, Ideas, Show and
  Tell. Seed one pinned welcome/roadmap post.
- Community health files (CODE_OF_CONDUCT, CONTRIBUTING, SECURITY, issue
  templates, PR template) already exist, no gap to fill.
- Explicitly out of scope: social preview image (needs a design asset,
  not a text-session deliverable), `FUNDING.yml` (premature pre-launch).
- This step touches live repo settings (visible to others). Confirm the
  exact `gh` commands with the user before running them, same as any
  other shared-state action.

## 2. README overhaul

- Keep current structure and tone (status caveat, architecture-at-a-glance,
  building instructions) since it's already accurate and not overclaiming.
- Add a badge row: CI status, License, Go Report Card, Go version.
- Rebuild the comparison table using the real findings in
  `docs-local/research/prior-art-{coolify,dokploy,caprover,dokku,kamal}.md`
  rather than the current shorter version, add license and build-engine
  columns.
- Add links to GitHub Discussions and the new `/docs` tree.
- No overclaiming: stays honest about beta status.

## 3. `/docs` tree

Real content, not stubs, structured to work as plain GitHub-rendered
markdown today and drop into glinr.com/levelrail later without a rewrite:

```
docs/
  getting-started.md      # build + run locally, deploy a first app
  architecture.md         # control plane / agent / reconciler, from CLAUDE.md 4.x
  app-spec-reference.md   # app.yaml contract, from CLAUDE.md 4.9
  comparison.md           # fuller version of the README table
  roadmap.md              # current status framed honestly, not the aspirational phase list
```

Content must be grounded in what's **actually built**, verified against
`TASKS.md`/`TASKS-v2.md` and the code (`cmd/`, `internal/`, `web/src/routes/`)
rather than copied from the CLAUDE.md phase plan, since the build has
already moved past parts of that plan (e.g. `internal/alerting`,
`internal/backup`, `internal/webhook` all exist despite the phase plan
describing Phase 3/4 work as not started). `roadmap.md` in particular must
not read as vaporware: say what's done, what's in progress, what's not
started.

## 4. Marketing plan

- Lives at `docs-local/strategy/marketing-plan.md`. Never referenced from
  `/docs`, README, or committed publicly, per this repo's own `docs-local`
  convention (CLAUDE.md section 8a: strategy and launch planning is
  private, pre-decision material).
- Covers: positioning against Coolify/Dokploy (grounded in the same
  prior-art research), launch channels (Hacker News, r/selfhosted,
  r/homelab), timing tied to the Phase 5 exit criterion (shadow-run period
  complete), content angles (the "N apps on one control plane, idle
  CPU/memory" number from Phase 5 is the strongest asset once it exists).
- Explicitly marked not-yet: this is a plan to execute later, not a
  trigger to start any outbound activity now.

## Verification

- No product name string leaks into `/cmd`, `/internal`, `/web`, `/proto`
  (this work is all in README/docs/docs-local, so N/A here, but re-confirm).
- No em dashes/en dashes anywhere written.
- Every comparison claim about a competitor traces back to a specific
  `docs-local/research/prior-art-*.md` finding, not assumption.
- Every `/docs` claim about Levelrail's own capabilities traces back to
  actual code or `TASKS.md`, not the phase plan.
- Markdown links resolve (no dead relative links between README and
  `/docs`).

## Explicitly not doing

- No social preview image / design assets.
- No actual marketing/outbound activity (posting, announcing).
- No changes to application code, CI, or build config.
- No `/docs` content that depends on the multi-node phase (WireGuard mesh,
  agent enrollment) actually shipping. Document what exists today.
