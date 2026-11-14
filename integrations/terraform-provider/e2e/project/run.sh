#!/usr/bin/env bash
# Drift then import for levelrail_project.t. See README.md in this folder.
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

resource "levelrail_project" "t" {
  name = "e2e-drift-project"
}
HCL
}

mutate() {
  local id
  id="$(e2e_api GET /api/v1/projects | jq -r '.[] | select(.name == "e2e-drift-project") | .id' | head -1)"
  [ -n "$id" ] || e2e_die "project not found"
  e2e_api DELETE "/api/v1/projects/$id" >/dev/null
}

import_id() {
  e2e_api GET /api/v1/projects | jq -r '.[] | select(.name == "e2e-drift-project") | .id' | head -1
}

e2e_guard
e2e_flow 'levelrail_project.t'
