---
name: levelrail-config
description: Set environment variables and secrets for a Levelrail app without exposing values. Use when asked to import a .env file, set a secret, or apply config changes.
---

# Env vars and secrets

- Plain config: `levelrail-cli apps env import <app> --file .env --dry-run`, then again without `--dry-run`.
- Secrets: put them in a git-ignored file and run `levelrail-cli apps secrets set <app> --env-file <path>`, so values never appear in shell history or chat. Do not read secret values back or print them.
- Env changes take effect on the next restart: `levelrail-cli apps apply <app>`.

Command details: [apps](../levelrail/references/commands/apps.md).
