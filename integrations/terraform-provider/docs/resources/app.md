---
page_title: "levelrail_app Resource - levelrail"
subcategory: "Apps"
description: |-
  An app deployed from a container image. Domains are managed with the domain resource, not here.
---

# levelrail_app (Resource)

An app deployed from a container image. Domains are managed with the domain resource, not here.

## Example Usage

```terraform
resource "levelrail_app" "web" {
  name       = "web"
  image      = "nginx:1.27"
  port       = 80
  replicas   = 2
  strategy   = "rolling"
  project_id = levelrail_project.shop.id

  internal_dns_names = ["shop"]

  memory_mb = 256
  cpus      = 0.5
  restart   = "on-failure:3"

  readiness_path = "/"
  liveness_tcp   = true

  volumes = [{
    name           = "cache"
    container_path = "/var/cache/nginx"
  }]

  tags = ["team:shop"]

  registry_credential = "ghcr-pull"

  labels = {
    team = "shop"
  }

  published_ports = [{
    container_port = 80
    host_port      = 8080
  }]
}
```

## Argument Reference

### Required

- `image` (String) Container image reference, for example nginx:1.27.
- `name` (String) Name of the resource.

### Optional

- `bind_address` (String) Host interface the app port binds to. Computed when unset.
- `command` (List of String) Overrides the image command. Unset leaves the live command untouched.
- `cpus` (Float64Attribute) CPU limit as a number of CPUs, fractions allowed, for example 0.5. 0 means no limit. Preferred over nano_cpus; set only one of them.
- `env` (Map of String) Plain environment variables. Unset to leave live variables untouched. Sensitive.
- `internal_dns_names` (List of String) Extra internal DNS names under the mesh zone that resolve like the app name (beta, needs the runtime setting dns.internal_names_enabled). Unset leaves the live names untouched, an empty list clears them.
- `labels` (Map of String) Container labels. Unset leaves live labels untouched.
- `liveness_exec` (List of String) Liveness probe as a command run inside the container, healthy on exit 0. Unset leaves the live probe untouched.
- `liveness_path` (String) HTTP path of the liveness probe. Empty means no probe. Not combined with liveness_exec or liveness_tcp. Computed when unset.
- `liveness_tcp` (Boolean) Liveness probe that only opens a TCP connection to the app port. Unset leaves the live probe untouched.
- `memory_bytes` (Number) Memory limit in bytes. 0 means no limit. Computed when unset.
- `memory_mb` (Float64Attribute) Memory limit in MiB, for example 256. 0 means no limit. Preferred over memory_bytes; set only one of them.
- `nano_cpus` (Number) CPU limit in billionths of a CPU (1000000000 is one CPU). 0 means no limit. Computed when unset.
- `port` (Number) Container port the app listens on. Computed when unset.
- `project_id` (String) Project the app belongs to. Computed when unset.
- `published_ports` (List of Object) Extra host ports published for the app. Unset leaves live ports untouched.
- `readiness_exec` (List of String) Readiness probe as a command run inside the container, healthy on exit 0. Unset leaves the live probe untouched.
- `readiness_path` (String) HTTP path of the readiness probe. Empty means no probe. Not combined with readiness_exec or readiness_tcp. Computed when unset.
- `readiness_tcp` (Boolean) Readiness probe that only opens a TCP connection to the app port. Unset leaves the live probe untouched.
- `registry_credential` (String) Name of an existing registry credential used to pull the image. Never a secret value; create the credential separately. Unset leaves the live credential untouched.
- `replicas` (Number) Number of running instances. Computed when unset.
- `restart` (String) Restart policy: no or on-failure[:N]. Empty keeps the app running. Computed when unset.
- `strategy` (String) Rollout strategy, for example rolling or recreate. Computed when unset.
- `tags` (Set of String) Tags on the app, lowercase, as key or key:value. Unset leaves live tags untouched; an empty set removes them all.
- `volumes` (List of Object) Named volumes mounted into the container. Unset leaves live volumes untouched; an empty list detaches them all.

## Attributes Reference

No read-only attributes.

### Nested attributes

- `published_ports.bind` (String, Optional) private or public for this port. Empty follows bind_address. Computed when unset.
- `published_ports.container_port` (Number, Required) Port inside the container.
- `published_ports.host_port` (Number, Required) Port on the host.
- `published_ports.protocol` (String, Optional) tcp or udp. Defaults to tcp. Computed when unset.
- `volumes.container_path` (String, Required) Absolute mount path inside the container.
- `volumes.name` (String, Required) Volume name: lowercase letters, digits and hyphens, starting with a letter.

## Import

Import by the app name. Lists, sets and maps show as unset until you add them to the configuration.

```shell
terraform import levelrail_app.example APP_NAME
```
