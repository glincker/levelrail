---
title: "IaC quickstart"
tag: "Experimental"
description: Describe an app and a database as YAML, preview the change, apply it, and export what exists. Five minutes.
---

<Warning>
Set `APP_EXPERIMENTAL=iac` on the control plane and in your shell until v1.1.0. See [Experimental features](/experimental-features).
</Warning>

## 1. Write one file

Save this as `infra/app.yaml` (also at `examples/iac/single-app/app.yaml`):

```yaml
version: 1
kind: Project
metadata: { name: demo }
---
version: 1
kind: App
metadata: { name: web }
spec:
  project: demo
  service:
    build: { type: image, image: "nginx:1.27" }
    port: 80
    replicas: 1
```

## 2. Preview, then apply

```bash
export APP_EXPERIMENTAL=iac
levelrail-cli diff -f infra/
levelrail-cli apply -f infra/ --yes
```

`diff` changes nothing and exits 2 when the files differ from the live state.

## 3. Add a database

Add this document to the same file. The app reads the connection by reference, so no password lands in git. A similar full file is `examples/iac/app-database/app.yaml`.

```yaml
version: 1
kind: Database
metadata: { name: main-db }
spec:
  engine: postgres
  version: "16"
  project: demo
```

Then point the app at it with `env: { DATABASE_URL: { from: { database: main-db, field: url } } }` and run `levelrail-cli apply -f infra/ --yes` again.

## 4. Export what exists

```bash
levelrail-cli export --project demo -o exported/
```

Export to a new directory, not the one you apply from.

## 5. Move it into a stack

A stack gives you a reviewed plan, numbered revisions and rollback. A stack refuses resources that a plain `apply` already created, because no stack owns them. Start the stack with fresh names. The first stack apply creates the stack, so there is no separate create step.

```bash
mkdir stack-infra
sed 's/demo/demo-stack/g; s/web/web-stack/g; s/main-db/stack-db/g' infra/app.yaml > stack-infra/app.yaml
levelrail-cli stack plan demo-prod -f stack-infra/
levelrail-cli stack apply demo-prod -f stack-infra/ --yes
```

To bring the resources from step 2 into a stack instead, delete them first. The REST stack apply also accepts `adopt: true`, which claims existing resources, but the CLI has no flag for it.

Next: [IaC reference](/iac/reference), [Platform as code](/platform-as-code), [Stacks and GitOps](/stacks).
