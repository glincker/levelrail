#!/usr/bin/env bash
# Drift then import for levelrail_setting.t. See README.md in this folder.
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

resource "levelrail_setting" "t" {
  key   = "deploy.inline_source_keep"
  value = "7"
}
HCL
}

mutate() {
  e2e_api PUT /api/v1/settings/runtime/deploy.inline_source_keep '{"value":9}' >/dev/null
}

import_id() {
  echo deploy.inline_source_keep
}

e2e_guard
e2e_flow 'levelrail_setting.t'
