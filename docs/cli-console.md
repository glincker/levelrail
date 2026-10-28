---
title: "CLI console (beta)"
tag: "Beta"
description: "The beta CLI console runs the platform CLI from the dashboard over a WebSocket, with step-up confirmation, a command allowlist, an audit row per line and no shell."
"og:url": "https://docs.levelrail.com/cli-console"
---

The CLI console is a terminal-style panel in the dashboard that runs `levelrail-cli` commands against your control plane. It is not a shell: a typed line is split into words, checked against the CLI command tree and a denylist, and started as the CLI binary with no shell in between. Pipes, redirects, chaining, substitutions and environment assignments are refused.

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

Everything in the CLI command tree except the denied set below. That includes commands that change things (`apps restart`, `deploy`, `rollback`, `system restart`): the console acts with your own full permissions, so treat an open console like a root session.

The session runs with a token minted for you, valid for the session only and revoked when it ends. It carries `root` when you have it, which the route requires. The CLI is pointed at the control plane's own loopback address.

Refused as a command or subcommand because they need your machine, local files or a TTY: `auth` (except `auth whoami`), `login`, `up`, `profile`, `completion`, `init`, `tui`, `proxy`, `migrate`, `import`, `apply`, `diff`, `export`, `update`, `tokens create`, `pipelines validate`, `pipelines save`, `apps exec`, `env pull`, `env push`, `logs fetch`, `backups download`, `app-volume-backups download`, `assets upload`, `control-plane-backups download`, `control-plane-backups keys`, `control-plane-backups escrow`, `secrets generate-master-key`, `secrets rotate-master-key`.

Refused flags, because they read or write local paths, stdin or local environment, or redirect the CLI's own credentials and target: `--file`, `-f`, `--out`, `-o`, `--out-dir`, `--output-file`, `--env-file`, `--var-file`, `--key-file`, `--new-key-file`, `--cert-file`, `--body-file`, `--ssh-key`, `--extract`, `--dir`, `--dynamic-dir`, `--upload`, `--mcp-binary`, `--password-stdin`, `--token-stdin`, `--hf-token-from-env`, `--tok-env`, `--interactive`, `-i`, `--token`, `--api-url`, `--profile`, `--profile-name`, `--target-api-url`, `--target-token`, `--target-profile`, `--debug`. Any argument containing `file:` and a bare `--` are refused too.

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
| Sessions opened per minute, per user and per IP | 6 | `APP_CLI_CONSOLE_OPENS_PER_MINUTE` |
| Unprivileged account when the control plane is root | `nobody` | `APP_CLI_CONSOLE_USER` |
| CLI binary path | `levelrail-cli` next to the control plane | `APP_CLI_BINARY` |

One command runs at a time per session; a second `run` while one is active is refused until you cancel it. A command past the timeout or the output cap is killed together with everything it started, and the stream says why. A value that does not parse keeps the default.

## Security notes

- **No shell.** The CLI binary is started directly with the checked words. Its location is `APP_CLI_BINARY` (an absolute path) or the file next to the control plane, never `PATH`, and it must be an executable regular file that is not group or world writable.
- **Clean environment.** The child inherits nothing from the control plane. It gets only the loopback API address, the session token, a throwaway home directory, and the enabled beta feature list. The home directory is removed after each command.
- **Least privilege.** When the control plane runs as root, the child is dropped to the unprivileged account above and the console refuses to run if that account is missing or is uid 0. Each command is in its own process group.
- **Audit.** Every typed line writes an audit row, actor, address and client kind included: `cli_console.run` with the exit code, duration and a note (timed out, cancelled, output truncated), or `cli_console.rejected` with the reason. The line is stored with secret-looking values masked: flag values whose name looks secret, `KEY=VALUE` pairs with a secret-looking key, URL credentials, and all values after the first positional word of secret-like commands (`secret`, `env`, `vault`, `password`, `credential`, `users`). Audit lines are cut at 512 bytes.
- **Output is not scanned.** The mask applies to the audit row only. What a command prints is streamed to you as is, so a command that prints a value prints it. Whether a revealing command needs its own step-up inside the CLI session is not verified here.
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

Read the reason. Commands that need local files (`up`, `apply`, `env pull`) belong on your own machine with a local CLI. Put the subcommand right after the command and before flags.

## What is missing before it leaves beta

- **No end-to-end test.** There are unit tests for the line parser, policy, redaction and runner (`internal/cliconsole`), and API tests for the gate order, step-up, rate limit and a session that runs and revokes its token (`internal/api/cli_console*_test.go`). Nothing in `test/e2e` drives a real browser session, and the dashboard component has tests only for the quick actions table.
- **No recorded run on a real host.** The root-to-unprivileged user drop, process-group kill and the loopback address behind a proxy have not been exercised on a live server. Non-unix builds use a stub: not verified.
- **No output scanning.** Secrets in command output are not masked (see above).
- **Allowlist drift.** The denylist is hand-kept next to the CLI tree. A new CLI command that reads local files is allowed until someone adds it, so it needs a test that fails when the tree grows an unreviewed path-reading command.
- **Parity.** The console is dashboard only. There is no API route to run a line without a browser session, which is intended. Not verified: whether the denied set matches every current CLI command.

See [feature status](/feature-status) for how beta is defined.
