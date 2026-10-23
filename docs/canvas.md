---
title: "Canvas"
tag: "Beta"
description: Beta visual editor for a project: add apps and databases, connect them, review every change as a plan, then apply it. What it can and cannot do.
"og:url": "https://docs.levelrail.com/canvas"
---

Canvas is a beta editor on top of the [project topology graph](/service-topology-graph). You add apps and databases, connect them, and see every change as a reviewable plan before anything is saved. Nothing reaches the server until you press **Apply**.

<Warning>
**Beta**

Canvas is off by default. Turn it on in Settings, Beta features (the `canvas` switch). It applies at once, with no restart. Review and Apply also need [Platform as code](/platform-as-code) (`iac`) switched on, because the plan and the apply are its API. If `iac` is off, Canvas says so and links to the switch.
</Warning>

## Open it

With the beta on, a project's page shows a **Canvas** button next to **Topology**. It opens `/projects/<id>/canvas`. With the beta off the button is not drawn, the route redirects to Beta features, and no extra data is requested on any other page.

## What you can do

| Edit | How it is applied |
| --- | --- |
| Add an app | A new app that runs a prebuilt image (name, image, port). Planned and applied as an `App` document in this project. |
| Add a database | Postgres, Redis or MySQL, with an optional version. Planned and applied as a `Database` document in this project. |
| Connect an app to a database | Applied through the app connections API after the plan is applied. The environment variable name is optional; blank lets the server pick one. |
| Remove a connection | Removes the connection that the connections API holds for that app and database. |

New nodes are drawn with a dashed border and the label "Draft, not applied yet". Pan, zoom, drag and the keyboard controls of the topology page work the same here. Boxes outside the visible area are not drawn, so a large project stays responsive.

## Review and apply

**Review changes** builds a plan from the draft with the same plan endpoint as **Settings, Infrastructure as code** (`POST /api/v1/apply/plan`) and shows it with the same diff view: created, changed and removed items, with field-level changes.

- If the server rejects the draft, the file, line and reason are shown in the dialog, with the next step. Go back, fix or remove the named item, and review again.
- **Apply** is disabled while the plan has error items or while there is nothing to change.
- Apply sends the reviewed plan hash (`POST /api/v1/apply`), so it only proceeds if live state still matches what you reviewed. If it changed, review again.
- Connection changes are not part of the server's plan, so they are listed separately and marked as not previewed. They run after the plan applies, in order, and stop at the first failure. Anything after a failed step is reported as skipped.

If an apply partly succeeds, the dialog says which items applied and which did not. The draft stays in the page so you can retry.

## What it cannot do yet

Canvas only edits what the existing API can express, so these are left out of v1:

- **Existing apps and templates as new nodes.** Apps and databases already in the project are shown, and can be connected or disconnected. Pulling an existing app in from another project, or deploying a catalog template, is not part of the draft.
- **Volumes.** Linking a shared volume has no plan or apply path (platform as code rejects volumes), so it is not editable here.
- **Domains, load balancers, dependencies and egress rules.** They show on the graph. Change them from the app.
- **Editing or deleting an existing app or database.** The draft only adds.
- **Bindings declared in `app.yaml`.** A database reached through an `env: { from: ... }` binding or an app.yaml attachment is not a connection Canvas owns. Removing one is reported and left alone.
- **Git-built apps.** Platform as code manages image apps only.

Draft state lives in the page. Reloading the page discards the draft.

## Evidence

Beta. Unit tests cover the draft-to-plan mapping, the connection step runner and the toggle gating, and component tests cover the review dialog states. There is no end-to-end test and it has not been run against a real instance yet. See [feature status](/feature-status).

## See also

- [Project topology graph](/service-topology-graph) - the read-only view Canvas builds on
- [Platform as code](/platform-as-code) - the plan and apply path, and its limits
- [Connecting apps to databases](/connecting-apps-to-databases) - the connections Canvas creates and removes
- [Experimental features](/experimental-features) - how the beta switches work
