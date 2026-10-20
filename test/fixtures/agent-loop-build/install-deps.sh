#!/bin/sh
# Resolves name==version pins against ./index, a stand-in package index.
set -eu
mkdir -p www
while IFS= read -r line; do
	[ -z "$line" ] && continue
	name="${line%%==*}"
	version="${line##*==}"
	if [ ! -f "index/$name/$version" ]; then
		available="$(ls "index/$name" 2>/dev/null | tr '\n' ' ' | sed 's/ $//')"
		echo "ERROR: No matching distribution found for $name==$version (available: $available)"
		exit 1
	fi
	cat "index/$name/$version" >>www/index.html
done <requirements.txt
