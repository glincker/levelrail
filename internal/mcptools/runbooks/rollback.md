# Runbook: rollback

Two different things share the word. Ask which one the user means.

## An app deploy went bad

1. Read first: list_deploy_attempts, get_deploy (the failed one), list_app_images.
2. Pick a past succeeded deploy. Prefer rollback_app_to_deploy (pinned by image digest, fails closed if the image was garbage collected or the tag moved) over rollback_app (tag only).
3. Protected environments need confirm and then a second human approval.
4. After the user confirms, run it, then wait_for_deploy or get_app_status until healthy.

CLI: `levelrail-cli apps deploys rollback-to <app> <deploy-id> --watch`

Auto rollback on a crashloop alert is opt in per app: `levelrail-cli apps auto-rollback enable <app>`.

## The control plane itself needs to go back

Nothing is applied from the dashboard or MCP. Read the plan (get_rollback_plan), then the operator runs it on the server:

```
levelrail-cli upgrade --list --channel all
levelrail-cli upgrade --rollback-plan <version> --json
sudo levelrail rollback --to <version> --dry-run
sudo levelrail rollback --to <version>
```

Verdicts: "Safe: binary only" keeps data. "Needs a backup restore" permanently loses everything written after that backup, so never suggest it without the operator confirming the data loss.
