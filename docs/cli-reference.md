---
description: Complete reference of all Levelrail CLI commands, organized by group with examples and typical use cases.
---

# Levelrail CLI Reference

Exhaustive reference of all Levelrail CLI commands, organized by command group and extracted directly from the source code.

## See also

- [Getting Started](getting-started.md) - First steps with Levelrail
- [Feature Catalog](feature-catalog.md) - Complete feature overview
- [App Spec Reference](app-spec-reference.md) - YAML configuration syntax

## Apps

```
levelrail apps alerts create <app> --name NAME --kind threshold --metric METRIC --comparator OP --threshold N [flags]
```

```
levelrail apps alerts delete <app> <id> [flags]
```

```
levelrail apps alerts list <app> [flags]
```

```
levelrail apps alerts update <app> <id> --name NAME --kind threshold --metric METRIC --comparator OP --threshold N [flags]
```

```
levelrail apps auto-rollback enable <app-name> [flags]
```

```
levelrail apps auto-rollback disable <app-name> [flags]
```

```
levelrail apps auto-rollback status <app-name> [flags]
```

```
levelrail apps health get <name> [flags]
```

```
levelrail apps health set <name> --probe readiness|liveness (--path PATH | --exec CMD) [--scheme https] [--host HOST] [--tls-skip-verify] [--follow-redirects true|false] [--expected-status 200-399] [--interval 5s] [--timeout 2s] [--failures 3] [--ready-timeout 90s] [flags]
```

```
levelrail apps health clear <name> [--probe readiness|liveness] [flags]
```

```
levelrail apps builds trigger <name> --repo URL --ref REF [flags]
```
build an image from a git source and deploy it to an existing app

```
levelrail apps clear-environment <name> [flags]
```

```
levelrail apps clear-project <name> [flags]
```

```
levelrail apps clone <name> <new-name> [flags]
```

```
levelrail apps create --name NAME --image IMAGE --port PORT [flags]
```

```
levelrail apps create [flags]
```
create an app (existing image, git build, --file, or --interactive)

```
levelrail apps database set <name> --database-name NAME [flags]
```
attach an already-created managed database to `<name>` as its connection-env-var source

```
levelrail apps database clear <name> [flags]
```
detach the database `<name>` currently resolves its connection env var from

```
levelrail apps delete <name> [flags]
```

```
levelrail apps deploy <name> --image IMAGE [flags]
```

```
levelrail apps wait <name> [flags]
```
poll until a deploy attempt actually converges, exit accordingly (a CI gate for "apps deploy")

```
levelrail apps deploy-compose <name> --file compose.yaml [flags]
```

```
levelrail apps deploy-notify-targets create <app> --channel-id ID [flags]
```

```
levelrail apps deploy-notify-targets delete <app> <id> [flags]
```

```
levelrail apps deploy-notify-targets list <app> [flags]
```

```
levelrail apps deploy-spec <name> --file app.yaml --repo-url <url> --ref <ref> [flags]
```

```
levelrail apps deploys list <name> [flags]
```
real, row-per-attempt deploy history, newest first

```
levelrail apps deploys compare <name> --from ID [--to ID] [flags]
```
diff two deploy attempts, or one against the current live state

```
levelrail apps deploys logs <name> <deploy-id> [flags]
```
one deploy attempt's full build/log output, printed to stdout (redirect to a file to save it)

```
levelrail deploy-approvals list [--status pending|all|approved|rejected|expired] [--service NAME] [flags]
```
list deploy approvals (status defaults to pending)

```
levelrail deploy-approvals get <id> [flags]
```

```
levelrail deploy-approvals approve <id> [flags]
```
approve a pending deploy; the gated deploy/promote runs now

```
levelrail deploy-approvals reject <id> [--reason TEXT] [flags]
```
reject a pending deploy; the app's desired state is left untouched

```
levelrail apps environments clone <id> --new-name NAME [--app-rename SOURCE=NEWNAME ...] [--domain SOURCE=D1,D2 ...] [--copy-secret-values] [flags]
```
clone a whole environment's app set plus config into a new environment

```
levelrail apps environments clone-preview <id> --new-name NAME [flags]
```
preview what cloning an environment would create, without applying it

```
levelrail apps environments create <project-id> --name NAME [--protected] [flags]
```
create an environment under a project

```
levelrail apps environments delete <id> [flags]
```

```
levelrail apps environments env-get <id> [flags]
```

