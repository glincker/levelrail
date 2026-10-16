---
title: "Migrating a service from Cloud Run"
description: "A runbook for moving a Google Cloud Run service to Levelrail: concept mapping, deploying the same image, secrets, Cloud SQL, domains, jobs and what you give up."
---

This is a runbook for moving one Cloud Run service onto a Levelrail instance you run yourself. It maps only to features that exist in this repository today. Where something is unverified or has no equivalent, it says so.

<Warning>
Some areas are still beta (see [feature status](/feature-status)). You operate the server it runs on: OS updates, disk, backups and the Docker daemon are yours. Run the new deployment in parallel with Cloud Run, and keep Cloud Run as the rollback path until the shadow run in section 7 is clean.
</Warning>

There is no importer for Cloud Run. Everything below is manual. A Cloud Run service is already a container image listening on a port, which is the unit Levelrail deploys, so most services move without a rebuild. Whether the move saves money is a separate question, covered in [Cloud Run alternative](/cloud-run-alternative).

## 1. Inventory what Cloud Run holds

Collect this before touching Levelrail. Much of it lives in the Google Cloud console or in the service's revision template, not in your repo.

| Item | How to get it | Why you need it |
| --- | --- | --- |
| Full service definition | `gcloud run services describe SERVICE --region REGION --format export` | One YAML with the image, port, env, resources, scaling and probes. Keep it next to your notes. |
| Image reference | The `image` field in that YAML | Usually Artifact Registry (`REGION-docker.pkg.dev/PROJECT/REPO/IMAGE:TAG`). Prefer a digest or a fixed tag over `latest`. |
| Env vars | `env` in that YAML | Plain values move with `apps env import`. |
| Secret Manager references | `env` entries with `valueFrom.secretKeyRef`, and secret volume mounts | Cloud Run resolves these at start. Levelrail has its own secret store, so you copy the values in. |
| Min and max instances, concurrency | `autoscaling.knative.dev/minScale`, `maxScale` and `containerConcurrency` | They map to a fixed replica count, see section 2. |
| CPU and memory | `resources.limits` | They map to `resources.cpu` and `resources.memory`. |
| Domains | `gcloud beta run domain-mappings list --region REGION`, or your load balancer config | You recreate each one and cut DNS over. |
| Cloud SQL connections | The `run.googleapis.com/cloudsql-instances` annotation | Each one needs its own data move, see section 5. |
| Cloud Scheduler jobs | `gcloud scheduler jobs list` | Scheduler calls an HTTP URL. Levelrail scheduled tasks run a command in the container. |
| Service account and IAM | The service's runtime service account | Anything the app does with Google APIs through that identity needs a replacement, see section 8. |
| VPC connector and egress settings | The service YAML | Reaching private Google resources from outside Google needs your own network path. |

## 2. Map Cloud Run concepts to Levelrail

