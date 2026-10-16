---

> **Draft, not published.** This table was generated from early runs that were distorted by harness and fixture bugs and a Docker network exhaustion bug that has since been fixed. It is not linked from the docs index until a clean regression run replaces it.

title: "Tested framework compatibility"
description: Framework detection, build, and runtime compatibility verified against real test deployments. Table data refreshed periodically.
---

## Overview

This page documents which frameworks and build packs have been tested end-to-end with Levelrail. Tests are run automatically, and results are version-stamped so you can see when each framework was last verified.

The table covers:
- **Framework detection**: Railpack recognized the framework and guessed the right language and port.
- **Build**: The build step completed without error.
- **Runs**: The container started and stayed running.
- **HTTP check**: A simple HTTP GET to `/` returned 200.
- **Health check**: The configured health endpoint (default `/`) passed.
- **Redeploy**: A second deploy worked and rolled out without error.
- **Rollback**: Rollback to a previous deploy worked.

A framework marked "Tested" with a date means all those steps passed on that date at that Levelrail version. "Not yet tested" means the framework is not in the test suite. "Unsupported" means the framework is detected but the platform cannot build it (e.g., Railpack does not support Ruby yet).

## Test data

<!-- DATA_START -->
| Framework | Detection | Build | Runs | HTTP | Health | Redeploy | Rollback | Notes | Tested | Status |
|---|---|---|---|---|---|---|---|---|---|---|
| Django | pass | pass | pass | pass | pass | pass | FAIL | Python 3.x, `/admin` sometimes used for health | 2026-10-15 v0.2.0 | Mostly working |
| Go | pass | pass | pass | pass | pass | pass | FAIL | Detected as "Go / go :8080" | 2026-10-15 v0.2.0 | Mostly working |
| Node.js (Express) | pass | pass | pass | pass | pass | pass | pass | Custom health check supported | 2026-10-15 v0.2.0 | Tested |
| Node.js (Next.js) | pass | FAIL | FAIL | FAIL | FAIL | - | - | Missing devDependencies (TypeScript, @types/*); fixture needs to run `npm install` pre-commit | 2026-10-15 v0.2.0 | Broken fixture |
| Node.js (Vite React) | pass | pass | - | - | - | pass | - | Detected as static frontend but built with railpack (not static); port mismatch | 2026-10-15 v0.2.0 | Unsupported build path |
| Python (FastAPI) | pass | FAIL | FAIL | FAIL | FAIL | - | - | Runs reported success but network unreachable; HTTP check then failed | 2026-10-15 v0.2.0 | Partial support |
| Ruby (Rails) | unsupported | unsupported | FAIL | FAIL | - | - | - | Railpack detected provider "ruby", only node, golang, java, and python are supported yet | 2026-10-15 v0.2.0 | Unsupported |
| Java (Spring Boot) | pass | pass | FAIL | FAIL | FAIL | - | - | Build succeeded but application failed to start or route traffic; network unreachable | 2026-10-15 v0.2.0 | Partial support |
| Static HTML | pass | pass | FAIL | FAIL | FAIL | pass | FAIL | Rollback not supported for static sites yet (no image digest); HTTP check failed | 2026-10-15 v0.2.0 | Partial support |
<!-- DATA_END -->

## How this is tested

Tests are run with the conformance harness in `test/conformance/`. Each test:

1. Builds a fixture app (sample code in `test/fixtures/`).
2. Detects the framework with Railpack.
3. Runs a deploy and watches the rollout.
4. Performs health and HTTP checks.
5. Redeploys the same commit and rolls back.

Tests run in parallel (4 concurrent jobs) to measure queue and build times under load. Failures are collected and sorted by step, so you can see whether a framework's build works but its runtime fails.

Results are stored in `test/conformance/results/` with version, commit, and timestamp. If a result is not in those files, it has not been tested at this Levelrail version.

## What "pass" and "FAIL" mean

- **pass**: The step completed without error and returned an expected result.
- **FAIL**: The step did not complete or returned an unexpected result. Details are in the conformance report.
- **unsupported**: Railpack detected the framework but the platform cannot build it (e.g., Ruby). This is a platform limitation, not a test failure.
- **-**: The step was skipped because a prior step failed. For example, if the build fails, "runs", "HTTP", and "health" are all skipped.

## Known limitations

- **Build missing for some frameworks**: Ruby and PHP are detected by Railpack but cannot be built yet. PHP support is not planned; Ruby is on the roadmap.
- **Network issues in tests**: Some frameworks (Spring, FastAPI, Go) report success but fail health checks, suggesting a network or ingress routing issue being investigated.
- **Rollback**: Rollback is failing across multiple frameworks, likely due to image digest or tag issues not specific to any one framework.
- **TypeScript**: Next.js fixture has missing TypeScript dev dependencies, causing test failure. This is a fixture issue, not a platform issue.

## Running the tests yourself

```bash
cd test/conformance
go run . -run wave1  # Run a named wave
go run . -run '*'    # Run all tests
```

See `test/conformance/README.md` for full options and how to interpret results.

## Reporting a framework issue

If you encounter a framework that Levelrail claims to support but does not work:

1. Try a minimal example from `test/fixtures/` to rule out app-specific issues.
2. Check the deploy logs in the dashboard for the exact error.
3. Open an issue on [GitHub](https://github.com/glincker/levelrail/issues) with your app's `app.yaml` and the deploy log output.

## Future frameworks

Planned support (not yet tested):

- PHP (via Railpack, planned)
- Ruby (via Railpack, planned)
- Compose multi-service apps (detection works; build path validation in progress)
