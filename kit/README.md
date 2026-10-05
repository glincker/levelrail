# kit

Small, dependency-free Go packages for infrastructure tooling: SSRF-safe outbound HTTP, cron schedules, health probes, log parsing, and similar building blocks. Extracted from, and used in production by, the Levelrail platform.

## Install

```
go get github.com/GLINCKER/levelrail/kit@latest
```

Requires Go 1.24 or newer. Import a single package, for example `github.com/GLINCKER/levelrail/kit/cronexpr`.

## Packages

| Package | What it does |
| --- | --- |
| `netguard` | HTTP client and URL validation that refuse loopback, private, and other internal addresses (SSRF protection). |
| `untrusted` | Cleans, redacts, truncates, and wraps untrusted text before it goes into an LLM prompt. |
| `cronexpr` | Parses 5-field cron expressions and computes the next run time, in UTC or a given time zone. |
| `pathfilter` | Doublestar-style include and ignore globs for deciding whether changed paths should trigger a build. |
| `probe` | Exec and HTTP health probe configuration, validation, and a runner with readiness polling. |
| `slowquery` | Parsers for Postgres and MySQL slow-query log lines. |
| `forecast` | Projects when a disk will fill using a least-squares fit over usage samples. |
| `diskspace` | Free and total bytes for a path on Unix and Windows, plus byte formatting. |
| `ticker` | Runs a function on an interval until its context is cancelled, logging errors. |
| `dockerhub` | Minimal client for Docker Hub repository search and tag listing. |
| `stackdetect` | Detects a project's framework and build method from its files. |
| `firewall` | Reconciles prefix-tagged ufw rules against a desired set, with lockout-safety checks. |
| `semver` | Compares release version strings by semantic-versioning precedence. |

## Stability

The module is v0.x. The API may change between minor versions, and there is no compatibility promise yet.

## Dependencies

Every package uses the Go standard library only. The module has no third-party requirements, and that is kept as a rule.

## Contributing

See [CONTRIBUTING.md](../CONTRIBUTING.md) in the repository root. Changes to `kit/` go through the same pull request flow as the rest of the repository.

## License

Apache-2.0. See [LICENSE](LICENSE).
