package pipeline

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// scriptExprEnvPrefix names the env vars InterpolateScript substitutes for
// untrusted expressions.
const scriptExprEnvPrefix = "PIPELINE_EXPR_"

var nonEnvChars = regexp.MustCompile(`[^A-Za-z0-9]+`)

// literalScriptVar reports whether a ${{ }} path holds a value that comes
// from the pipeline file or a validated identifier, so splicing it into
// script text gives an attacker nothing. Everything else (ref, branch, tag,
// actor, inputs, needs outputs) can carry attacker-chosen text from a
// webhook, PR or earlier step.
func literalScriptVar(path string) bool {
	switch path {
	case "app", "trigger", "sha", "pipeline", "job", "run.number", "run.id":
		return true
	}
	for _, p := range []string{"matrix.", "env.", "secrets."} {
		if strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

// InterpolateScript is Interpolate for shell script text. Values that
// could be attacker-controlled are not spliced into the script: each is
// replaced by a ${PIPELINE_EXPR_*} reference and returned in env, which the
// caller exports single-quoted, so the shell never parses them as code.
func InterpolateScript(s string, sc Scope) (body string, env map[string]string, err error) {
	env = map[string]string{}
	names := map[string]string{}
	used := map[string]bool{}
	var firstErr error
	body = exprRe.ReplaceAllStringFunc(s, func(m string) string {
		path := strings.TrimSpace(exprRe.FindStringSubmatch(m)[1])
		v, ok := sc.Vars[path]
		if !ok && strings.HasPrefix(path, "secrets.") && firstErr == nil {
			firstErr = fmt.Errorf("secret %q is not set", strings.TrimPrefix(path, "secrets."))
		}
		if literalScriptVar(path) {
			return v
		}
		name, seen := names[path]
		if !seen {
			base := scriptExprEnvPrefix + strings.ToUpper(strings.Trim(nonEnvChars.ReplaceAllString(path, "_"), "_"))
			name = base
			for i := 2; used[name]; i++ {
				name = base + "_" + strconv.Itoa(i)
			}
			used[name] = true
			names[path] = name
			env[name] = v
		}
		return "${" + name + "}"
	})
	return body, env, firstErr
}
