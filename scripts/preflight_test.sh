#!/usr/bin/env bash
# Checks the path-to-guard mapping in scripts/preflight.sh with fixture file lists.
# Usage: scripts/preflight_test.sh
set -uo pipefail
cd "$(dirname "$0")/.." || exit 2

fail=0
# expect "<files, space separated>" "<guards that must be selected>" "<guards that must not>"
expect() {
	local files="$1" want="$2" nowant="$3" got g
	# shellcheck disable=SC2086 # files is a space separated fixture list
	got="$(printf '%s\n' $files | scripts/preflight.sh --stdin --plan | tr '\n' ' ')"
	for g in $want; do
		case " $got" in *" $g "*) ;; *) echo "FAIL: [$files] missing $g (got: $got)"; fail=1 ;; esac
	done
	for g in $nowant; do
		case " $got" in *" $g "*) echo "FAIL: [$files] unexpected $g (got: $got)"; fail=1 ;; esac
	done
}

expect "cmd/levelrail-cli/ddns.go" "dashes clitree fanin execgate" "docs locales catalog"
expect "internal/clitree/tree.go" "clitree" "catalog"
expect "internal/catalog/templates/x.go" "catalog logos validate-templates codegen" "clitree locales"
expect "web/src/lib/templateLogos.ts" "catalog logos" "fanin execgate"
expect "internal/runtimesettings/registry.go" "runtimesettings codegen fanin" "docs"
expect "internal/api/foo_routes.go" "codegen fanin" "clitree"
expect "web/src/locales/en/common.json" "locales" "fanin codegen"
expect "web/src/i18n/index.ts" "locales" "docs"
expect "docs/self-host/seaweedfs.mdx" "docs codegen" "fanin locales"
expect "scripts/newtool/main.go" "fanin execgate" "docs"
expect "README.txt" "dashes" "docs codegen fanin execgate clitree"

[ "$fail" -eq 0 ] && echo "preflight_test: ok"
exit "$fail"
