#!/bin/sh
set -eu
if [ -z "${GREETING:-}" ]; then
	echo "missing required env GREETING" >&2
	exit 1
fi
mkdir -p /tmp/www
echo "$GREETING" >/tmp/www/index.html
exec busybox httpd -f -p 8080 -h /tmp/www
