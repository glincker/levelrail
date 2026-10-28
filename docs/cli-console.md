---
title: "CLI console (beta)"
tag: "Beta"
description: "The beta CLI console runs the platform CLI from the dashboard over a WebSocket, with step-up confirmation, a reviewed command allowlist, an audit row per line and no shell."
"og:url": "https://docs.levelrail.com/cli-console"
---

The CLI console is a terminal-style panel in the dashboard that runs `levelrail-cli` commands against your control plane. It is not a shell: a typed line is split into words, checked against a reviewed command allowlist and flag deny rules, and started as the CLI binary with no shell in between. Pipes, redirects, chaining, substitutions and environment assignments are refused.

It is a browser feature only. There is no CLI command or MCP tool that opens a console; from a terminal you run `levelrail-cli` directly.

## Turn it on

The console is off by default. Turn it on in **Settings, Beta features**, or from the CLI:

```bash
levelrail-cli settings beta-features set cli-console on
```

The switch applies live, with no restart. `APP_EXPERIMENTAL=cli-console` on the control plane turns it on too, and then Settings cannot turn it off. `APP_CLI_CONSOLE=off` (also `false`, `0`, `no`, `disabled`) is a kill switch that wins over both. While the feature is off, the console's two routes answer `409` with `"code": "experimental_feature_disabled"`, and the dashboard shows a locked **Console** entry in the sidebar and a "turn it on" prompt in the dock. See [Experimental features](/experimental-features).

## Where it shows up

- **Console** in the global navigation (`/terminal`).
- A **Console** tab in the bottom dock, so you can keep it open next to another page.
- A quick actions toolbar of ready-made lines: status, version, apps, deploys, nodes and databases lists, per-app status, logs and deploys, and node details (read only); app restart and redeploy (change); app rollback and system restart (destructive). Change and destructive actions ask you to confirm before the line is sent.
- Command completion from `GET /api/v1/system/cli-console/commands`, and arrow-key history for the open session. Typing `clear` clears the screen.

## Who may use it

Only users with the `root` ability, on both routes. On top of that, opening the console needs:

1. **A dashboard sign-in.** A session link or an API token is refused (`step_up_session_only`).
2. **A same-origin request.** The WebSocket must carry an `Origin` that matches the dashboard host, otherwise `cross_origin`.
3. **A fresh step-up.** You confirm your password (or passkey) for the action `manage` on the pseudo app `cli-console`. The grant is bound to your session and lasts 5 minutes by default (`APP_SECRET_STEP_UP_TTL`, at most 15 minutes). Without it the answer is `step_up_required`.

If the user loses the `root` ability while a session is open, the server ends the session.

## What it can run

