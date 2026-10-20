#!/usr/bin/env bash
# Prints the estimated tools/list token cost per MCP mode and fails when a
# mode is over its budget. Override with APP_MCP_TOKEN_BUDGET_<MODE>.
set -euo pipefail
cd "$(dirname "$0")/.."
go test -count=1 -v -run 'TestToolListTokenBudget|TestToolListReport|TestAgentCoreBudget' ./internal/mcptools |
  grep -E 'budget_test.go|^(---|ok|FAIL|PASS)|^ +[a-z_]+ +total'
