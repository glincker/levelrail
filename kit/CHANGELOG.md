# Changelog

## [0.8.12](https://github.com/glincker/levelrail/compare/kit/v0.8.1...kit/v0.8.12) (2026-10-31)


### Features

* analysis reports on demand with digest evidence, right-sizing thresholds read from env, and a per-app report list ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* CLI suggests the closest command or flag on a typo, explains empty lists and adds doctor paths and cleaner flag errors ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* DNS host detection wizard with live record status, zone and delegation checks, and Platform settings split into searchable pages with a DNS providers page ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* edit an app's rolling update budget from deploy settings ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* experimental managed mail server with guided setup, DNS records, mailboxes, aliases, queue, certificates, backup and restore, and system email through your own server ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* experimental remote MCP endpoint with an OAuth 2.1 authorization server, a consent page, a connected clients section in CLI access and mcp-clients CLI verbs ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* first-run polish with a compact Get started, one-click starts on the empty Apps page, visible template Deploy buttons and a clearer framework picker ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* internal incidents opened from confirmed monitor outages, with draft then explicit publish, an operator incidents view, API and CLI, and signed webhooks to status page subscribers ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* local backup targets default to a private folder under the data directory and refuse writes on a full disk ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* match running images against a verified advisory feed and refresh affected apps onto the newest digest, with a Security Advisories tab and an advisories CLI ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* plugin egress proxy core and APP_ prefixed plugin and pipeline settings ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* plugins can take their allowed host from operator config, and the approval screens say what network access a plugin gets ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* upgrade --dry-run and a visual upgrade preview with disk and backup checks, and an upgrade is refused when its pre-upgrade backup fails unless skipped ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* uptime monitors with confirmed failures, cron heartbeat and TLS expiry checks, a monitor_down alert rule and live monitors on the status page ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))


### Bug Fixes

* clip the Apps table node column and show realistic image digests in demo data ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* dashboard polish across incidents, consent, advisories, rollout, monitors, security tabs, upgrade card, backups, domains, login, deployments, templates and the Apps header, with clearer errors, retry actions, readable contrast, a single-row top bar and layouts that hold on phones ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* security and reliability hardening across sign-in, deploys, builds, storage and installs ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* show only published incidents and public updates on public status surfaces and open incidents past the global threshold ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* translate the app overview, deployments page and shell panels and show a real deployments empty state ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* translate the navigation, status page admin, security center, access, audit, SSO, teams, firewall, platform settings, databases, nodes, updates, backups, templates, attention and cleanup pages to Spanish ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* verify image digests on the direct promote path and treat private registry auth failures as inconclusive ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))


### Performance

* lower boot peak memory from 229 MB to 157 MB and index the busiest read paths, with failed deploys lists up to 75 times faster ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))
* split the dashboard first load, 1794 kB down to 1642 kB, and keep i18next on 26.5.3 so the vendor chunk stays 43 kB smaller ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))


### Miscellaneous

