# ADR 069: Outbound error forwarding to Sentry-compatible endpoints

Status: Proposed

Date: 2026-11-01

## Context

Levelrail already receives errors: apps point a Sentry-protocol SDK at the
control plane (`internal/erroringest`, `internal/api/errors_ingest.go`,
`docs/error-tracking.md`). Some operators also run, or already use, a
Sentry-compatible service (Sentry or GlitchTip) and want an off-box copy of
errors there, including errors from the control plane itself, which today only
reach its own logs.

What exists today, checked in code:

- `erroringest.ParseEnvelope` and `ParseStore` decode envelope and store
  payloads into `telemetry.ErrorEvent`; `scrub` replaces values of sensitive
  keys (authorization, cookie, password, token and similar) with `[Filtered]`;
  `MaxRawBytes` bounds one stored event; `NormalizeMessage` and `topFrame`
  group events.
- Ingest is rate limited per app (`errorTrackingState.allow`) with limits read
  through `envPositiveInt`.
- The control plane logs through slog wrapped by `secretmask.Handler`
  (`cmd/levelrail/main.go`), so registered secrets are masked in every record.
- `kit/netguard` and `internal/dataexport/guard.go` provide guarded outbound
  clients and endpoint validation (private and metadata addresses refused
  unless opted in).
- Nothing sends errors out.

## Decision

Build an opt-in forwarder with two independent sources and one sender. Both
sources are off by default.

1. **Source A: control plane errors.** A slog handler stage, placed after
   `secretmask.Handler`, turns records at error level (configurable floor
   `APP_ERROR_FORWARD_MIN_LEVEL`) into events. Panics recovered by the HTTP and
   reconciler recover paths become events with a stack. Attributes become
   tags only when their keys are on a short allow list (component, reconciler,
   request id); everything else is dropped, not scrubbed, so new log fields
   never leak by default.
2. **Source B: app error events (optional, per app).** When enabled for an
   app, events that ingest has already accepted, scrubbed and rate limited are
   copied to the sender. Forwarding never runs before the ingest rate limit,
   so an app cannot use the forwarder to exceed its own budget.
3. **Sender.** A small envelope writer and HTTP client in the repo, not a
   vendor SDK. Adding an SDK to `go.mod` is an owner decision and is not
   assumed. The sender speaks the store and envelope endpoints that Sentry and
   GlitchTip both accept, authenticated by the DSN key.
4. **Scrubbing before send.** Every outgoing event passes, in order: the
   existing `scrub`; removal of request bodies, cookies, query strings, user
   objects, IP addresses and `server_name` (replaced by a stable instance id);
   then `secretmask.Mask` over the serialized body as the last step. Fields not
   on the outgoing schema allow list are dropped.
5. **Egress.** The sender uses the netguard client with the DSN host validated
   by the existing endpoint validator. A self-hosted GlitchTip on a private
   network needs `APP_ERROR_FORWARD_ALLOW_PRIVATE`, a dedicated opt-in so
   enabling forwarding does not widen other egress. https only unless
   `APP_ERROR_FORWARD_ALLOW_INSECURE_HTTP` is on. No redirects to another host.
6. **Volume caps.** A bounded queue (`APP_ERROR_FORWARD_QUEUE_SIZE`), a global
   send rate (`APP_ERROR_FORWARD_MAX_PER_MINUTE`), a per-group dedupe window
   (`APP_ERROR_FORWARD_DEDUPE_WINDOW`, keyed on the existing grouping), a
   request timeout (`APP_ERROR_FORWARD_TIMEOUT`) and a body cap reusing
   `MaxRawBytes`. Over a cap, events are dropped and counted; the drop count
   appears in status and as a metric. A 429 from the destination pauses
   sending for the time it asks, bounded by `APP_ERROR_FORWARD_MAX_PAUSE`.
7. **No loops.** The forwarder's own errors are logged at warn with a marker
   attribute that Source A ignores, and never enqueued.
8. **Secrets.** The DSN is stored through `internal/secrets`, added to the
   masking registry, and shown as host plus project id only. Audit records the
   change, never the value.
9. **Surfaces.** Settings API, CLI, MCP and a dashboard card under error
   tracking: destination, which sources are on, last send, drop count, last
   error class.

## Alternatives rejected

- **Docs only (apps point their SDK straight at the external service).** Still
  documented as the simplest path for app errors, but it gives no control
  plane errors and bypasses Levelrail's scrubbing.
