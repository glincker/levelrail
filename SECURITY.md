# Security Policy

## Supported Versions

Levelrail is pre-1.0 and has not cut a stable release yet. There is no
supported-versions table to publish because there is nothing to compare
against: security fixes land on `main` as they're found.

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