```
levelrail apps environments env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]
```

```
levelrail apps environments list <project-id> [flags]
```

```
levelrail apps environments update <id> --protected=true|false [flags]
```

```
levelrail apps exec <name> -- <command> [args...] [flags]
```

```
levelrail apps git-source get <name> [flags]
```
show an app's connected repo

```
levelrail apps images <name> [flags]
```

```
levelrail apps log-drain get <name> [flags]
```
show an app's configured log drain

```
levelrail apps logs <name> [flags]
```

```
levelrail apps metrics <name> --metric NAME [flags]
```

```
levelrail apps moves list <name> [flags]
```
list every node-to-node move attempt for `<name>`, newest first

```
levelrail apps moves get <name> <id> [flags]
```
show one move attempt's step-by-step progress

```
levelrail apps organizations clear-project <project-id> [flags]
```

```
levelrail apps organizations create --name NAME [flags]
```
create an organization

```
levelrail apps organizations delete <id> [flags]
```

```
levelrail apps organizations env-get <id> [flags]
```

```
levelrail apps organizations env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]
```

```
levelrail apps organizations get <id> [flags]
```

```
levelrail apps organizations list [flags]
```

```
levelrail apps organizations set-project <project-id> <org-id> [flags]
```

```
levelrail apps preview-env set <name> <key> --value VALUE [flags]
```
declare (or replace) a preview-specific env var override

```
levelrail apps preview-env clear <name> <key> [flags]
```
remove a preview-specific env var override

```
levelrail apps branch-env list <name> [flags]
```
list an app's branch-scoped env var overrides

```
levelrail apps branch-env set <name> <key> --branch PATTERN --value VALUE [--secret] [flags]
```
declare (or replace) a branch-scoped env var override, applied only when a preview's own branch matches PATTERN

```
levelrail apps branch-env clear <name> <id> [flags]
```
remove one branch-scoped override by its id (from `list` or `set`)

```
levelrail apps previews list <app-name> [flags]
```
list active previews for an app

```
levelrail apps previews pr-status enable <app-name> [flags]
```

```
levelrail apps previews sweep [flags]
```

```
levelrail apps previews teardown <app-name> <pr-number> [flags]
```

```
levelrail apps projects create --name NAME [flags]
```
create a project

```
levelrail apps projects delete <id> [flags]
```

```
levelrail apps projects env-get <id> [flags]
```

```
levelrail apps projects env-set <id> --var KEY=VALUE [--var KEY=VALUE ...] [flags]
```

```
levelrail apps projects get <id> [flags]
```

```
levelrail apps projects list [flags]
```

```
levelrail apps projects restart <id> [flags]
```

```
levelrail apps projects start <id> [flags]
```

```
levelrail apps projects stop <id> [flags]
```

```
levelrail apps promote <name> --to ENVIRONMENT_ID [--target NAME] [--preview] [flags]
```

```
levelrail apps restart <name> [flags]
```

```
levelrail apps rollback <name> --image IMAGE [flags]
```

```
levelrail apps scheduled-tasks create <app> --schedule CRON [--disabled] -- <command> [args...]
```

```
levelrail apps scheduled-tasks delete <app> <id> [flags]
```

```
levelrail apps scheduled-tasks get <app> <id> [flags]
```

```
levelrail apps scheduled-tasks list <app> [flags]
```

```
levelrail apps scheduled-tasks run <app> <id> [flags]
```

```
levelrail apps scheduled-tasks update <app> <id> --schedule CRON [--disabled] -- <command> [args...]
```

```
levelrail apps secrets list <name> [flags]
```
list an app's secret keys and their locked state

```
levelrail apps secrets set <name> <key> <value> [flags]
```
set or rotate one secret's encrypted value

```
levelrail apps secrets set <name> --env-file <path> [flags]
```
bulk-import every key in a .env-format file as its own secret

```
levelrail apps secrets lock <name> <key> --locked=true|false [flags]
```
toggle a secret's overwrite guard

```
levelrail apps set-environment <name> <environment-id> [flags]
```

```
levelrail apps set-project <name> <project-id> [flags]
```

```
levelrail apps start <name> [flags]
```

```
levelrail apps stop <name> [flags]
```

```
levelrail apps storage set <name> --storage-target-id ID [flags]
```
attach a connected bucket as object storage

```
levelrail apps vault-env set <name> <key> --path PATH --key FIELD [flags]
```
declare (or replace) a Vault-sourced env var

