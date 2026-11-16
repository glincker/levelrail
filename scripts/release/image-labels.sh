#!/usr/bin/env bash
# Prints OCI image labels, one per line, for docker/build-push-action `labels`.
# Usage: scripts/release/image-labels.sh <title> <description> <version> <sha> [created]
# Env: none. A leading v on the version is dropped; created defaults to now (UTC).
set -euo pipefail

title="${1:?title}"
description="${2:?description}"
version="${3:?version}"
sha="${4:?sha}"
created="${5:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}"

cat <<LABELS
org.opencontainers.image.source=https://github.com/glincker/levelrail
org.opencontainers.image.version=${version#v}
org.opencontainers.image.revision=${sha}
org.opencontainers.image.created=${created}
org.opencontainers.image.title=${title}
org.opencontainers.image.description=${description}
org.opencontainers.image.url=https://levelrail.com
org.opencontainers.image.licenses=Apache-2.0
org.opencontainers.image.vendor=GLINR STUDIOS
LABELS
