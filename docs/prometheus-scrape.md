---
title: "Prometheus scrape endpoint"
description: "Scrape app, database, node and deploy metrics from GET /metrics with a read token."
---

`GET /metrics` serves app, database, node and deploy metrics in the Prometheus text format. It is never public: every request needs a read token, and a token limited to some apps only sees those apps.

## Set up

```sh
levelrail-cli tokens scrape
```

This mints a read-only token named `prometheus-scrape` and prints a scrape config to paste into `prometheus.yml`. The token is shown once. Run `levelrail-cli tokens scrape --rotate` to replace it; the old token is revoked after the new one exists.

## Turn it off

The `observability.metrics_scrape_enabled` runtime setting (default on) switches the endpoint without touching tokens. When off, `/metrics` answers 404. It applies live, from the dashboard, API or CLI.

## What is exported

- Per app, database and node: CPU, memory, network and disk metrics, labelled `app`, `database` or `node`.
- Platform gauges: app count, deploy queue depth, running deploys, last and mean deploy duration, deploys finished by `status`, online agents, reconcile lag.
- Export destination health, labelled `destination`.

Labels are limited to those names, so the series count tracks what you run and never grows with request paths or user input.
