---
description: The shared Go kit, a nested standard-library-only module of small infrastructure packages (SSRF-safe HTTP, cron parsing, health probes, log parsing, firewall sync and more) that Levelrail itself uses, and how to import it.
---

# Shared Go kit

`kit` is a Go module that lives in this repository under `./kit`. It holds the small, self-contained building blocks that the control plane and node agent share, packaged so other Go programs can import them too. Every package uses the Go standard library only, and the module has no third-party requirements.

Import a single package rather than the whole module, for example `github.com/GLINCKER/levelrail/kit/cronexpr`. The full package list and install notes are in [kit/README.md](https://github.com/glincker/levelrail/blob/main/kit/README.md); this page explains what each package is for.

## Install

<CopyCommand command="go get github.com/GLINCKER/levelrail/kit@latest" />

The module requires Go 1.24 or newer.

## Versioning and stability

- The module is **v0.x**. The API may change between minor versions and there is no compatibility promise yet. Pin an exact version if you depend on it.
- The module lives in this repository as a nested module with its own `go.mod`, so it is versioned separately from the platform. Releases are tagged `kit/vX.Y.Z`, which is the tag form Go uses for a module in a subdirectory.
- Changes to `kit/` go through the same pull request flow as the rest of the repository (see `CONTRIBUTING.md`). The module is Apache-2.0 licensed.
- Source for the kit never names the product. A repository check (`scripts/check-brand-strings.sh`) scans `kit/**/*.go` for brand strings, in line with the platform's [rebrandability rule](white-labeling.md).

## Packages

### Networking and safety

| Package | What it does |
| --- | --- |
| `netguard` | Builds HTTP clients and validates URLs for user-configured outbound requests such as webhooks. It refuses loopback, private, link-local and other internal addresses, and checks at dial time so redirects and DNS rebinding are covered. `NewClient` is the strict default; `ValidateURL` and `IsBlocked` are the standalone checks. Setting `APP_NOTIFY_ALLOW_PRIVATE_NETWORKS` to a true value allows private addresses (`AllowPrivate` reads it). |
| `untrusted` | Prepares attacker-influenced text such as logs, environment values and commit messages for an LLM prompt. It strips terminal and invisible characters, redacts obvious secrets, truncates, and wraps the result in a delimited block marked as data (`Sanitize`, `Wrap`, `SanitizeValue`). This lowers prompt-injection risk but is not a guarantee. Size limits come from `APP_UNTRUSTED_MAX_FIELD_BYTES` (default 4096) and `APP_UNTRUSTED_MAX_BLOCK_BYTES` (default 65536). |
| `firewall` | Manages host `ufw` rules as declarative allow and deny records. Every rule it creates carries a comment prefix you choose, and `Sync` only adds or removes rules carrying that prefix. It never touches your own rules, never runs `ufw enable`, and never changes the default policy. `Validate` refuses a deny, or a source-restricted allow, on a port you list as required, so a rule cannot lock the platform out. |

### Scheduling and probing

| Package | What it does |
| --- | --- |
| `cronexpr` | Parses standard 5-field cron expressions and computes the next matching time, in UTC or in a given time zone (`Parse`, `Schedule.Next`, `NextInLocation`). It only evaluates schedules; it does not run anything. |
| `ticker` | `Run` calls a function on a fixed interval until its context is cancelled, and logs tick errors instead of stopping. |
| `probe` | Readiness and liveness checks against a container: an HTTP(S) request to its address, or a command run inside it. `Check` runs one attempt and `WaitReady` polls until the target is ready. Limits (redirect count, exec output size, default timeout and interval) read `APP_PROBE_MAX_REDIRECTS`, `APP_PROBE_EXEC_OUTPUT_BYTES`, `APP_PROBE_DEFAULT_TIMEOUT` and `APP_PROBE_DEFAULT_INTERVAL`. |

### Parsing and detection

| Package | What it does |
| --- | --- |
| `slowquery` | Parses Postgres and MySQL slow-query log lines into structured entries with a timestamp, duration and query text (`ParsePostgres`, `ParseMySQL`). |
| `stackdetect` | Inspects a local project directory and guesses its framework and how to build it (Dockerfile, Compose, Node.js, Go, Java, Python or static site) without executing anything from the project. `Detect` returns a `Stack` with a build type, path and port. |
| `pathfilter` | Doublestar-style include and ignore globs for deciding whether a set of changed paths should trigger a build or deploy in a monorepo (`Filter.Apply`, `Match`). |
| `semver` | `Compare` orders release version strings such as `v1.2.3` or `0.4.0-beta.2` by semantic-versioning precedence. |

### System and release helpers

| Package | What it does |
| --- | --- |
| `diskspace` | Free and total bytes for the filesystem holding a path, on Unix and Windows, plus `HumanBytes` for display. |
| `forecast` | Projects when a disk will fill using a least-squares line fit over usage samples. Real usage is rarely linear, so treat the result as a rough estimate. |
| `dockerhub` | Minimal client for Docker Hub's public, unauthenticated API: repository search and tag listing. |
| `upgrade` | Fetches the latest release for the stable, beta and edge channels from a GitHub repository, compares versions, and runs the read-only preflight checks that gate a self-upgrade. Preflight reads `APP_DOCKER_KNOWN_BAD_FILE`, `APP_UPGRADE_MIN_FREE_BYTES` and `APP_UPGRADE_MAX_BACKUP_AGE`. |

## Examples

Most packages ship runnable `Example` functions next to their code (in each package's `example_test.go`), which `go doc` and pkg.go.dev render. Start there for exact signatures.

<CardGroup :cols="2">
<Card title="White-labeling" href="/white-labeling">

How the platform derives names, prefixes and the update repository from one brand file.

</Card>
<Card title="Architecture" href="/architecture">

Where these packages sit in the control plane and agent.

</Card>
</CardGroup>
