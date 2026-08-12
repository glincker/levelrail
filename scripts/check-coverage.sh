#!/usr/bin/env bash
# Coverage gate for CLAUDE.md section 7: "Coverage gate at 70% for
# /internal, enforced in CI."
#
# Reads a go test coverprofile (produced by
# `go test ./... -race -coverprofile=coverage.out`) and computes coverage
# scoped only to files whose import path contains the given prefix, so
# packages outside that prefix (cmd/, proto/, etc.) cannot dilute or
# inflate the number either way.
#
# Usage: scripts/check-coverage.sh <profile> <path-prefix> <threshold-percent>
# Example: scripts/check-coverage.sh coverage.out internal/ 70

set -euo pipefail

PROFILE="${1:?usage: check-coverage.sh <profile> <path-prefix> <threshold>}"
PREFIX="${2:?usage: check-coverage.sh <profile> <path-prefix> <threshold>}"
THRESHOLD="${3:?usage: check-coverage.sh <profile> <path-prefix> <threshold>}"

if [ ! -f "$PROFILE" ]; then
  echo "coverage profile not found: $PROFILE" >&2
  exit 1
fi

FILTERED="$(mktemp)"
trap 'rm -f "$FILTERED"' EXIT

# First line of a coverprofile is the mode line (e.g. "mode: atomic"), the
# rest are one statement-block per line. Keep the mode line, then keep only
# statement lines whose file path contains the prefix we're gating on.
head -n1 "$PROFILE" > "$FILTERED"
grep "/${PREFIX}" "$PROFILE" >> "$FILTERED" || true

MATCHED_LINES="$(($(wc -l < "$FILTERED") - 1))"
if [ "$MATCHED_LINES" -le 0 ]; then
  echo "no coverage data found for path prefix '${PREFIX}' in ${PROFILE}" >&2
  exit 1
fi

TOTAL_LINE="$(go tool cover -func="$FILTERED" | tail -n1)"
PERCENT="$(echo "$TOTAL_LINE" | awk '{print $NF}' | tr -d '%')"

echo "Coverage for '${PREFIX}': ${PERCENT}% (threshold: ${THRESHOLD}%)"

# Compare as tenths of a percent so this doesn't depend on bc or bash
# floating point, which don't do fractional comparisons.
PERCENT_X10="$(awk -v p="$PERCENT" 'BEGIN { printf "%d", p * 10 }')"
THRESHOLD_X10="$(awk -v t="$THRESHOLD" 'BEGIN { printf "%d", t * 10 }')"

if [ "$PERCENT_X10" -lt "$THRESHOLD_X10" ]; then
  echo "FAIL: coverage ${PERCENT}% is below the ${THRESHOLD}% gate for '${PREFIX}'" >&2
  exit 1
fi

echo "PASS: coverage ${PERCENT}% meets the ${THRESHOLD}% gate for '${PREFIX}'"
