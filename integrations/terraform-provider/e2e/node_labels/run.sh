#!/usr/bin/env bash
# Drift then import for levelrail_node_labels.t. See README.md in this folder.
set -euo pipefail
# shellcheck source=../lib.sh
. "$(dirname "$0")/../lib.sh"

write_config() {
  cat <<'HCL'
terraform {
  required_providers {
    levelrail = { source = "registry.terraform.io/glincker/levelrail" }
  }
}
HCL
  cat <<HCL
resource "levelrail_node_labels" "t" {
  node   = "$E2E_NODE"
  labels = { e2e = "one" }
}
HCL
}

mutate() {
  local id
  id="$(e2e_api GET /api/v1/nodes | jq -r --arg n "$E2E_NODE" '.[] | select(.name == $n) | .id' | head -1)"
  [ -n "$id" ] || e2e_die "node $E2E_NODE not found"
  e2e_api PUT "/api/v1/nodes/$id/labels" '{"labels":{"e2e":"changed"}}' >/dev/null
}

import_id() {
  echo "$E2E_NODE"
}

e2e_guard
: "${E2E_NODE:?set E2E_NODE to the name of an existing node}"
e2e_flow 'levelrail_node_labels.t'