Only the command paths in the reviewed allowlist, `internal/cliconsole/allowlist.go` (236 of the CLI's 1488 leaf commands). Every entry has a one-line reason and was reviewed as API-only: it takes no host path, file, directory, URL to fetch or program to launch, and does not print secret values by default. Most entries are reads (`list`, `get`, `show`, `status`, `logs`, `metrics`, `history`, `wait`); the few writes are `apps restart`, `apps rollback`, `apps projects restart`, `compose redeploy` and `models restart`. Everything else, including `deploy`, `dev`, `functions dev`, `databases connect`, `images lint`, `bench`, `plugins` and `storage ... upload`, is refused. A new CLI command is refused until someone adds it to the table.

The session runs with a token minted for you, valid for the session only and revoked when it ends. It never carries `root`, `read:sensitive`, `write:sensitive`, `secrets:reveal` or `signin:approve`: a root caller gets `read`, `write` and `deploy`. Endpoints that need `root` or a sensitive ability answer `403` inside the console. The CLI is pointed at the control plane's own loopback address.

Refused flags, on every command, because they read or write local paths, stdin or local environment, launch programs, or redirect the CLI's own credentials and target: any flag whose name ends in `file`, `files`, `-path`, `-dir`, `-dirs`, `-stdin`, `-from-env`, `-cert`, `-socket`, `-bin` or `-binary`, plus a fixed list that includes `--file`, `-f`, `--out`, `-o`, `--output`, `--input`, `--include`, `--exclude`, `--manifest`, `--signature`, `--config`, `--redirects`, `--bin`, `--image`, `--launch`, `--csv`, `--ratchet`, `--record`, `--fix`, `--static`, `--path`, `--dir`, `--token`, `--api-url`, `--profile`, `--interactive` and `--debug` (the full list is `deniedFlags` in `internal/cliconsole/policy.go`). Any argument containing `file:`, a bare `--`, a bare `-` (stdin), an argument or flag value starting with `@`, and `--flag=-` are refused too.

### Drift tests

- `TestCheckDeniesByDefault` (`internal/cliconsole`) walks the whole CLI command tree and fails if any path outside the allowlist passes the check, or if an allowlist entry has no reason or is no longer a CLI command.
- `TestConsoleAllowlistFlagsReviewed` (`cmd/levelrail-cli`) runs `-h` for every allowlisted command and fails when one accepts a flag that looks like a path, URL or launch flag (a deliberately broader pattern than the deny rules) and the console does not refuse it. A flag that only filters an API response is listed with a reason in `reviewedSafeFlags`. It also fails if fewer than half of the allowlisted commands print their flags, so it cannot pass blind.
- `TestConsoleScopeStripMatchesCLI` (`cmd/levelrail-cli`) checks, for every allowlisted command, that the console removes `--team` and `--project` exactly as the CLI's own dispatch does, so the check vets the command the CLI will run.

Other line rules: at most 4096 bytes and 64 words; no newlines or control characters; quotes and backslashes work as in a POSIX shell but nothing is expanded; `` ` ``, `$(` and `${` are refused; a subcommand must come right after its command, before flags.

A refusal never runs anything. It shows the reason and is audited.

### Host diagnostics

A small read-only `host` namespace (`host df`, `host free`, `host uptime`, `host load`, `host os`, `host service`, `host help`) is answered inside the control plane process without starting any program. See [CLI reference](/cli-reference#web-cli-console-host-diagnostics). `APP_CLI_CONSOLE_HOST=off` turns only that namespace off.

## Limits

| Limit | Default | Env var |
| --- | --- | --- |
| Idle time before the session closes | 10 minutes | `APP_CLI_CONSOLE_IDLE_TIMEOUT` |
| Time one command may run | 2 minutes | `APP_CLI_CONSOLE_COMMAND_TIMEOUT` |
| Total session length | 1 hour | `APP_CLI_CONSOLE_MAX_SESSION` |
| Output of one command | 1 MiB | `APP_CLI_CONSOLE_MAX_OUTPUT_BYTES` |
| One output line | 4096 bytes | `APP_CLI_CONSOLE_MAX_LINE_BYTES` |
| Sessions opened per minute, per user and per IP | 6 | `APP_CLI_CONSOLE_OPENS_PER_MINUTE` |
| Unprivileged account when the control plane is root | `nobody` | `APP_CLI_CONSOLE_USER` |
| CLI binary path | `levelrail-cli` next to the control plane | `APP_CLI_BINARY` |

One command runs at a time per session; a second `run` while one is active is refused until you cancel it. A command past the timeout or the output cap is killed together with everything it started, and the stream says why. A value that does not parse keeps the default.

## Security notes

- **No shell.** The CLI binary is started directly with the checked words. Its location is `APP_CLI_BINARY` (an absolute path) or the file next to the control plane, never `PATH`, and it must be an executable regular file that is not group or world writable.
- **Clean environment.** The child inherits nothing from the control plane. It gets only the loopback API address, the session token, a throwaway home directory that is also its working directory (mode 0700), and the enabled beta feature list. Stdin is empty, no file descriptors beyond stdout and stderr are passed, and the child is in its own session and process group. The home directory is removed after each command.
- **What is guaranteed, per deployment mode.**
  - *Control plane runs as root:* the child is dropped to the unprivileged account above with no supplementary groups, so it cannot read the data directory or the Docker socket by file permissions. The console refuses to run if that account is missing or is uid 0.
  - *Control plane runs as a non-root user (the official image runs `USER nonroot`):* a non-root process cannot switch users, so the child runs as the same uid and could read whatever the control plane can, data directory included. Here the guarantee rests on the allowlist and flag rules: only API-only commands run, none takes a host path, and the drift tests above keep it that way. It is not an operating system boundary.
- **Output is sanitized.** Everything streamed to the browser passes through one filter, line by line: terminal escape sequences (CSI, OSC, other ESC sequences) and control bytes are stripped, the session token, `KEY=VALUE` and JSON pairs with a secret-looking key, URL credentials, bearer tokens and private key blocks are masked, and each line is cut at 4096 bytes (`APP_CLI_CONSOLE_MAX_LINE_BYTES`). The same filter runs on the audit row.
- **Least privilege.** Each command is in its own process group, killed as a group on cancel or timeout.
- **Audit.** Every typed line writes an audit row, actor, address and client kind included: `cli_console.run` with the exit code, duration and a note (timed out, cancelled, output truncated), or `cli_console.rejected` with the reason. The line is stored with secret-looking values masked: flag values whose name looks secret, `KEY=VALUE` pairs with a secret-looking key, URL credentials, and all values after the first positional word of secret-like commands (`secret`, `env`, `vault`, `password`, `credential`, `users`). Audit lines are cut at 512 bytes. The masking is pattern based: a secret in an unusual shape can get through.
- **Not on the demo instance.** The console routes are blocked in demo mode.

## Troubleshooting

### The console shows a locked entry or "turn it on"

The beta is off. Turn it on in Settings, Beta features, or `levelrail-cli settings beta-features set cli-console on`. If it is on and still refuses, check that `APP_CLI_CONSOLE` is not set to `off` on the control plane.

### "Confirm your password to open the CLI console"

The step-up grant is missing or has expired (5 minutes by default). Confirm again in the dialog. If you signed in with a link or token, sign in to the dashboard directly.

### `cli_console_unavailable`

Either the CLI binary was not found or is unsafe (set `APP_CLI_BINARY` to an absolute path, or install `levelrail-cli` next to the control plane, not writable by group or others), or the control plane's own listen address is not known. The server log has the exact reason.

### "running as root needs an unprivileged user"

The control plane runs as root and the account in `APP_CLI_CONSOLE_USER` (default `nobody`) does not exist. Create it or point the variable at another unprivileged account.

### Too many sessions

More than 6 opens in a minute answers with a rate-limit response and a retry time. Wait, or raise `APP_CLI_CONSOLE_OPENS_PER_MINUTE`.

### A command is refused

Read the reason. Only allowlisted commands run; commands that need local files (`up`, `apply`, `deploy`, `env pull`) belong on your own machine with a local CLI. Put the subcommand right after the command and before flags.

## What is missing before it leaves beta

- **No end-to-end test.** There are unit tests for the line parser, policy, redaction and runner (`internal/cliconsole`), and API tests for the gate order, step-up, rate limit and a session that runs and revokes its token (`internal/api/cli_console*_test.go`). Nothing in `test/e2e` drives a real browser session, and the dashboard component has tests only for the quick actions table.
- **No recorded run on a real host.** The root-to-unprivileged user drop, process-group kill and the loopback address behind a proxy have not been exercised on a live server. Non-unix builds use a stub: not verified.
- **Output masking is pattern based.** A secret printed in a shape the filter does not know is shown.
- **Same uid without root.** On a non-root control plane the child is not isolated by the operating system (see above). Not verified on a live host: whether any allowlisted command reads a file through a code path its flags do not show.
- **Allowlist review is by command and flag name.** Each allowlisted command was classified from its name and its `-h` flags, not by reading every code path. Root-only endpoints behind allowlisted reads answer `403` because the session token has no `root`; which ones do is not verified.
- **Parity.** The console is dashboard only. There is no API route to run a line without a browser session, which is intended.

See [feature status](/feature-status) for how beta is defined.