* release v0.8.12 ([8fa3a0b](https://github.com/glincker/levelrail/commit/8fa3a0b6c3000679443feb2cb8b6bbcad4f3f33e))

## [0.8.1](https://github.com/glincker/levelrail/compare/kit/v0.8.0...kit/v0.8.1) (2026-10-26)


### Performance

* skip weekend ([#1390](https://github.com/glincker/levelrail/issues/1390)) ([b9a361e](https://github.com/glincker/levelrail/commit/b9a361ee53746945b68447ee3dc8c4b5da5252f2))

## [0.8.0](https://github.com/glincker/levelrail/compare/kit/v0.4.0...kit/v0.8.0) (2026-10-21)


### Features

* 0.8 with inbox, upgrade recovery, secret rotation and zone spread ([#1316](https://github.com/glincker/levelrail/issues/1316)) ([fec2538](https://github.com/glincker/levelrail/commit/fec2538615e5bdcb25169ccf852afb6a03140d64))


### Miscellaneous

* release 0.8.0 ([#1320](https://github.com/glincker/levelrail/issues/1320)) ([b53dcba](https://github.com/glincker/levelrail/commit/b53dcbaf27f806e44ad9cfb43c3048a820ad0f34))

## [0.4.0](https://github.com/glincker/levelrail/compare/kit/v0.3.0...kit/v0.4.0) (2026-10-20)


### Features

* release 0.13 with safer defaults, a plugin marketplace and one search across everything ([#1291](https://github.com/glincker/levelrail/issues/1291)) ([8bfd8c7](https://github.com/glincker/levelrail/commit/8bfd8c7b22747a8a0fc007194c9b1ce9aa4f558f))
* release 0.14 with easier migration, one-command deploys, git push deploys and stronger SSO ([#1294](https://github.com/glincker/levelrail/issues/1294)) ([6a2bafe](https://github.com/glincker/levelrail/commit/6a2bafe173ca88222c11d2b943f4b18a55d268aa))

## [0.3.0](https://github.com/glincker/levelrail/compare/kit/v0.2.1...kit/v0.3.0) (2026-10-17)


### Features

* release notes between deploys, monorepo detection, and build performance stats ([#1253](https://github.com/glincker/levelrail/issues/1253)) ([af1a9db](https://github.com/glincker/levelrail/commit/af1a9db81b4346b16dfde1a98f06bd19f7266364))

## [0.2.1](https://github.com/glincker/levelrail/compare/kit/v0.2.0...kit/v0.2.1) (2026-10-12)


### Bug Fixes

* updates channel resolution, Edge dead end and Cloud Run guides for launch ([#1175](https://github.com/glincker/levelrail/issues/1175)) ([8d9f783](https://github.com/glincker/levelrail/commit/8d9f7839eaeca17ec5c7e67228c754c0f4d8d340))

## [0.2.0](https://github.com/glincker/levelrail/compare/kit/v0.1.0...kit/v0.2.0) (2026-10-12)


### Features

* 0.6 upgrades, dashboard v2, standalone domains, secrets v2 and reliability fixes ([#1164](https://github.com/glincker/levelrail/issues/1164)) ([0c6fd6c](https://github.com/glincker/levelrail/commit/0c6fd6c0ac4737981691c9cbc43786278108d48d))
* upgrade blockers with fixes, upgrade now, scheduled and automatic upgrades ([70b677c](https://github.com/glincker/levelrail/commit/70b677c68e8476caec42a48c4356b7f9fac447ea))


### Bug Fixes

* ignore nested module tags when picking the latest platform release ([#1158](https://github.com/glincker/levelrail/issues/1158)) ([843d71a](https://github.com/glincker/levelrail/commit/843d71a5704abb2c799112faa583effaebf66c1a))
* merge edge channel modelling and stale incident fixes with scheduled upgrades ([63c2390](https://github.com/glincker/levelrail/commit/63c23901e61f34045222d1202b6ee5fcfdcef17c))
* stale data copy incidents, plain-language copy failures, and edge channel update messaging ([1109cee](https://github.com/glincker/levelrail/commit/1109ceeacbba27c7234b6064f5a71004aa55d4af))
* tcp readiness fallback, server help flags, backup target validation, version stamp ([cb01128](https://github.com/glincker/levelrail/commit/cb01128313e239338a9550605540826e57cebb5b))

## [0.1.0](https://github.com/glincker/levelrail/compare/kit/v0.1.0...kit/v0.1.0) (2026-10-11)


### Bug Fixes

* ignore nested module tags when picking the latest platform release ([#1158](https://github.com/glincker/levelrail/issues/1158)) ([843d71a](https://github.com/glincker/levelrail/commit/843d71a5704abb2c799112faa583effaebf66c1a))

## 0.1.0 (2026-10-11)


### Features

* attention center, device login approval, setup redesign, release rollback ([#1077](https://github.com/glincker/levelrail/issues/1077)) ([3e5571d](https://github.com/glincker/levelrail/commit/3e5571d8ec1ae38b1ed1174ca89970ba46845291))
* extract generic packages into a nested kit module ([#1014](https://github.com/glincker/levelrail/issues/1014)) ([6c6ddda](https://github.com/glincker/levelrail/commit/6c6dddaf16fbbc9e5f02ffef8cda10056293886d))
* move upgrade into kit and brand-drive agent provisioning names ([#1020](https://github.com/glincker/levelrail/issues/1020)) ([63d654e](https://github.com/glincker/levelrail/commit/63d654e72d4e231772cca3880f9a0863d47a39dc))


### Bug Fixes

* findings from real-server QA (sign-in, firewall, redeploy, templates, status UI) ([#1043](https://github.com/glincker/levelrail/issues/1043)) ([037b89d](https://github.com/glincker/levelrail/commit/037b89d10bf34889cf97f18b59069f30c49d7953))


### Performance

* Skip weekend run ([#724](https://github.com/glincker/levelrail/issues/724)) ([e0097fd](https://github.com/glincker/levelrail/commit/e0097fd84bfbb355bf2cb5202d0bbadf5bcdfed3))
