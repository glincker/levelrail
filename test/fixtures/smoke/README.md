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
