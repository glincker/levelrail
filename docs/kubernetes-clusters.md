---
title: "Connect and deploy to an existing Kubernetes cluster"
sidebarTitle: "Kubernetes clusters"
description: "Connect a Kubernetes cluster you already run, deploy a public image to it, and read status and logs from Levelrail. Experimental and off by default."
"og:url": "https://docs.levelrail.com/kubernetes-clusters"
---

Levelrail can connect a Kubernetes cluster you already run and deploy a container image to it. This feature is experimental and off by default. It adds a second deploy target next to the single-server Docker runtime and does not replace it.

## How do I turn it on?

Set the runtime setting `k8s.clusters_enabled` to `true`. It is off by default and applies without a restart.

```bash
levelrail-cli settings runtime set k8s.clusters_enabled true
```

You can also use Settings > Security and runtime, Experimental card. While it is off, the cluster API refuses requests with `kubernetes clusters are off; enable the runtime setting k8s.clusters_enabled`, and the Clusters page in the dashboard shows "Kubernetes clusters are off". All cluster routes are root only. Every `k8s.*` setting is listed in [Runtime settings](/runtime-settings).

## How do I connect a cluster?

Use `levelrail-cli clusters connect` with either a kubeconfig or a server URL plus a service-account token. The cluster name must be lowercase letters, digits and dashes, up to 63 characters.

```bash
levelrail-cli clusters connect prod --kubeconfig ./kubeconfig.yaml --context prod-admin
levelrail-cli clusters connect prod --server https://203.0.113.10:6443 --token-file ./sa.token --ca-file ./ca.pem
```

Credentials come from a file, or `-` for stdin, never from an argument. Only one input can read stdin. The dashboard offers the same two methods under Clusters, Connect a cluster: "Kubeconfig" and "Server and token".

Connecting checks the credential and stores it. It does not deploy anything. To check the connection again, run a read-only test:

```bash
levelrail-cli clusters test prod
```

The output looks like `ok=true class=ok cluster_admin=false`.

## What kubeconfig content is accepted?

Only a kubeconfig whose context carries a token or an embedded client certificate and key, with an embedded CA when one is needed. Levelrail never reads files from its own host on behalf of a kubeconfig, so entries that point to files or run programs are refused.

The refused fields are: `certificate-authority` (file path), `insecure-skip-tls-verify`, `proxy-url`, `tokenFile`, `client-certificate` and `client-key` (file paths), `exec`, `auth-provider`, basic auth, and impersonation. Use `certificate-authority-data`, `client-certificate-data` and `client-key-data` instead. The kubeconfig size is capped by `k8s.kubeconfig_max_bytes` (65536 bytes by default).

## What permissions does Levelrail need on the cluster?

A ServiceAccount limited to one namespace is enough to deploy. Print the manifests with `clusters rbac` and apply them with your own tooling.

```bash
levelrail-cli clusters rbac --namespace levelrail-managed
```

This prints a ServiceAccount, a Role and a RoleBinding in that namespace. The Role allows get, list, watch, create, update, patch and delete on `deployments`, `services`, `ingresses`, `pods`, `pods/log` and `secrets` in the API groups `""`, `apps` and `networking.k8s.io`. Use `--service-account NAME` to change the account and Role name (default `levelrail`).

Listing nodes and namespaces needs cluster-scope reads, which that Role does not grant. Add `--cluster-read` to also print a read-only ClusterRole and ClusterRoleBinding for `nodes`, `namespaces` and `ingresses` (the last lets a deploy with `--host` check that no other ingress claims the host):

```bash
levelrail-cli clusters rbac --namespace levelrail-managed --cluster-read
```

Create the service account token yourself and pass it with `--token-file`.

## Why was my cluster-admin credential refused?

Connecting checks the credential and refuses one that is effectively cluster-admin (wildcard verbs on all resources and API groups). The same refusal applies when the check cannot be completed. To connect anyway, turn on `k8s.allow_cluster_admin` and pass `--allow-cluster-admin`:

```bash
levelrail-cli settings runtime set k8s.allow_cluster_admin true
levelrail-cli clusters connect prod --kubeconfig ./kubeconfig.yaml --allow-cluster-admin
```

Both are needed. Weakening the setting asks for confirmation, and the connect is recorded in the audit log as a cluster-admin override. A namespaced Role from `clusters rbac` is the recommended path.

## Can I connect a local or private cluster?

Yes, after turning on `k8s.allow_private_api`. API servers on private or loopback addresses (a local Kind or k3d cluster, a private network) are refused by default. Metadata and link-local addresses are always refused.

```bash
levelrail-cli settings runtime set k8s.allow_private_api true
```

## What can I do after connecting?

You can inspect the cluster, deploy a public image, watch its status, read logs and remove the app. In the dashboard these are on the Clusters page and the cluster detail page. The CLI commands are:

```bash
levelrail-cli clusters list
levelrail-cli clusters show prod
levelrail-cli clusters nodes prod
levelrail-cli clusters namespaces prod
levelrail-cli clusters deployments prod
levelrail-cli clusters status prod web
levelrail-cli clusters logs prod web --tail 100
levelrail-cli clusters remove prod web --yes
levelrail-cli clusters disconnect prod
```