- **Vendor SDK in the control plane.** `go.mod` growth, global hooks and
  default integrations that capture more than the allow list. Owner decision
  if ever wanted.
- **Forward raw ingest payloads unchanged.** Skips the outgoing allow list;
  rejected for PII.
- **OTLP logs export instead.** Different consumer; the OTLP path stays
  separate.

## Security

- Opt-in at two levels (forwarding on, then each source).
- Outgoing schema is an allow list, then the existing scrub, then the masking
  registry, so a new field or a secret in a message is not sent by default.
- One dedicated private-network opt-in, no reuse of broader switches.
- The DSN is a credential and handled like other stored secrets.

## Failure modes

- Destination down: queue fills, then drops are counted; nothing blocks
  request handling or reconcilers.
- Destination slow: request timeout and the queue bound keep memory flat.
- Error storm: dedupe and rate cap bound outgoing volume; the drop count shows
  it.
- Wrong DSN: 401 or 403 marks the destination failed; sending pauses until the
  setting changes, with a status message.

## What cannot be verified locally

- Real acceptance by hosted Sentry and by a GlitchTip instance (endpoint and
  header details per version).
- How each destination groups forwarded events next to native SDK events.
- Behaviour under a real 429 from a hosted plan limit.

## Testing plan

- Unit: allow list and scrub order with fixtures that carry cookies, tokens,
  IPs and a registered secret in the message; none survive serialization.
- Caps with an injected clock: queue bound, rate cap, dedupe window, 429
  pause, counted drops.
- Loop guard: a sender failure never produces a forwarded event.
- Sender against an `httptest` server that checks the auth header and body
  shape.
- Real run once against a self-hosted GlitchTip and once against a hosted
  Sentry project, recorded in `docs/error-tracking.md` as verified dates.

## Slices

| # | Slice | Files it owns | Effort | Model | Real host | Secret-sensitive |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | Sender: envelope writer, guarded client, caps, dedupe, pause, drop counter | new `internal/errorforward/` | 1 day | Sonnet | No | Yes (DSN in client) |
| 2 | Outgoing allow list and scrub pipeline with fixtures | `internal/errorforward/scrub.go`, `internal/erroringest/scrub.go` (export only) | 0.5 day | Opus | No | Yes |
| 3 | Source A: slog stage and recover hooks | `cmd/levelrail/main.go`, new `internal/errorforward/sloghandler.go` | 0.5 day | Sonnet | No | No |
| 4 | Source B: per-app opt-in after ingest | `internal/api/errors_ingest.go` | 0.5 day | Sonnet | No | No |
| 5 | DSN storage, settings API, CLI, MCP | new `internal/api/error_forward.go`, `cmd/levelrail-cli` error tracking file, `internal/mcptools` | 1 day | Sonnet, Opus review | No | Yes |
| 6 | Dashboard card and locale keys | `web/src` error tracking settings, `web/src/locales/en/*.json` | 0.5 day | Sonnet | No | No |
| 7 | Docs, env rows, real runs | `docs/error-tracking.md`, `docs/environment-variables.md` | 0.5 day plus runs | Haiku draft, Sonnet runs | Yes | No |

Slices 1 and 2 first (2 can run with 1 since files differ), then 3, 4 and 5 in
parallel, then 6 and 7.

New env: `APP_ERROR_FORWARD_MIN_LEVEL`, `APP_ERROR_FORWARD_QUEUE_SIZE`,
`APP_ERROR_FORWARD_MAX_PER_MINUTE`, `APP_ERROR_FORWARD_DEDUPE_WINDOW`,
`APP_ERROR_FORWARD_TIMEOUT`, `APP_ERROR_FORWARD_MAX_PAUSE`,
`APP_ERROR_FORWARD_ALLOW_PRIVATE`, `APP_ERROR_FORWARD_ALLOW_INSECURE_HTTP`.

## Consequences

- Operators get control plane errors in a tool they already watch.
- One more outbound path and one more stored credential, both on shared guard
  and secret code.
- An outgoing schema to maintain as ingest grows.
- No `go.mod` change.

## Open questions

- Priority: the built-in ingest already covers app errors. Should this wait
  until a user asks for control plane errors off-box? Proposed: build after
  ADR 066 and ADR 067, Source A first.
- Should Source B forward only new groups (first seen) instead of every event,
  to stay inside small hosted quotas by default?
