---
title: "Case studies: measured results, not testimonials"
description: "Real Levelrail measurements: a 500-app idle benchmark and multi-node verification against real Docker daemons, plus what is not proven yet."
---

**Measured results, stated honestly**

Levelrail is pre-release and has no customer logos yet, so instead of testimonials this page shows what has actually been measured and verified, and what has not.

[Run your own test](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)

## What has been verified

<CardGroup cols={2}>
  <Card title="Footprint as apps grow">
    In the idle benchmark, going from 0 to 500 suspended apps moved resident memory from about 68 MB to about 91 MB and CPU from 0.03% to 1.6%. It ran on an Apple M4 Max with a development build and no containers running, so expect different numbers on a Linux VPS. The method is reproducible.
  </Card>
  <Card title="Multi-node against real daemons">
    Enrollment, cordon, drain with real container relocation and explicit placement pinning were run with two real agent processes, each with its own Docker daemon.
  </Card>
</CardGroup>

## What is not proven yet

Single node is the best tested path. The WireGuard mesh and cross-host remote transport beyond the join flow are still unverified, there is no Linux release-build idle measurement yet, and public ACME has one live run with renewal and wildcards unverified. The feature status page lists the evidence behind each area.

Fewer real-world deployments also means less community knowledge than projects that have existed for years.

## Be the first case study

If you run Levelrail on a real workload, we would like to hear what broke and what worked. Open a discussion or an issue on GitHub, and if you are happy to be named, we will write it up with your numbers.

## At a glance

- **68 MB**: RSS at 0 apps, dev build
- **91 MB**: RSS at 500 suspended apps
- **1.6%**: CPU at 500 suspended apps
- **500**: apps in the idle benchmark

## Frequently asked questions

<AccordionGroup>
  <Accordion title="Why are there no customer testimonials?">
    Because the project is pre-release and has no public production customers yet. This page only shows measurements and verifications that exist.
  </Accordion>
  <Accordion title="Where do the idle numbers come from?">
    From a documented benchmark script (`scripts/bench-idle.sh`) you can run yourself. The [performance page](/performance) lists the method, the machine and the caveats.
  </Accordion>
  <Accordion title="Are these numbers comparable to other platforms?">
    They were not measured against other platforms here, so this page makes no comparative claim. Measure your own setup with the same method if you want a comparison.
  </Accordion>
</AccordionGroup>

## Add your results

Run it, measure it, and tell us what you find.

[Run your own test](/getting-started) | [View on GitHub](https://github.com/glincker/levelrail)
