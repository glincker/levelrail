---
title: "Plugins"
tag: "Experimental"
description: "Run signed, operator-approved plugins in locked-down containers. The first release supports notification channels."
---

A plugin is a separate program that extends the control plane without being loaded into it. It runs in its own container, talks to the control plane over a local socket, and can only do what you approved. This first release supports one kind: a notification channel that delivers alert events somewhere the built-in channels do not.

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

A manifest can contribute read-only panels (a title, a description and label/value rows of plain text). The dashboard renders them on the Plugins page from the manifest, labelled with the plugin and its publisher. A plugin never ships JavaScript into the dashboard.

## Use it as an alert channel

Once a plugin is enabled, add a notification channel whose kind is `plugin:<name>` (for example `plugin:example-notify`). The channel's target field holds the credential the plugin needs, such as a token. It is stored sealed like any other channel credential and sent to the plugin on each call only. Alert rules then deliver through the plugin like any other channel, with the same delivery history. Test sends and deploy notices are not supported for plugin channels yet.

## Write a plugin

The contract is `proto/plugin/v1/plugin.proto`: `Describe` (name, version, kind, requested abilities, config schema, with contract version negotiation), `Health`, and for notification channels `Send`. The plugin listens on the unix socket path in the `PLUGIN_SOCKET` environment variable. A complete example lives in `test/fixtures/plugin-notify`, including a small signer that creates a key and signs a manifest.

## Not yet

- A marketplace or any hosted index of plugins. You install a package you obtained yourself.
- Richer UI extension points. Panels are static text rows; plugins cannot add pages, forms or CLI commands yet.
- Auto-update. Upgrades are an explicit operator action.
- Other plugin kinds such as DNS or CDN providers, and plugins on remote nodes.
- Per-host network enforcement and a supply chain scan of the plugin image.
- Rootless Docker uid mapping for the socket directory.
