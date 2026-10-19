---
title: "Plugins"
tag: "Experimental"
description: "Run signed, operator-approved plugins in locked-down containers. The first release supports notification channels."
"og:url": "https://docs.levelrail.com/plugins"
---

A plugin is a separate program that extends the control plane without being loaded into it. It runs in its own container, talks to the control plane over a local socket, and can only do what you approved. It supports three kinds: a notification channel that delivers alert events somewhere the built-in channels do not, a panel provider, and a DNS provider that writes DNS records (see [DNS provider plugins](/plugins-first-party#dns-provider-plugins)).

Plugins are experimental and off by default. Set `APP_EXPERIMENTAL=plugins` and restart (see [experimental features](/experimental-features)). The decisions behind this design are in ADR 034.

## What a plugin can and cannot do

Plugins run third-party code on your server, next to a control plane that holds your master key and every stored credential. The design assumes a plugin may be hostile or buggy:

- It runs in a container with a read-only root filesystem, every Linux capability dropped, no new privileges, a process limit and memory and CPU limits. It runs as an unprivileged user and never gets the Docker socket.
- Its only mount is a private socket directory. It has no access to the control plane database, the master key, your apps' files or other plugins.
- With no declared outbound hosts it has no network at all. With declared hosts it joins a dedicated network that is separate from your apps. The declared hosts are shown to you at approval but are not yet enforced per host, so treat a plugin that declares hosts as able to reach the internet.
- It receives only the event being delivered, its own non-secret config, and the credential for that one call. The control plane sends the credential with each call and never stores it in the plugin. A plugin must not persist or log it.
- It receives alert text under the `read` ability. Log excerpts reach it only if you also approved `read:sensitive`.
- A broken plugin cannot block alerting: calls have a timeout, a concurrency limit and a circuit breaker, and a failing plugin is restarted with backoff.

Every install, approval, enable, disable and removal is written to the audit log, and so is every use of an ability, one entry per delivered event.

## Trust a publisher key

Nothing third party loads until you trust a key. The trust list starts empty, and an install is refused unless its signature verifies against a key on the list.

A publisher signs the plugin's manifest and its image digest with an ed25519 key. To trust a key you have verified out of band:

```bash
levelrail-cli plugins trust add --name "Example Org" --public-key <base64 public key>
levelrail-cli plugins trust list
```

Removing a key stops every plugin it signed at the next supervisor pass:

```bash
levelrail-cli plugins trust remove <key-id>
```

## Install a plugin

A package is two things: an image pinned by digest (`name@sha256:...`, tags are refused) and a `plugin.yaml` manifest, plus a detached signature. The manifest is validated against a JSON Schema (the same rules the dashboard, CLI and API share).

```yaml
api_version: 1
name: example-notify
version: 0.1.0
kind: notification_channel
publisher: Example Org
description: Logs each alert event.
image: registry.example.com/example/plugin-notify@sha256:<64 hex>
abilities: [read]
outbound_hosts: []
contributes:
  panels:
    - id: delivery
      title: Delivery
      description: Where this plugin sends alerts.
      rows:
        - { label: Target, value: Example chat }
```

Inspect first. It verifies the package and shows the publisher, the abilities it asks for, the digest pin and the signature status, and writes nothing. From a file or from an https URL (a `<url>.sig` file next to the manifest is the signature):

```bash
levelrail-cli plugins inspect --manifest plugin.yaml --signature plugin.sig
levelrail-cli plugins inspect --url https://plugins.example.com/notify/plugin.yaml
```

Install records and verifies the package. By default it approves nothing and starts nothing. The signing key is found among your trusted keys, or pass `--key-id`:

```bash
levelrail-cli plugins install --manifest plugin.yaml --signature plugin.sig
levelrail-cli plugins install --url https://plugins.example.com/notify/plugin.yaml --approve read
```

`--approve` is the operator approving the exact ability set at install: it must list exactly the abilities the manifest requests, never fewer or more. In the dashboard, Install plugin shows the same review (publisher, verification, abilities, network) and an Approve and install button. Pass `--digest sha256:...` to refuse the install unless the manifest pins exactly the digest you expect. The install is refused if the signature does not verify, the key is not trusted, the manifest has an unknown field, or it asks for an ability plugins cannot hold.

Unsigned packages are refused. To run one you built yourself, set `APP_PLUGIN_ALLOW_UNSIGNED=true` on the control plane and restart. Unsigned plugins are labelled as such everywhere, and a signed plugin can never be replaced by an unsigned one.

## Approve abilities

Approval is explicit, either at install with the exact set or as a separate step. The dashboard shows exactly what the manifest requests before you confirm, and the CLI will not approve without an explicit list:

```bash
levelrail-cli plugins show example-notify
levelrail-cli plugins approve example-notify --abilities read
levelrail-cli plugins enable example-notify
```

Plugins can currently hold two abilities from the scoped-token vocabulary: `read` (receive the event) and `read:sensitive` (also receive log excerpts). `root` and every write ability are refused. A plugin that asks for something it did not declare in its signed manifest is stopped at the handshake.

## Upgrade, disable, uninstall

```bash
levelrail-cli plugins upgrade --url https://plugins.example.com/notify/plugin.yaml
levelrail-cli plugins disable example-notify
levelrail-cli plugins uninstall example-notify
```

An upgrade is a new signed package for an installed plugin. If it asks for an ability or an outbound host the installed version did not, the plugin is stopped and loses its approval until you approve the wider set (the dashboard shows the added abilities highlighted, and approving there grants exactly the listed set). An upgrade that asks for the same or less keeps your approval and the plugin keeps running on the new image. Use `disable` to stop a plugin and keep its approval, and `uninstall` to delete it. If the container cannot be removed right away the plugin stays in a removing state and cleanup is retried.

## Health and fixes

Each plugin shows its health (healthy, unhealthy, stopped), its verification status (verified, unsigned, key not trusted, not verified) and, when something is wrong, a Fix button on the Plugins page: restart an unhealthy plugin (clears the failure backoff and circuit breaker), jump to the trusted keys when its signing key was removed, approve a plugin that is waiting, or reinstall a signed package when verification no longer passes. `levelrail-cli plugins restart <name>` does the same restart. The MCP tools `list_plugins` and `get_plugin` expose the same data read only.

## Contributed panels

A manifest can contribute panels. A plugin never ships JavaScript, HTML or an iframe into the dashboard: it declares data and a closed set of widgets, and the dashboard renders them with its own components, labelled with the plugin and its publisher.

A static panel is a title, a description and label/value rows of plain text, shown on the Plugins page.

A dynamic panel (plugin kind `panel_provider`) has a `source` and `widgets`. The control plane calls the plugin's `PanelService.PanelData` for the source, binds the returned JSON document to the widgets and sends the dashboard only typed text and numbers. Where it shows is set by `slot`: `plugins` (the default, Settings > Plugins), `app-detail` (the app Overview page, scoped to that app), `settings` (Settings > General) or `dashboard` (the home page).

Widgets: `stat`, `table`, `list`, `chart`, `markdown`, `status` and `button`. Each reads a dotted `field` path from the source's JSON. A button names an `action` and an optional `confirm` text (the dashboard asks before running it) and calls `PanelService.PanelAction`.

```yaml
api_version: 1
name: deploy-panel
version: 1.0.0
kind: panel_provider
publisher: Example Org
image: registry.example.com/team/deploy-panel@sha256:<64 hex>
abilities: [read]
contributes:
  panels:
    - id: deploys
      title: Recent deploys
      slot: app-detail
      source: { id: deploys, abilities: [read] }
      widgets:
        - { type: stat, title: Deploys this week, field: total, unit: deploys }
        - type: table
          title: Latest
          field: rows
          columns: [{ key: sha, label: Commit }, { key: status, label: Status }]
        - { type: markdown, field: note }
        - { type: button, action: refresh, label: Refresh, confirm: Refresh the list now? }
  commands:
    - { id: deploys, description: Show recent deploys, panel: deploys }
    - { id: refresh, description: Refresh the list, panel: deploys, action: refresh }
```

The plugin answers `PanelData` with, for example, `{"total": 12, "rows": [{"sha": "a1b2c3", "status": "ok"}], "note": "All **green**."}`.

What the control plane enforces on every panel call:

- Only an enabled `panel_provider`, only a panel and action the manifest declares, and only abilities the operator approved. A panel source may use `read` only (never `read:sensitive`, never a write ability). Revoking the approval stops the panel immediately.
- A call timeout (`APP_PLUGIN_CALL_TIMEOUT`, default 10 seconds), a concurrency cap and the circuit breaker shared with other plugin calls. Every call is audited as a plugin ability use before it is made, and a failed audit refuses the call.
- A response size cap of 128 KiB, a table cap of 200 rows (the dashboard shows 50 at a time), cell and item length caps, and a nesting limit. Oversize or malformed data is rejected with a clear error, and a missing field fails only its own widget.
- Markdown is reduced to paragraphs, lists, bold, code and http(s) links. Raw HTML, images, entities and `javascript:` or `data:` links are removed on the server, and the dashboard renders what is left as text nodes, never as HTML.
- Scope: an app page passes `app_id`, and the control plane checks the caller may see that app before the plugin ever sees the id.

Each panel shows its own loading, error and Retry state, so one failing plugin never blanks a page.

### Commands

`commands` add CLI subcommands run through the same checks: `levelrail-cli plugins run <plugin> <command> [--app NAME]` prints the panel data (needs `read`) or presses the button named by `action` (needs `write:sensitive`). Add `--json` for machine output.

## Use it as an alert channel

Once a plugin is enabled, add a notification channel whose kind is `plugin:<name>` (for example `plugin:example-notify`). The channel's target field holds the credential the plugin needs, such as a token. It is stored sealed like any other channel credential and sent to the plugin on each call only. Alert rules then deliver through the plugin like any other channel, with the same delivery history. Test sends and deploy notices are not supported for plugin channels yet.

## Write a plugin

The contract is `proto/plugin/v1/plugin.proto`: `Describe` (name, version, kind, requested abilities, config schema, with contract version negotiation), `Health`, for notification channels `Send`, for panel providers `PanelService` (`PanelData`, `PanelAction`), and for DNS providers `DnsProviderService` (`ListZones`, `ListRecords`, `UpsertRecord`, `DeleteRecord`, `VerifyRecord`, each carrying the credential for that call only). The plugin listens on the unix socket path in the `PLUGIN_SOCKET` environment variable. A complete example lives in `test/fixtures/plugin-notify`, including a small signer that creates a key and signs a manifest.

## Marketplace

The Marketplace tab on the Plugins page lists plugins from signed indexes. An index is a JSON file signed by a root key; the control plane only shows listings whose index signature verifies against that source's root key. To publish your own, see [publishing plugins](/plugin-publishing).

Sources: set `APP_MARKETPLACE_URL` and `APP_MARKETPLACE_ROOT_KEY` (base64 ed25519 public key) for a default source, and add more under Marketplace sources in the dashboard or with the CLI. With neither, the tab is empty and nothing is fetched. Fetches are https only, go through the same SSRF guard as every other outbound request, are capped at 1 MiB, cached with an ETag and refreshed once a day or when you press Re-check.

An index that fails its signature, is malformed, is too large, or is older than one the control plane already saw (each index has a `generated_at` and an ever increasing `sequence`) is ignored and shown with a typed error and a Re-check button. A network failure keeps showing the last good copy, marked stale.

Install is one confirmed step. The sheet shows the publisher, the publisher key fingerprint, the image digest, the abilities that will be approved and the outbound hosts. If the publisher key is not on your trust list you must tick "Trust the publisher key" too; the key is added labelled "added from marketplace", and declining does nothing. On confirm the control plane fetches the manifest again, checks that its name, version, digest, abilities and hosts equal the signed listing and that its signature verifies under the publisher key the index vouches for, then installs, approves exactly the abilities the sheet showed and enables. If the listing changed after you opened the sheet, the install is refused and you review again. A failed install removes the key and plugin it added. Installs and updates are written to the audit log.

An update appears as an Update available badge. If it asks for an ability or outbound host the installed version did not, the sheet highlights it and you approve the exact new set; the control plane never approves more than you saw. It refuses to roll a plugin back to an older version and refuses a listing signed by a different publisher key than the installed plugin.

```bash
levelrail-cli plugins marketplace search chat
levelrail-cli plugins marketplace info example-notify
levelrail-cli plugins marketplace install example-notify --approve-abilities read --trust-publisher --yes
levelrail-cli plugins marketplace update --all --approve-abilities read --yes
levelrail-cli plugins marketplace sources add --url https://plugins.example.com/index.json --root-key <base64>
```

`--yes` is required to proceed and `--approve-abilities` must list exactly the abilities `info` shows. The MCP tool `list_marketplace_plugins` lists the same catalog read only; there is no install tool over MCP.

## Not yet

- A hosted public index. The control plane ships with no default source until the project publishes one.
- More UI extension points. Plugins can add panels and CLI commands, but not whole pages or forms yet.
- Auto-update. Upgrades are an explicit operator action.
- CDN providers and other kinds, Route53 as a DNS plugin, and plugins on remote nodes.
- Per-host network enforcement and a supply chain scan of the plugin image.
- Rootless Docker uid mapping for the socket directory.
