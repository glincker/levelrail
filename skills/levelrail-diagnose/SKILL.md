---
name: levelrail-diagnose
description: Diagnose a failed or unhealthy Levelrail app and roll back. Use when a deploy failed, an app is down, logs or status are needed, or production needs a rollback.
---

# Diagnose and roll back

```
levelrail-cli apps status <app>
levelrail-cli attention
levelrail-cli apps deploys list <app>
levelrail-cli apps deploys show <app> <deploy-id>
levelrail-cli apps diagnose <app>
levelrail-cli apps logs <app> --follow
```

`apps deploys show` returns the structured failure (stage, reason, suggested fix). Read it before changing anything, and change one thing per attempt. Add `--json` for machine-readable output.

## Roll back

```
levelrail-cli apps deploys list <app>
levelrail-cli apps deploys rollback-to <app> <deploy-id>
```

Roll back first when production is down, then investigate. Stay inside the app you were asked about.

Command details: [apps](../levelrail/references/commands/apps.md).