| Cloud Run | Levelrail equivalent | Notes |
| --- | --- | --- |
| Service | An app with one service | See [Deploying apps](/deploying-apps). |
| Container image from a registry | `apps create --image`, or `image:` in `app.yaml` | Private registries use a registry credential, see section 3. |
| Revision | A deploy | Each deploy is recorded with its image, status and reason. `apps deploys list <app>` shows them. |
| Traffic to a previous revision, or revert | `apps deploys rollback-to <app> <deploy-id>`, or `apps rollback <app> --image <tag>` | Prior images are pinned so garbage collection cannot remove a rollback target. |
| Gradual traffic split between revisions | Canary deploys, see [Canary deploys](/canary-deploys) | Check that page for the exact split behavior before relying on it. |
| Min instances | `replicas` | Fixed, always running. `apps scale <app> --replicas N`. |
| Max instances and autoscaling | None | There is no autoscaling. You choose the replica count. |
| Concurrency per instance | No setting | The container serves as many concurrent requests as it can handle. Replicas share load through the ingress, see [Load balancing](/load-balancing). |
| CPU and memory limits | `resources.cpu` and `resources.memory` in `app.yaml` | `512Mi` and `1` style values. Cloud Run's CPU throttling when idle has no equivalent: the container always has its CPU. |
| Env vars | Plain env, `apps env import --file` | Applied at container create. |
| Secret Manager secrets | `apps secrets set`, envelope-encrypted and write-only | Values are never returned after save. See [Environment and secrets](/environment-and-secrets). |
| Startup probe | `health.readiness` | A deploy that fails readiness does not take traffic. |
| Liveness probe | `health.liveness` | |
| Custom domain with a managed certificate | `domains:` in the spec, or `apps domains add`, with Caddy handling TLS | See [Domains and ingress](/domains-and-ingress). |
| Cloud SQL | A managed database on Levelrail | You dump and restore the data yourself, section 5. |
| Cloud Scheduler to an HTTP endpoint | Scheduled tasks: `apps scheduled-tasks create` | A command inside the container, not an HTTP call. |
| Cloud Logging | `apps logs`, a node-local log store with search | See [Observability](/observability). |
| Cloud Monitoring request metrics | `apps requests` and `apps metrics` | Server-side request rate, errors and latency percentiles per app. |
| IAM invoker and "allow unauthenticated" | Public by default once a domain is attached. Per-domain basic auth and an auth gate exist | See [Domains and ingress](/domains-and-ingress). |
| Min instances at zero (scale to zero) | Opt-in whole-app sleep: `apps sleep enable <app> --after 30m` | Not the same thing, see section 6 and [Sleep when idle](/sleep-when-idle). |

## 3. Deploy the same container image

If the service already runs from an image in a registry, you can deploy that image as is.

### Public image

```bash
levelrail-cli apps create --name web --image docker.io/your-org/web:1.4.2 --port 8080
```

`--port` is the port your container listens on. Cloud Run tells your container which port to bind through the `PORT` environment variable. Set the same value as a plain env var so the app behaves the same here:

```bash
levelrail-cli apps env import web --file prod.env --apply
```

where `prod.env` contains `PORT=8080` along with your other plain variables.

### Private image in Artifact Registry

Create a registry credential first. Artifact Registry accepts a service account key as the password with the username `_json_key`, and the host is `REGION-docker.pkg.dev`:

```bash
levelrail-cli registry-credentials create \
  --name gar-private \
  --registry-host us-central1-docker.pkg.dev \
  --username _json_key \
  --password "$(cat sa-key.json)"
```

Create a service account that has only the `Artifact Registry Reader` role for this, and keep the key out of your shell history. Check the credential before the first deploy:

```bash
levelrail-cli registry-credentials test <id>
```

Then reference it from an `app.yaml` and create the app from the file:

```yaml
version: 1
services:
  web:
    build:
      type: image
      image: us-central1-docker.pkg.dev/PROJECT/REPO/web:1.4.2
      registryCredential: gar-private
    port: 8080
    replicas: 1
    resources:
      memory: 512Mi
      cpu: 1
    health:
      readiness: { path: /healthz, interval: 5s, timeout: 2s }
    env:
      PORT: "8080"
      DATABASE_URL: { secret: true, required: true }
```

```bash
levelrail-cli apps create --file app.yaml --secret DATABASE_URL=postgres://...
```

