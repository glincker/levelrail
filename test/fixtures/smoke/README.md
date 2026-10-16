Minimal fixtures for scripts/vps-smoke.sh. Each serves its MARKER file at / and "ok" at /healthz.

## Reading build stats after a smoke deploy

Each deploy of a fixture records one build. After deploying one, read its duration trend and cache hits:

```bash
levelrail-cli apps build-stats <fixture-app> --limit 10
```

A second deploy of an unchanged fixture should show a higher cached share than the first. To exercise the limits, run `levelrail-cli apps build-limits set --timeout 1s` and redeploy: the build fails with `build_timeout` and appears in the trend as timed out. Run `levelrail-cli apps build-limits reset` afterwards.

## Trying the GitHub Actions report

To see the deployment status and check run, copy `integrations/github-action/example-workflow.yml` into a repo that holds one of these fixtures, set the two secrets, and push. `levelrail-cli apps github-actions get <fixture-app>` shows what will be reported.
