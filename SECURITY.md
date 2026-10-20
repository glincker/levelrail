# Security Policy

## Supported Versions

Security fixes ship in the latest release, so run the newest version from
the [releases page](https://github.com/glincker/levelrail/releases).
Older releases are not patched separately.

## Reporting a Vulnerability

Please do not open a public GitHub issue for a suspected security
vulnerability.

Instead, use GitHub's private vulnerability reporting: open a [Security
Advisory](../../security/advisories/new) on this repository. That gives
maintainers a private channel to discuss and fix the issue with you
before any public disclosure.

Include as much detail as you reasonably can:

- The component affected (control plane, agent, ingress, build,
  secrets, etc.) and, if known, the file or code path.
- Steps to reproduce, or a proof of concept if you have one.
- The impact you believe the issue has.

Fixed issues are listed with stable IDs on the
[security advisories page](docs/security-advisories.md).

## What to expect

There is no dedicated security team yet and no formal SLA. Reports are
handled best effort by the maintainers. You should expect an initial
acknowledgment within a few days; how quickly a fix ships after that
depends on severity and complexity. If you haven't heard back in a
reasonable amount of time, it's fine to follow up on the same advisory
thread.

Given the platform's scope, particularly relevant areas include secret
handling (`internal/secrets`), agent enrollment and certificate
validation, container escape surface in the Docker integration, and
anything touching the install path once one exists. Reports in these
areas will get prompt attention.