A `{ secret: true, required: true }` variable must be supplied with `--secret` at creation, otherwise the CLI refuses to create the app. The field reference is in [app.yaml reference](/app-spec-reference), and the registry credential steps are in [Backup targets, registry credentials and app volume backups](/backups-and-storage#registry-credentials).

### Image not reachable from your server

If you would rather not give the server a Google credential, pull the image where you have access, retag it and push it to a registry the server can reach:

```bash
docker pull us-central1-docker.pkg.dev/PROJECT/REPO/web:1.4.2
docker tag us-central1-docker.pkg.dev/PROJECT/REPO/web:1.4.2 registry.example.com/web:1.4.2
docker push registry.example.com/web:1.4.2
```

Image deploys do not rebuild anything. If you want Levelrail to build from your repo instead, that is the git flow in [Deploying apps](/deploying-apps#from-a-git-repository).

### Redeploy and roll back

```bash
levelrail-cli apps deploy web --image us-central1-docker.pkg.dev/PROJECT/REPO/web:1.4.3
levelrail-cli apps deploys list web
levelrail-cli apps deploys rollback-to web <deploy-id> --watch
```

## 4. Env vars, secrets, health checks

1. Plain env. Copy the non-secret entries from the exported service YAML into a `.env` file, then `apps env import <app> --file prod.env --dry-run`, then again without `--dry-run`.
2. Secrets. Read each value from Secret Manager and set it as a Levelrail secret:

   ```bash
   levelrail-cli apps secrets set web DATABASE_URL "$(gcloud secrets versions access latest --secret=DATABASE_URL)"
   ```

   For many keys write a `secrets.env` file and use `apps secrets set web --env-file secrets.env`. Delete the file afterwards. There is no live link to Secret Manager: rotating a secret in Google does not change it here.
3. Apply. `apps apply web` restarts the app so saved env and secrets reach the running container.
4. Secret files. Cloud Run can mount a secret as a file. Levelrail injects secrets as env vars. If your code reads a file path, either change it to read the env var or write the file from the env var in your entrypoint.
5. Health. A Cloud Run startup probe becomes a readiness probe, and a liveness probe stays a liveness probe:

   ```bash
   levelrail-cli apps health set web --probe readiness --path /healthz --interval 5s
   levelrail-cli apps health set web --probe liveness --path /healthz --failures 3
   ```

   A deploy that fails readiness does not take over traffic: the ingress keeps routing to the previous release and the attempt is recorded as failed. Cloud Run's default TCP startup check has no direct counterpart, so add a real `/healthz` route.

## 5. Move Cloud SQL data

A managed database on Levelrail is a container on your node with its own volume, credentials and scheduled backups. It is not a hosted service with a replica in another zone. Read [Managing databases](/managing-databases) before you commit.

1. Create the database with a matching major version:

   ```bash
   levelrail-cli databases create --name main --engine postgres --version 16
   levelrail-cli databases status main
   ```

2. Dump from Cloud SQL. Use the Cloud SQL Auth Proxy or an authorized network, then:

   ```bash
   pg_dump --format=custom --no-owner --no-acl \
     --host=127.0.0.1 --port=5432 --username=app --dbname=appdb \
     --file=appdb.dump
   ```

   Alternatively use `gcloud sql export sql INSTANCE gs://BUCKET/appdb.sql --database=appdb` and download the file.
3. Restore into the new database. Publish the database port temporarily, then restore with `pg_restore` using the database's own credentials:

   ```bash
   levelrail-cli databases public-access set main --port 5433
   pg_restore --no-owner --no-acl --dbname="postgres://USER:PASSWORD@SERVER_IP:5433/DBNAME?sslmode=require" appdb.dump
   levelrail-cli databases public-access clear main
   ```

   The credentials are the ones Levelrail generated for the database (see the Overview tab and `levelrail-cli databases get main`). A temporary login from `databases access temp` with `--preset read_write` cannot change the schema, so it is not enough for a first restore. Clear public access when you are done.
4. Connect the app to the database instead of pasting a URL:

   ```bash
   levelrail-cli apps connect web main --env-var DATABASE_URL
   ```

5. Rehearse the cutover. A dump is a point in time. For a live database, plan a short write freeze: stop writes on Cloud Run, take a final dump, restore, switch. Time the restore before the real day.
6. Backups. Configure a backup target and a schedule, then run a restore into a new database to prove it works. See [Backup targets](/backups-and-storage).

Cloud SQL's automatic failover, read replicas and point-in-time recovery are not replicated by default here. Point-in-time restore exists for Postgres, see [Managing databases](/managing-databases).

## 6. Domains, jobs, logs and metrics

### Domains and certificates

1. Add the domain to the app while DNS still points at Google:

   ```bash
   levelrail-cli apps domains add web app.example.com --dns off
   ```

2. Test before you move DNS, with a host override:

   ```bash
   curl --resolve app.example.com:443:SERVER_IP -k https://app.example.com/healthz
   ```

3. Lower the DNS TTL a day ahead, then change the A and AAAA records to the server's public IP. Cloud Run domain mappings point at Google's records, so those are replaced, not edited.
4. Certificates: with ACME enabled, issuance needs the domain to already point at the server, so expect a short window, or upload a certificate for the domain first. ACME has one recorded live run, see the [ACME verification runbook](/acme-verification-runbook).
5. Keep the Cloud Run service and its domain mapping untouched until the shadow run is clean. Restoring the old DNS records is the rollback.

### Scheduled jobs

A Cloud Scheduler job that POSTs to `/cron/cleanup` has no direct equivalent, because Levelrail runs a command, not an HTTP request. Options:

- Put the work behind a command your image already contains and schedule that: `levelrail-cli apps scheduled-tasks create web --schedule "0 3 * * *" -- ./cleanup.sh`.
- Keep the HTTP route and call it from a command that exists in the image, for example `curl` if it is installed. Confirm the tool exists before relying on it.

Run each once with `levelrail-cli apps scheduled-tasks run web <id>` and check `last_run_status`. More in [Scheduled jobs](/scheduled-jobs). Cloud Run Jobs (batch tasks) can use the `jobs:` block, which runs a command in a fresh container of the service's image.

### Logs and metrics

```bash
levelrail-cli apps logs web --since 1h --q error
levelrail-cli apps logs web --follow
levelrail-cli apps requests web
levelrail-cli apps metrics web --metric cpu_percent --since 1h --step 5m
```

Logs and metrics are stored on the node. If you have alerts or dashboards in Google Cloud Monitoring, rebuild them as Levelrail alert rules, see [Observability](/observability). Structured JSON logs written to stdout work as they do on Cloud Run, but the field names Google uses for severity and trace are not interpreted specially.

## 7. Cutover and shadow run

Run both platforms side by side long enough for boring problems to appear. Suggested minimum: one full business cycle, including any weekly or monthly job.

- [ ] A deliberately broken deploy is rejected while the old container keeps serving.
- [ ] A rollback to the previous image works from the CLI and the dashboard.
- [ ] Every env var key that exists on Cloud Run exists here, and secrets were copied, not left behind.
- [ ] The app reads `PORT` or the configured port and passes readiness on the first deploy.
- [ ] Webhooks, OAuth callback URLs and allowed origins point at the new hostname or still resolve through the same domain.
- [ ] Each scheduled task ran on schedule.
- [ ] A database backup ran, and a restore was rehearsed into a new database.
- [ ] Certificate issuance finished and expiry alerting is configured.
- [ ] Response time and error rate compare acceptably against Cloud Run, using `apps requests`.
- [ ] A control plane backup exists, see [Control plane backup](/control-plane-backup).
- [ ] The server has an owner for patching and disk monitoring.

Decommission the Cloud Run service only after this passes and you have decided what to do about each item in the next section.

## 8. What you give up

| Cloud Run feature | Status on Levelrail |
| --- | --- |
| Scale to zero | Not automatic per request. Whole-app sleep after a quiet period exists as an opt-in, and the first request waits for a cold start of seconds to a minute. Compare that to Cloud Run's much shorter cold starts. |
| Autoscaling | None. Replicas are fixed. A traffic spike beyond your server's capacity is a slow service, not a new instance. |
| Pay per request billing | You pay for the server whether it is busy or idle. See [Cloud Run alternative](/cloud-run-alternative) for when that favors you. |
| Multi-zone and regional redundancy | One server is one failure domain. A multi-node setup is possible, but you build and run it. |
| Google's global frontend and edge | Caddy serves from the nodes you run. Put a CDN in front if you need one. |
| Managed IAM, workload identity and service-to-service auth | Your app's Google API calls need a credential you provide, such as a service account key stored as a secret. |
| Cloud SQL operations (automated patching, HA, replicas) | You upgrade and back up the database container yourself, see [Database upgrades](/database-upgrades). |
| Automatic OS and runtime patching | The host and Docker are yours to patch. |
| Revision traffic percentages in the console | Canary deploys cover a subset, see [Canary deploys](/canary-deploys). |

## See also

- [Cloud Run alternative](/cloud-run-alternative)
- [Deploying and managing apps](/deploying-apps)
- [Domains and ingress](/domains-and-ingress)
- [Managing databases](/managing-databases)
- [app.yaml reference](/app-spec-reference)
- [Feature status](/feature-status)
- [Migrating from Vercel](/migrating-from-vercel)
