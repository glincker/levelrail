---
title: "Contribute to Levelrail: open source deployment platform"
description: "Help build Levelrail: try it and report what breaks, pick a good first issue, improve docs, add templates or test multi-node. Apache 2.0, active development."
---

**Help build a deployment platform you would want to use**

Levelrail is open source and moving fast. The most valuable contributions right now are real usage, bug reports and focused pull requests.

[Browse good first issues](https://github.com/glincker/levelrail/labels/good%20first%20issue) | [View on GitHub](https://github.com/glincker/levelrail)

## Ways to help

<CardGroup cols={3}>
  <Card title="Try it and tell us what breaks">
    Install on a spare server, deploy something real and open an issue with what you saw. Reports from real workloads are the most useful thing right now.
  </Card>
  <Card title="Pick a good first issue">
    Issues labeled good first issue and help wanted are scoped for newcomers. Read the contributing guide first for branch and commit conventions.
  </Card>
  <Card title="Improve the docs">
    Every docs page has an edit link. Fixes to unclear steps, missing flags and broken examples land fast.
  </Card>
  <Card title="Test multi-node">
    Enrollment, cordon and drain are verified against real Docker daemons. Cross-host transport and the WireGuard mesh need more real-world testing.
  </Card>
  <Card title="Talk to us">
    Ask a quick question in the Levelrail forum on the GLINR Discord, or start a GitHub discussion.
  </Card>
  <Card title="Star the repo">
    A star helps other operators find the project. It costs nothing and it does help.
  </Card>
</CardGroup>

## At a glance

- **Apache 2.0**: license
- **Go + React**: stack
- **1 edition**: no paid tier

## Your first pull request

<Steps>
  <Step title="Open an issue first for anything bigger than a small fix">
    The reconciler, agent transport and database schema have deliberate design constraints worth discussing before you write code.
  </Step>
  <Step title="Branch and commit with the conventions">
    Branch names like fix/short-description and conventional commits. Hooks check formatting, lint, tests and dashes.
  </Step>
  <Step title="Keep each pull request to one logical change">
    Smaller, focused pull requests get reviewed faster.
  </Step>
</Steps>

## Adding an integration

Per-vendor pages under `docs/integrations/` are generated. To add a backup destination, registry, provider, sign-in method or notification channel, add one entry to `docs/integrations/catalog.json` with facts taken from the code, then run:

```bash
go run ./scripts/gen-integration-docs
go run ./scripts/gen-integration-docs -check
```

The generator writes the pages, the Integrations tab in `docs/docs.json` and the Integrations section of `docs/public/llms.txt`. Do not edit those outputs by hand.

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Do I need to sign a CLA?">
    The contributing guide is the source of truth for contribution terms. Read it before opening your first pull request.
  </Accordion>
  <Accordion title="What languages is it written in?">
    Go for the control plane and agent, and TypeScript with React for the dashboard.
  </Accordion>
  <Accordion title="Where do I report a security issue?">
    Follow the security policy in the repository rather than opening a public issue.
  </Accordion>
  <Accordion title="Is there a roadmap?">
    Yes, the roadmap page lists phases and current status.
  </Accordion>
</AccordionGroup>

## Come build it with us

Every issue, fix and star moves it forward.

[Browse good first issues](https://github.com/glincker/levelrail/labels/good%20first%20issue) | [View on GitHub](https://github.com/glincker/levelrail)
