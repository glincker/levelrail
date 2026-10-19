---
title: "Log pipelines"
description: Apply transformations to every log line before storage, export or live tail. Redact secrets, drop noise, parse structured fields.
"og:url": "https://docs.levelrail.com/log-pipelines"
---

Transform logs on every line before storage, live tailing or export. Rules can redact secrets, drop health checks, parse fields and join multiline records. Rule changes take effect within about 15 seconds and only affect new lines.

## Set up a pipeline

In the dashboard, open the app, then **Observe, Logs**, scroll to **Pipeline**, and paste your rules. Or:

```bash
levelrail-cli apps log-pipeline set web --file rules.json
levelrail-cli apps log-pipeline preview web --file rules.json --input lines.txt
```

A pipeline has at most 20 rules, applied in order, and each rule must match one of these kinds.

## Rule kinds

**Multiline (one per pipeline)**

Matches the first line of a record with a regex pattern; following lines are joined to it until 2 seconds of quiet or 64 KiB is reached. Use this for exceptions, stack traces or logs with embedded newlines.

```json
{
  "kind": "multiline",
  "pattern": "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}"
}
```

**Redact (zero or more)**

Matches part of a line with a regex and replaces it with `[REDACTED]` (or a `replace` value you set). Applied to the stored and exported copy, not the container's own stdout.

```json
{
  "kind": "redact",
  "pattern": "password=[\\S]+",
  "replace": "[REDACTED]"
}
```

**Drop (zero or more)**

Matches a line and discards it entirely. Nothing is stored or tailed.

```json
{
  "kind": "drop",
  "pattern": "GET /healthz"
}
```

**Parse (zero or more)**

Matches a line with a regex that has named groups. Each named group becomes a structured field (stored and queryable like JSON log fields).

```json
{
  "kind": "parse",
  "pattern": "^(?P<level>[A-Z]+) (?P<msg>.+)"
}
```

## Example rules.json

```json
[
  {
    "kind": "multiline",
    "pattern": "^\\d{4}-\\d{2}-\\d{2}T\\d{2}:\\d{2}:\\d{2}"
  },
  {
    "kind": "drop",
    "pattern": "GET /healthz HTTP"
  },
  {
    "kind": "redact",
    "pattern": "password=[\\S]+",
    "replace": "[REDACTED]"
  },
  {
    "kind": "parse",
    "pattern": "^(?P<level>[A-Z]+) (?P<timestamp>[^ ]+) (?P<msg>.+)"
  }
]
```

## Manage pipelines

| Action | Command |
| --- | --- |
| Set or update | `levelrail-cli apps log-pipeline set <app> --file rules.json` |
| View | `levelrail-cli apps log-pipeline get <app>` |
| Preview (dry run) | `levelrail-cli apps log-pipeline preview <app> --file rules.json --input lines.txt` or `--input -` for stdin |
| Clear | `levelrail-cli apps log-pipeline clear <app>` |

A pipeline is app-specific. Changes take effect within about 15 seconds and apply only to new lines.

## API

- **GET** `/api/v1/apps/{name}/log-pipeline` - fetch the pipeline
- **PUT** `/api/v1/apps/{name}/log-pipeline` - set the pipeline (body: `{"rules":[...]}`), or empty list to clear
- **DELETE** `/api/v1/apps/{name}/log-pipeline` - clear the pipeline
- **POST** `/api/v1/apps/{name}/log-pipeline/preview` - dry run a pipeline against sample lines (body: `{"rules":[...],"lines":[...]}`)

## Patterns

Patterns use Go RE2 syntax, so backreferences and lookahead are not available. Multiline and parse must have a `pattern`. Redact has `pattern` and optional `replace` (default `[REDACTED]`). Drop has `pattern` only.

## Dashboard

The **Pipeline** card on the app's Observe, Logs page shows your current rules, a live preview pane below it, and a full-screen editor. Changes save on click; errors appear inline.

## What redaction covers

Redaction applies to:
- Lines stored in the node-local log store
- Lines returned by the `/api/v1/apps/{name}/logs` search endpoint
- Lines downloaded via `/api/v1/apps/{name}/logs/download`
- Lines exported via log drains (external HTTP endpoints)

It does **not** change:
- The container's own stdout or stderr (visible if you exec into the container)
- Traces or metrics (separate systems)

## Troubleshooting

### Pattern is not matching

Check the pattern with a test case: `levelrail-cli apps log-pipeline preview <app> --input -` and paste a matching line. Verify the pattern compiles as Go RE2.

### Rules apply to past logs?

Rules only affect new lines. Search or download past logs, they keep the original text. To redact old logs, delete the app's log store (drops all history) and redaction starts fresh.

### What if a rule fails?

A malformed rule (invalid regex, bad JSON) is rejected when you save. A rule that matches nothing is silently ignored on every new line.
