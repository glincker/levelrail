# Known limits

Last updated 2026-11-08 (release/v0.9.5 branch).

Current limits, each with the page it comes from. Labels and evidence change with every release, so check [feature status](feature-status.md) for the latest.

## Release maturity

- There is no stable release yet. Every release so far is a beta. Source: [roadmap](roadmap.md).
- Single node is the stable scope. Multi-node, stacks and flagged features are beta or experimental. Source: [feature status](feature-status.md).

## Multi-node

- The join flow is verified across two real Docker daemons, not across a real WAN. Source: [roadmap](roadmap.md), [multi-node](multi-node.md).
- The WireGuard mesh and cross-host remote transport are unverified outside fakes and local Docker daemons. Source: [roadmap](roadmap.md).
- Multi-cloud node onboarding has no run against a real cloud metadata endpoint. Source: [feature status](feature-status.md).
- Capacity cleanup covers the control plane node only. Remote nodes monitor and block. Source: [feature status](feature-status.md).

## Not verified on real infrastructure

- A stable label means tests plus a live end-to-end run on one machine. It does not mean a run on a fresh production host. Source: [feature status](feature-status.md).
- The build log's builder line (Dockerfile or Railpack, plus the buildpack file warning) is unit tested through its helper only. No real BuildKit build has run with it, because the build client has no seam to run without BuildKit. Source: [importing apps](importing-apps.md), [feature status](feature-status.md).
- Public ACME has one recorded run on a live VPS. Renewal close to expiry, DNS-01 wildcards and a deliberate failure against the production CA are not yet exercised live. Source: [roadmap](roadmap.md), [ACME runbook](acme-verification-runbook.md).
- Idle footprint was measured in a Docker Desktop container. A bare metal or VPS number is still open. Source: [roadmap](roadmap.md).
- Load and failover numbers have no published results yet. Source: [roadmap](roadmap.md), [benchmarks](benchmarks.md).
- The resilience checks are proven in tests. A real process kill under load on a real host and the container restart policy on a real reboot are not proven. Source: [feature status](feature-status.md).
- The 15 vendor notification kinds have never had a real vendor accept a send. Source: [feature status](feature-status.md).
- SAML and SCIM were run against Keycloak and SimpleSAMLphp in a local lab, not against Okta or Entra ID. Source: [feature status](feature-status.md).

## App controls

- The one-off command, per-service replica and restart controls, and the Logs service selector are unit and component tested with mocked fetch. None has run against a live server, and none has a browser check at phone or desktop width. The Logs selector's navigate-on-change path has no test. Source: [feature status](feature-status.md).

## Upgrades

- Upgrade restore points are beta. Systemd and agent reconnection after an upgrade, and a live host run, are not yet proven. Source: [feature status](feature-status.md), [upgrade safely](upgrade-safely.md).
- Restoring across a schema change discards data written since the pre-upgrade snapshot. The attempt records how many minutes that was. Source: [upgrade safely](upgrade-safely.md).
- A release older than your database schema is blocked and not offered. Source: [upgrade safely](upgrade-safely.md).
- After the first verified release signature, an upgrade to a release without a signature bundle is refused. Source: [upgrade safely](upgrade-safely.md).
- Upgrade and downgrade testing across versions with real data is still part of the work toward a stable release. The populated-database SQLite test uses synthetic rows, and the domains table is not seeded yet. Source: [roadmap](roadmap.md).

## Templates and databases

- The last local catalog smoke (2026-10-25, plus eight hand-booted on 2026-10-27) passed 101 of 108 templates that need no operator configuration. The rest of the catalog is untested. Source: [feature status](feature-status.md), [templates status](templates-status.md).
- Point-in-time restore covers Postgres, MySQL and MariaDB only. MongoDB has no oplog here. TLS is built for Postgres, Redis, Valkey, MySQL, MariaDB and MongoDB (on at creation for Postgres, Redis and, from the TLS release on, MySQL, MariaDB and MongoDB; older MySQL, MariaDB and MongoDB databases, Redis and every Valkey database can be switched with `PUT /api/v1/databases/{name}/tls`, and Valkey stays plaintext until switched). KeyDB, Dragonfly, ClickHouse, Meilisearch, Qdrant, Typesense and Memcached have no TLS, and no TLS path, Valkey included, was run on a real host. Source: [roadmap](roadmap.md), [feature status](feature-status.md).
- Database backups for compose services (Postgres, MySQL, MariaDB) have no restore control in the dashboard or CLI. Restore is a manual load of the dump, covered in [compose apps](compose-apps.md). The schedules, run history and dashboard card are built and unit tested, but not yet run against a live server or a real compose app with a database. A restore path is not part of this release. Source: [compose apps](compose-apps.md), [feature status](feature-status.md).

- `databases export` and `databases clone` are built and unit tested with fakes. Nothing has run against a live control plane or a real backup object. Source: [managing databases](managing-databases.md), [feature status](feature-status.md).

## Behind flags or beta

- Seventeen features sit behind the platform gate and are off by default. Ten are experimental: AI models on GPU nodes, the load balancer, platform as code, Cloudflare Tunnel, access roles, global environments, AI control, plugins, mail DNS and remote MCP. Seven are beta: AI assistant chat, resource advisor, CLI console, mobile API, git push deploy, canvas, and SAML and SCIM. Turn one on in Settings, Beta features, with `levelrail-cli settings beta-features set <feature> on`, or list it in `APP_EXPERIMENTAL`. Features wired at server start show "restart to apply" until the control plane restarts. Add `--restart` to the CLI command to restart it at once (root or a supervised install). Source: [experimental features](experimental-features.md), [feature status](feature-status.md).
- `git remote add --apply` and `rotate --apply` write the push token in plain text into the repository's `.git/config`. Anyone who can read that repository can push deploys to the app. The flags are unit tested with fakes and have not run against a live control plane. Source: [git push deploy](git-push-deploy.md).
- The `pause` spend cap action serves a maintenance response and does not stop containers. Source: [roadmap](roadmap.md).
