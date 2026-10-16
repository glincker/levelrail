Minimal fixtures for scripts/vps-smoke.sh. Each serves its MARKER file at / and "ok" at /healthz.

## Reading build stats after a smoke deploy

Each deploy of a fixture records one build. After deploying one, read its duration trend and cache hits:

```bash
levelrail-cli apps build-stats <fixture-app> --limit 10
```

A second deploy of an unchanged fixture should show a higher cached share than the first. To exercise the limits, run `levelrail-cli apps build-limits set --timeout 1s` and redeploy: the build fails with `build_timeout` and appears in the trend as timed out. Run `levelrail-cli apps build-limits reset` afterwards.

## Trying the GitHub Actions report

To see the deployment status and check run, copy `integrations/github-action/example-workflow.yml` into a repo that holds one of these fixtures, set the two secrets, and push. `levelrail-cli apps github-actions get <fixture-app>` shows what will be reported.

## Trying per-pull-request env overrides

Connect a fixture app to a repo with previews on, set an override for the pull request, then push a commit to that pull request:

```bash
levelrail-cli apps pr-env set <fixture-app> MARKER --pr 7 --value preview-seven
levelrail-cli apps pr-env list <fixture-app> --pr 7
```

The preview serves `preview-seven` while production keeps its own marker. The pull request comment lists the status, URL, build logs link and deploy time, and only counts the overrides. Close the pull request and `levelrail-cli apps pr-env list <fixture-app> --pr 7` comes back empty.

## Reading the pipeline trace of a smoke deploy

Every deploy records a step trace, build or not. After deploying a fixture (an image deploy is the quickest), print it:

```bash
levelrail-cli apps deploys steps <fixture-app> <deploy-id>
```

You should see `resolve_digest`, `image_pull` (or `skipped` with reason `Cached` on a second deploy), `container_start`, one or more `readiness_probe` rows hitting `/healthz`, `cutover` and a closing `deploy_summary`. To see a failure trace, deploy a tag that does not exist: `image_pull` fails with the registry's own message and `deploy_summary` is `failed`.

## What `apps analyze` should print per fixture

Run `levelrail-cli apps analyze <repo-url> --dir <fixture-dir>` against a repository holding the fixtures. Nothing is created. See [Import a repository](../../../docs/import-a-repo.md) for full output.

| Fixture | Framework | Build type | Port (source) | Notable |
| --- | --- | --- | --- | --- |
| `smoke/nextjs` | Next.js | dockerfile | 3000 (EXPOSE) | confidence high, App Router reason |
| `smoke/goapi` | Go (net/http) | dockerfile | 8080 (EXPOSE) | runtime 1.22 from go.mod |
| `smoke/static` | none named | dockerfile | 8080 (EXPOSE) | the Dockerfile decides; without it, `static` on port 80 |
| `dogfood-nextjs` | Next.js | dockerfile | 3000 (EXPOSE) | `output: 'standalone'` reason |
| `monorepo` | none at the root | dockerfile | 3000 (EXPOSE) | `multiple_dockerfiles` and `monorepo_no_root_app` warnings, two packages |
| `failing-build` | Node.js, low signal | dockerfile | none | `dockerfile_no_expose` and `port_unknown` warnings |