```
levelrail apps vault-env clear <name> <key> [flags]
```
remove a Vault-sourced env var declaration

```
levelrail apps webhook-deliveries list <app-name> [flags]
```
list recent inbound webhook requests

```
levelrail apps webhook-deliveries replay <app-name> <delivery-id> [flags]
```

```
levelrail apps tag <name> <tag> [flags]
```
attach a tag (by name) to an app, creating the tag if it doesn't exist

```
levelrail apps untag <name> <tag> [flags]
```
detach a tag (by name) from an app

## Tags

```
levelrail tags list [flags]
```

```
levelrail tags create --name NAME [flags]
```

```
levelrail tags delete <name> [flags]
```
delete a tag, identified by name (detaches from all apps)

```
levelrail tags apps <name> [flags]
```
list every app attached to a tag, identified by name

## Databases

```
levelrail databases clear-project <name> [flags]
```

```
levelrail databases create --name NAME --engine ENGINE --version VERSION [flags]
```

```
levelrail databases create [flags]
```
create a managed database

```
levelrail databases delete <name> [flags]
```

```
levelrail databases metrics <name> --metric NAME [flags]
```

```
levelrail databases public-access set <name> [--port N] [--bind-address ADDR] [flags]
```
expose a database on a host port; --bind-address is "private" (default), "public", or a literal IP

```
levelrail databases public-access clear <name> [flags]
```

```
levelrail databases set-resources <name> [--memory 512Mi] [--cpu 0.5] [flags]
```
applies memory/CPU limits to an already-created database, replacing whatever was set before (full replace, not a patch)

```
levelrail databases set-project <name> <project-id> [flags]
```

```
levelrail databases start <name> [flags]
```

```
levelrail databases stop <name> [flags]
```

## Auth

```
levelrail auth 2fa disable --code CODE|--recovery-code CODE [flags]
```

```
levelrail auth 2fa enable --code CODE [flags]
```

```
levelrail auth 2fa recovery-codes --code CODE [flags]
```

```
levelrail auth 2fa setup [flags]
```

```
levelrail auth 2fa status [flags]
```
show whether two-factor auth is enabled

```
levelrail auth login [flags]
```
authenticate and persist a new API token

```
levelrail auth whoami [flags]
```

## Profile

```
levelrail profile list [flags]
```
list configured credentials profiles

## Tokens

```
levelrail tokens create --name NAME --abilities LIST [flags]
```
mint a new API token

```
levelrail tokens list [flags]
```

```
levelrail tokens revoke <id> [flags]
```

## Domains

```
levelrail domains basic-auth get <app> <domain> [flags]
```
show a domain's basic auth state

```
levelrail domains check <app> <domain> [flags]
```

```
levelrail domains cloudflare-dns get [flags]
```
show the current settings

```
levelrail domains route53-dns get [flags]
```
show the current settings

```
levelrail domains list [flags]
```
list every app's domains in one call

```
levelrail domains maintenance get <app> <domain> [flags]
```
show a domain's maintenance state

```
levelrail domains redirect get <app> <domain> [flags]
```
show a domain's redirect state

```
levelrail domains tls-cert get <app> <domain> [flags]
```
show a domain's BYO certificate state

```
levelrail domains waf get <app> <domain> [flags]
```
show a domain's WAF and rate-limit state

```
levelrail domains error-pages get <app> <domain> [--code N] [flags]
```
 show a domain's custom error pages

## Backups

```
levelrail backups list <database> [flags]
```
list backup history for a database

```
levelrail backups list-all [flags]
```
list backup history across every database and app volume instance-wide

```
levelrail backups restore <database> --backup ID [--confirm NAME] [flags]
```

```
levelrail backups restore-as-new <database> --backup ID --new-name NAME [flags]
```

```
levelrail backups schedule set <database> --target ID --cron EXPR [flags]
```
 configure a recurring backup

```
levelrail backups trigger <database> --target ID [flags]
```

```
levelrail backups verifications <database> --backup ID [flags]
```

```
levelrail backups verify <database> --backup ID [flags]
```

## App Volume Backups

```
levelrail app-volume-backups list <app> <volume> [flags]
```
list backup history for an app's named volume

```
levelrail app-volume-backups restore <app> <volume> --backup ID [--confirm APP/VOLUME] [flags]
```

```
levelrail app-volume-backups restore-as-new <app> <volume> --backup ID [--new-volume-name NAME] [flags]
```

