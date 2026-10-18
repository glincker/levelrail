---
title: "Private networks and allow rules"
description: "Group apps into named private networks, allow specific app to app paths, and fix blocked paths from the dashboard, CLI or API."
---

Every app already runs on its own Docker network, so two apps cannot reach each
other by default. Private networks and allow rules let you choose who can talk to
whom, and show the result as a map.

Open **Network, Private networks and rules** in the dashboard.

## Concepts

- **Private network**: a named Docker network. Apps you add as members can reach each other.
- **Default deny**: a private network flag. Members are isolated from each other until a rule allows a pair.
- **Allow rule**: lets two apps reach each other across network boundaries. Rules work in both directions at the network level.

## How it is enforced

- **Same node**: the control plane creates one Docker network per private network and per rule, and attaches the apps' containers to it through the Docker Engine API. It re-applies every reconcile pass, so redeployed containers are attached again and removed rules are detached.
- **Across nodes**: apps on other nodes are found through mesh DNS. Members of a default-deny network with no rule naming them are left out of mesh DNS, so their names stop resolving. This gates name resolution, it is not a packet filter.
- **Limits**: rules are app level, not per port. Direct IP access across the mesh is not filtered.

## The map

Green lines are allowed, red dashed lines are blocked, and amber lines mean the
policy and what runs on Docker disagree. Blocked paths between apps of the same
project show a **Allow** button that creates the rule. Amber paths show **Re-apply**.

## CLI

```
levelrail-cli private-networks list
levelrail-cli private-networks map
levelrail-cli private-networks create backend --members api,worker --default-deny
levelrail-cli private-networks set backend --members api,worker,cron
levelrail-cli private-networks allow web api --note "frontend calls the API"
levelrail-cli private-networks deny <rule-id>
levelrail-cli private-networks apply
levelrail-cli private-networks delete backend
```

## API

| Method and path | Purpose |
| --- | --- |
| `GET /api/v1/private-networks` | Networks and rules |
| `POST /api/v1/private-networks` | Create a network |
| `PUT /api/v1/private-networks/{id}` | Replace members and default deny |
| `DELETE /api/v1/private-networks/{id}` | Delete a network |
| `POST /api/v1/private-networks/rules` | Add an allow rule |
| `DELETE /api/v1/private-networks/rules/{id}` | Remove a rule |
| `GET /api/v1/private-networks/map` | Apps, paths, verdicts and fixes |
| `POST /api/v1/private-networks/apply` | Re-apply enforcement now |

Changes need root. Reads need the read ability.
