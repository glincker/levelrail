# 074. Runtime options allowlist

Status: accepted

## Context

Apps need a few `docker run` options: shared memory, tmpfs, ulimits, init, stop timeout, extra hosts, DNS, namespaced sysctls, dropped capabilities, a PID cap.

## Decision

A `runtime:` block accepts only options that cannot widen the container's reach into the host, each with bounded sizes and counts from `APP_RUNTIME_*` variables. Sysctls are limited to a fixed list of namespaced `net.*` keys. `cap_add`, `privileged`, `devices`, `security_opt`, host `pid`/`ipc`/`uts`/`network_mode`/`userns_mode` and host bind mounts stay refused with an error naming the key. Agents advertise `container_spec_runtime`; older agents fail the deploy instead of dropping the options.

## Proposal for a later decision

Allow `cap_add` and `devices` only for a root user, from a short list of named capabilities and device paths, with an approval record and an audit event on every change, and never for an app writer role. Not part of this change.