```
levelrail app-volume-backups schedule set <app> <volume> --target ID --cron EXPR [flags]
```
 configure a recurring backup

```
levelrail app-volume-backups trigger <app> <volume> --target ID [flags]
```

```
levelrail app-volume-backups verifications <app> <volume> --backup ID [flags]
```

```
levelrail app-volume-backups verify <app> <volume> --backup ID [flags]
```

## Cloudflare Tunnel

```
levelrail cloudflare-tunnel get [flags]
```
show the current settings and connection status

## Vault

```
levelrail vault get [flags]
```
show the current external Vault integration settings

```
levelrail vault set --address URL --auth-method token|approle [flags]
```
configure and enable resolving app secrets from an external HashiCorp Vault instance

```
levelrail vault disconnect [flags]
```
disable and forget the stored credential

## Channels

```
levelrail channels create --name NAME --kind KIND --notify-url URL [flags]
```

```
levelrail channels delete <id> [flags]
```

```
levelrail channels deliveries <id> [flags]
```

```
levelrail channels list [flags]
```
list connected notification channels

```
levelrail channels test <id> [flags]
```

```
levelrail channels update <id> --name NAME --kind KIND --notify-url URL [flags]
```

## Backup Targets

```
levelrail backup-targets create --name NAME --provider PROVIDER --bucket BUCKET --access-key-id ID --secret-access-key KEY [flags]
```

```
levelrail backup-targets delete <id> [flags]
```

```
levelrail backup-targets get <id> [flags]
```

```
levelrail backup-targets list [flags]
```
list connected backup targets

```
levelrail backup-targets test <id> [flags]
```

```
levelrail backup-targets update <id> --name NAME --provider PROVIDER --bucket BUCKET [flags]
```

## Registry Credentials

```
levelrail registry-credentials create --name NAME --registry-host HOST --username USER --password PASS [flags]
```

```
levelrail registry-credentials delete <id> [flags]
```

```
levelrail registry-credentials get <id> [flags]
```

```
levelrail registry-credentials list [flags]
```
list connected registry credentials

```
levelrail registry-credentials repositories <id> [flags]
```

```
levelrail registry-credentials tags <id> <repository> [flags]
```

```
levelrail registry-credentials test <id> [flags]
```

```
levelrail registry-credentials update <id> --name NAME --registry-host HOST --username USER [flags]
```

## Registry

```
levelrail registry status [flags]
```
show the current settings and container status

## Flags

```
levelrail flags create <app> --key KEY --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]
```

```
levelrail flags delete <app> <id> [flags]
```

```
levelrail flags get <app> <id> [flags]
```

```
levelrail flags list <app> [flags]
```

```
levelrail flags set <app> <id> --name NAME [--description DESC] [--disabled] [--rollout PERCENT] [flags]
```

## Nodes

```
levelrail nodes delete <id> [flags]
```

```
levelrail nodes drain <id> [--target NODE-ID] [flags]
```

```
levelrail nodes get <id> [flags]
```

```
levelrail nodes health <id> [flags]
```

```
levelrail nodes join-token [flags]
```

```
levelrail nodes list [flags]
```

```
levelrail nodes list [flags]
```
list every node

```
levelrail nodes metrics <id> --metric NAME [flags]
```

```
levelrail nodes patch-status <id> [flags]
```

```
levelrail nodes workloads <id> --accepts-app=BOOL --accepts-build=BOOL [flags]
```

## Status

```
levelrail status [flags]
```

## Version

```
levelrail version [flags]
```

::: details Audit Log and Audit Purge (administrative)

### Audit Log

```
levelrail audit-log [flags]
```

### Audit Purge

```
levelrail audit-purge [flags]
```

:::

::: details Doctor (troubleshooting)

### Doctor

```
levelrail doctor [flags]
```

:::

::: details Containers (low-level)

### Containers

```
levelrail containers [flags]
```

:::

::: details System Maintenance (fleet-wide cleanup, requires an admin/root-scoped token)

### System Prune

```
levelrail system-prune [flags]
```

