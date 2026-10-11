#!/usr/bin/env bash
# Prints the product version from git: nearest v* tag, never a module tag such as kit/v0.1.0.
set -euo pipefail
cd "${1:-$(dirname "$0")/..}"
git describe --tags --match 'v*' --exclude 'kit/*' --always --dirty 2>/dev/null || echo dev
