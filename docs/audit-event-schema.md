---
title: "Audit event schema"
description: The fields of an exported audit event and how Splunk, syslog and OpenTelemetry sinks map them.
"og:url": "https://docs.levelrail.com/audit-event-schema"
---

Every audit sink sends the same record, whether it goes to a webhook, a bucket, Splunk, syslog or an OpenTelemetry collector. Field names are stable: a name is only ever added, and a rename or removal bumps the schema version noted in the release notes. Free text fields have token-shaped strings, bearer values, URL credentials and stored secret values masked before they leave the control plane.

## Fields

| Field | Type | Meaning |
| --- | --- | --- |
| `id` | string | Row id, unique and the key for deduplicating at least once delivery |
| `time` | string | RFC 3339 timestamp with nanoseconds, UTC |
| `actor_type` | string | `session`, `token` or `system` |
| `actor_id` | string | Session user or token id |
| `actor_name` | string | Display name of the actor |
| `ability` | string | Ability the request was authorized under |
| `method` | string | HTTP method |
| `path` | string | Request path without the query string |
| `status` | integer | HTTP status code |
| `remote_addr` | string | Client address |
| `client_kind` | string | Surface: `cli`, `dashboard`, `mcp`, `api` or `system` |
| `agent_name`, `agent_client` | string | Agent label and MCP client, when present |
| `action` | string | Stable event name such as `login_code.redeemed`, when the row is a lifecycle event, or `sensitive_read` for a read of env, secrets, logs, Loki queries or an export (path only, never the content) |
| `on_behalf_of` | string | `user:<id>` an AI turn acted for, when present |
| `chain_seq` | integer | Position in the tamper evidence chain, absent on rows written before the chain existed |
| `chain_prev` | string | Hash of the previous row |
| `chain_hash` | string | Hash of this row |
| `chain_alg` | string | `k` (keyed HMAC) or `h` (plain SHA-256) |

Empty optional fields are omitted. The lifecycle `detail` JSON is not exported.

## Verifying the chain downstream

A receiver can check that `chain_seq` increases by one and that each `chain_prev` equals the `chain_hash` of the row before it. A gap or a mismatch means a row is missing or was changed. Recomputing a keyed hash needs the chain key, which never leaves the control plane, so the full check is `levelrail-cli audit-verify` there.

## Sink mappings

| Sink | Shape |
| --- | --- |
| Webhook, bucket | NDJSON, one record per line |
| Splunk | HEC event, `event` is the record, `time` is epoch seconds |
| Syslog | RFC 5424, structured data element `audit@32473` holds the fields as `key="value"` pairs, the message is `action` or `METHOD path status` |
| OpenTelemetry | Log record, `body` is the message, attributes `audit.id`, `audit.actor.type`, `audit.actor.id`, `audit.actor.name`, `audit.ability`, `http.request.method`, `url.path`, `http.response.status_code`, `client.address`, `audit.surface`, `audit.action`, `audit.on_behalf_of`, `audit.chain.seq`, `audit.chain.prev`, `audit.chain.hash`, `audit.chain.alg` |

See [Data export](/data-export#splunk-syslog-and-opentelemetry-sinks) for setting a sink up.