`nodes` shows readiness, roles and kubelet version. `status` shows rollout state, conditions, pods and the ingress address. `logs` reads the first running pod of the app unless you pass `--pod`; the line count is capped by `k8s.log_tail_lines_max` (500 by default). Without `--yes`, `remove` only previews. With it, Levelrail deletes the app's Deployment, Service and Ingress and leaves the namespace. Only objects Levelrail created are touched. Every verb accepts `--json`.

## How do I deploy an app?

Run `clusters deploy` with an app name, a public image and the container port.

```bash
levelrail-cli clusters deploy prod --app web --image nginx:1.27 --port 80 --replicas 2 --host web.example.com --namespace levelrail-managed
```

Flags: `--app`, `--image` and `--port` are required. `--replicas`, `--host`, `--namespace`, `--env-file FILE` (KEY=VALUE lines, or `-` for stdin, values never printed) and `--allow-root` are optional. The dashboard form has the same fields.

What Levelrail creates and enforces:

- A Deployment and a Service, plus an Ingress when you set a host. All carry the `app.kubernetes.io/managed-by` label.
- The default namespace is `levelrail-managed` (`k8s.default_namespace`). To use a namespace Levelrail did not create, list it in `k8s.allowed_namespaces`.
- CPU and memory requests and limits on every container, from `k8s.cpu_request`, `k8s.cpu_limit`, `k8s.memory_request` and `k8s.memory_limit`.
- Containers run as user 10001 (`k8s.run_as_user`) with all capabilities dropped. `--allow-root` lets the image run as its own user and adds back minimal capabilities.
- Limits per request: replicas up to `k8s.max_replicas` (20), up to `k8s.max_env_vars` variables (64) of up to `k8s.max_env_value_bytes` bytes (4096).
- A host already used by an Ingress that Levelrail does not own is refused.

Environment variables are written into the Deployment as plain `env` entries.

## How does this relate to the Docker runtime?

It is an additional deploy target. Apps you create the usual way still run on your own servers through the Docker runtime. A cluster deploy is a separate path: Levelrail calls the cluster API and records the deployment, and it does not move or manage your Docker apps. Disconnecting a cluster deletes the stored credential and the deployment records Levelrail kept for it. Workloads already on the cluster are not touched. To convert existing YAML into Levelrail apps instead, see [Import Kubernetes manifests](/import-kubernetes).

## How are credentials stored?

Credentials are stored in Levelrail's secrets store, which needs the secrets master key, in a per-cluster namespace. The kubeconfig is sent once and not kept locally. The cluster list, show and API responses never include credential material; they show the server, auth mode (`token` or `client_cert`), a CA fingerprint and the last check result. Connects, tests and disconnects are written to the audit log with names and hosts, not credentials. The stored CA is compared with the fingerprint taken at connect time before each use. `clusters disconnect` still works after `k8s.clusters_enabled` is turned off, so stored credentials can always be removed.

## What is not supported yet?

- Kubeconfig entries that use files, `exec`, `auth-provider`, basic auth, impersonation, a proxy, or skipped TLS verification are refused.
- Deploys take a public registry image. References to `localhost`, `127.*` and `.local` registries are refused because a remote cluster cannot pull from them.
- The setting `k8s.in_cluster_enabled` exists for connecting the cluster Levelrail itself runs in, but this page does not cover it.

The feature itself is experimental. See [Feature status](/feature-status) for how labels are used.

## Troubleshooting

- `kubernetes clusters are off; enable the runtime setting k8s.clusters_enabled`: turn the setting on.
- `storing cluster credentials needs the secrets master key`: set up the master key. If `APP_MASTER_KEY` is set and malformed, fix it or unset it to use the key file.
- `api server address refused: private and loopback addresses need the runtime setting k8s.allow_private_api`: see the private cluster section above.
- `this credential is effectively cluster-admin ... and is refused` or `could not verify the credential is not cluster-admin`: use a namespaced Role, or the override described above.
- `k8sconn: kubeconfig field refused: exec` (or another field name): replace the entry with embedded data or a token.
- `k8sconn: context "NAME" not found`: check the `--context` value.
- `pass exactly one of --kubeconfig or --server` and `--server needs --token-file`: give one connection method.
- `a cluster with this name already exists`: pick another name or disconnect first.
- `cluster is unreachable or its credentials cannot be loaded`: run `clusters test NAME` and check the address and the token.
- `the cluster refused the stored credential: it lacks the cluster-scope permission`: add the ClusterRole from `clusters rbac --cluster-read`.
- `the namespace already exists and is not managed by levelrail; add it to k8s.allowed_namespaces to use it`: add it to the setting.
- `the namespace does not exist and the service account cannot create namespaces`: deploy into the namespace the role was made for (`clusters rbac --namespace NS`). A namespace you created by hand also needs `k8s.allowed_namespaces`, unless it is the default namespace or carries the levelrail label.
- `the host is already used by an ingress that levelrail does not own`: choose another host.
- `the host cannot be verified: levelrail needs permission to list ingresses across namespaces`: apply `clusters rbac --cluster-read`. To override, send `skip_host_check` while `k8s.allow_unverified_hosts` is on; the CLI has no flag for it.
- `an object with this name already exists in the namespace and is not managed by levelrail`: rename the app or remove the object.
- `another deploy of this app on this cluster is already running`: wait and retry.