Removes every stopped container, dangling image, and unused anonymous
volume or build cache not part of the reconciler's current desired
state, fleet-wide. Never touches a named volume (an app's storage
attachment, a database's data volume), even one that's actually
orphaned: see Orphaned Volumes below for those.

### Orphaned Volumes

```
levelrail volumes-orphaned [flags]
levelrail volumes-orphaned-cleanup --names name1,name2 [flags]
```

Named Docker volumes (an app's storage attachment, a database's data
volume) survive `system-prune` even after the app or database that
created them is deleted, since Docker never removes a named volume on
its own. `volumes-orphaned` lists every one this instance created that
no current app, database, or storage attachment references any more.
`volumes-orphaned-cleanup` removes exactly the volumes named with
`--names` (comma-separated), after the control plane re-confirms each
one is still genuinely orphaned; there is no flag that deletes every
currently orphaned volume sight unseen, review the list first.

:::

## Users

```
levelrail invites create --email EMAIL --role ROLE [flags]
```
 invite a new teammate

```
levelrail users create --email EMAIL --password PASSWORD --role ROLE [flags]
```

```
levelrail users delete <id> [flags]
```

```
levelrail users list [flags]
```
list every user

```
levelrail users roles [flags]
```

```
levelrail users set-abilities <id> --role ROLE [flags]
```

## Iam

```
levelrail iam policies <verb> [flags]
```

```
levelrail iam policies attach <id> --principal-type TYPE --principal-id ID [flags]
```

```
levelrail iam policies attachments <id> [flags]
```

```
levelrail iam policies create --name NAME --document DOC [flags]
```
create a policy

```
levelrail iam policies delete <id> [flags]
```

```
levelrail iam policies detach <id> --principal-type TYPE --principal-id ID [flags]
```

```
levelrail iam policies get <id> [flags]
```

```
levelrail iam policies list [flags]
```

```
levelrail iam policies update <id> --name NAME --document DOC [flags]
```

## Secrets

```
levelrail secrets rotate-master-key --new-key-file PATH [flags]
```

::: details Migrate (one-time platform migration)

### Migrate

```
levelrail migrate caprover --url URL --token TOKEN [flags]
```

```
levelrail migrate coolify --url URL --token TOKEN [flags]
```
migrate apps from a Coolify instance

```
levelrail migrate dokploy --url URL --token TOKEN [flags]
```

:::

::: details Completion (shell setup)

```
levelrail completion bash
```
print a bash completion script

:::

::: details Settings (system configuration)

### Settings

```
levelrail settings email get [flags]
```

```
levelrail settings ingress get [flags]
```

```
levelrail settings dashboard-url get [flags]
```
shows the public dashboard URL

```
levelrail settings dashboard-url set --url URL [flags]
```
sets it; once it is `https://`, sign-in over plain HTTP is refused (`--url ""` clears it)

```
levelrail settings oauth list [flags]
```
show every OAuth sign-in provider's current settings

```
levelrail settings ai-assistant get [flags]
```
shows the current AI assistant settings (the key itself is never returned, only whether one is stored)

```
levelrail settings ai-assistant set --model NAME --api-key KEY [flags]
```
configures the AI assistant

```
levelrail settings ai-assistant clear [flags]
```
clears the stored key and resets provider/model

:::

::: details Git Integrations (Github, Gitlab, Bitbucket, Gitea setup)

```
levelrail git-providers [flags]
```
connection status and capabilities (list branches, register a webhook, authenticated clone) for github, gitlab, bitbucket, and gitea in one call

### Github App

```
levelrail github-app status [flags]
```

```
levelrail github-app disconnect [flags]
```
forgets the stored connection locally; does not uninstall or delete the App on GitHub's own side

```
levelrail github-app repos [flags]
```
list repos the connected installation can access

### Gitlab App

```
levelrail gitlab-app status [flags]
```

```
levelrail gitlab-app disconnect [flags]
```
forgets the stored connection locally; does not revoke the token or delete the Application on GitLab's own side

```
levelrail gitlab-app projects [flags]
```
list projects the connected account can access

### Bitbucket App

```
levelrail bitbucket-app status [flags]
```

```
levelrail bitbucket-app disconnect [flags]
```
forgets the stored connection locally; does not revoke the token or delete the consumer on Bitbucket's own side

```
levelrail bitbucket-app repos [flags]
```
list repos the connected account can access

### Gitea App

```
levelrail gitea-app status [flags]
```

```
levelrail gitea-app disconnect [flags]
```
forgets the stored connection locally; does not revoke the token or delete the application on Gitea's own side

```
levelrail gitea-app repos [flags]
```
list repos the connected account can access

:::

## Templates

```
levelrail templates list [flags]
```
browse the curated service catalog

## Static Sites

```
levelrail static-sites list [flags]
```

