#!/usr/bin/env bash
# Release pacing guard for the release-please PR: enforces a cooldown since the
# last platform tag and refuses an unapproved minor/major jump while pre-1.0.
# Env: PR_TITLE (chore(main): release X.Y.Z), PR_LABELS (comma list),
# RELEASE_COOLDOWN_HOURS (default 6), NOW (epoch, tests only).
set -euo pipefail

cooldown_hours="${RELEASE_COOLDOWN_HOURS:-6}"
now="${NOW:-$(date +%s)}"
title="${PR_TITLE:-}"
labels=",${PR_LABELS:-},"

next="$(sed -nE 's/.*release ([0-9]+\.[0-9]+\.[0-9]+).*/\1/p' <<<"$title" | head -1)"
if [ -z "$next" ]; then
  echo "not a release PR title, nothing to pace"
  exit 0
fi

last_tag="$(git tag -l 'v[0-9]*.[0-9]*.[0-9]*' --sort=-v:refname | grep -Ev -- '-' | head -1 || true)"
if [ -z "$last_tag" ]; then
  echo "no previous release, nothing to pace"
  exit 0
fi
last="${last_tag#v}"

IFS=. read -r lmaj lmin _ <<<"$last"
IFS=. read -r nmaj nmin _ <<<"$next"

if [ "$nmaj" = 0 ] && { [ "$nmaj" != "$lmaj" ] || [ "$nmin" != "$lmin" ]; } && [[ "$labels" != *",allow-minor-release,"* ]]; then
  echo "::error::$last_tag to v$next is a minor jump while pre-1.0. Patch releases are the default (0.8.1, 0.8.2); add the allow-minor-release label to ship it."
  exit 1
fi

tagged_at="$(git log -1 --format=%ct "$last_tag")"
age=$(( now - tagged_at ))
need=$(( cooldown_hours * 3600 ))
if [ "$age" -lt "$need" ] && [[ "$labels" != *",skip-release-cooldown,"* ]]; then
  wait_min=$(( (need - age + 59) / 60 ))
  echo "::error::$last_tag was cut $(( age / 60 )) min ago; cooldown is ${cooldown_hours}h. Re-checks automatically in about ${wait_min} min (label skip-release-cooldown overrides)."
  exit 1
fi

echo "pacing ok: $last_tag -> v$next, last release $(( age / 3600 ))h ago"
