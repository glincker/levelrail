#!/usr/bin/env bash
# Renders every CLI tape to docs/assets/demos/*.gif. Needs vhs, ttyd, ffmpeg.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/../.."
mkdir -p docs/assets/demos
for t in install-deploy deploy-rollback db-create templates; do
  vhs "scripts/demos/$t.tape"
done
ls -l docs/assets/demos
