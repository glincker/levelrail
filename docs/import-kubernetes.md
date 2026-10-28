# Import Kubernetes manifests

`levelrail-cli import k8s` reads existing Kubernetes YAML and creates the matching Levelrail apps, so a manifest you already have runs here without a rewrite. The conversion is text only: nothing is fetched from a URL, nothing is executed, and the cluster is never contacted.

```
levelrail-cli import k8s -f manifests.yaml --dry-run
levelrail-cli import k8s -f manifests.yaml
cat manifests.yaml | levelrail-cli import k8s -f - -o json
```

Flags: `--dry-run` prints the documents and the report and changes nothing. `--app-prefix P` prefixes every app name. `--namespace-as environment|prefix|ignore` decides what a namespace becomes (default `environment`, one environment per namespace under the project named by `--project`, default `kubernetes`). `--force` applies even when the report holds blockers. `-o json|table` picks the output.

The API is `POST /api/v1/import/k8s?dry_run=true|false&force=&app_prefix=&namespace_as=&project=` with the YAML as the body (or `{"manifest": "..."}` as JSON). It needs the same permission as applying documents, and every app, domain and secret is created with the caller's own abilities.

## What maps

| Kubernetes | Levelrail |
| --- | --- |
| Deployment, StatefulSet | an app: image, replicas, strategy (`RollingUpdate` to `rolling`, `Recreate` to `recreate`) |
| DaemonSet | one replica, reported |
| Job | an app that runs to completion (`restart: no`, or `on-failure` for `OnFailure`) |
| CronJob | an app plus a scheduled job (schedule, time zone, concurrency policy, suspend, active deadline) |
| container `command` and `args` | `command` (args alone replace the image CMD; `command` is reported) |
| `env` values | env; names that look like secrets become secrets |
| `configMapKeyRef`, `envFrom` ConfigMap | env, resolved from ConfigMaps in the same input |
| `secretKeyRef`, `envFrom` Secret | app secrets, resolved from Secrets in the same input |
| `resources.limits` (or requests when no limit) | `resources.memory` and `resources.cpu` |
| `httpGet` and `exec` probes | `health.readiness` and `health.liveness` with path, interval, timeout and failures |
| `startupProbe` | `health.readyTimeout` |
| PVC and `volumeClaimTemplates` mounts, `emptyDir` | named volumes |
| Service ports | the routed `port`; NodePort and LoadBalancer ports (and any UDP) become `published_ports` |
| Ingress host rules | `domains` on the app behind the backend Service |

Output is the stack document format `levelrail-cli apply` reads, so the result goes through the normal apply path.

## What does not map

Everything the importer cannot carry over lands in the report with a kind, a name, a field path, a reason and a severity:

- `ignored-safe`: no equivalent, and ignoring it changes nothing that runs (selectors, history limits, service accounts, requests next to a limit, TLS secrets, storage size).
- `changes-behavior`: the app runs, but differently. Examples: sidecar containers (they share localhost in a pod, services here do not, so run them as their own apps), init containers, TCP probes, ConfigMap or Secret files mounted into the container, `subPath`, path-based Ingress rules (domains route by host), `emptyDir` becoming persistent, in-cluster DNS names in env values, node selectors and affinity, unsupported kinds (skipped and listed).
- `blocker`: the manifest asks for something the platform will not grant. A non-dry-run import refuses while any blocker remains, unless you pass `--force` (the blocked setting is still left out).

Every app needs a port, so a workload that declares none gets the placeholder `APP_K8S_IMPORT_DEFAULT_PORT` (default 8080) and a report line.

## Security refusals

These are always blockers: `hostNetwork`, `hostPID`, `hostIPC`, `hostPath` volumes, `privileged` containers and `securityContext.capabilities.add`. The platform has no equivalent and the import never weakens isolation to fit.

Secret `data` and `stringData` become app secrets. Values are never printed, logged, returned in a dry run or written into the documents; only key names appear. Dry runs show env keys and secret key names.

Input limits, all from the environment: `APP_K8S_IMPORT_MAX_BYTES` (default 2 MiB), `APP_K8S_IMPORT_MAX_DOCS` (200), `APP_K8S_IMPORT_MAX_NODES` (200000, counted after alias expansion), `APP_K8S_IMPORT_MAX_DEPTH` (64) and `APP_K8S_IMPORT_MAX_ANCHOR_DEPTH` (8). A file over a limit, or a YAML alias bomb, is refused before anything is converted.

## Example: web app with an Ingress

```yaml
apiVersion: apps/v1
kind: Deployment
metadata: {name: web}
spec:
  replicas: 2
  selector: {matchLabels: {app: web}}
  template:
    metadata: {labels: {app: web}}
    spec:
      containers:
        - name: web
          image: nginx:1.27
          ports: [{name: http, containerPort: 80}]
          readinessProbe: {httpGet: {path: /healthz, port: http}}
---
apiVersion: v1
kind: Service
metadata: {name: web}
spec:
  selector: {app: web}
  ports: [{name: http, port: 80, targetPort: http}]
---
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata: {name: web}
spec:
  rules:
    - host: shop.example.com
      http:
        paths:
          - path: /
            pathType: Prefix
            backend: {service: {name: web, port: {number: 80}}}
```

`levelrail-cli import k8s -f web.yaml --dry-run` shows one app `web` on port 80 with two replicas, the domain `shop.example.com` and a readiness check on `/healthz`.

## Example: Postgres StatefulSet

A StatefulSet with a `volumeClaimTemplates` entry named `data`, mounted at `/var/lib/postgresql/data`, and a Secret holding `POSTGRES_PASSWORD` imports as one app with a named volume `data` at that path and `POSTGRES_PASSWORD` as an app secret. The report notes that the storage size request is not enforced. For a managed database you can also create a `postgres` database resource instead and point the app at it.
