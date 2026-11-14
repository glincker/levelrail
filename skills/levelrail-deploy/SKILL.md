---
name: levelrail-deploy
description: Deploy or redeploy an app on a self-hosted Levelrail instance and wait for it to converge. Use when asked to ship, deploy, redeploy, preview a spec change, or check that a deploy worked.
---

# Deploy with Levelrail

Needs `APP_API_URL` and `APP_API_TOKEN` set (see the `levelrail` skill). Never print or store the token.

1. Inspect the project first: `levelrail-cli apps detect --json` (offline) shows the language, build method and port. If there is no `app.yaml`, run `levelrail-cli init`.
2. Commit and push. A push to the tracked branch deploys through the webhook. To deploy by hand: `levelrail-cli apps deploy <app>`.
3. Preview a spec change without applying it: `levelrail-cli apps preflight <app>` (MCP: `preflight_app`, `plan_change`).
4. Do not assume a deploy worked. Block until it converges: `levelrail-cli apps wait <app>`. The exit code says whether it succeeded (MCP: `wait_for_deploy`).
5. If it failed, switch to the `levelrail-diagnose` skill.

Command details: [apps](../levelrail/references/commands/apps.md).
