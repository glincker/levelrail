package main

import (
	"fmt"
	"strings"
)

// bashCompletionScript renders a bash completion script for prog that
// walks COMP_WORDS through cliCommandTree to offer the right verbs at
// whatever depth the cursor is at, falling back to globalFlags once the
// tree bottoms out (or a flag has already been typed).
func bashCompletionScript(prog string) string {
	ident := sanitizeIdent(prog)
	var b strings.Builder

	fmt.Fprintf(&b, "# bash completion for %[1]s\n# Install: source <(%[1]s completion bash)\n", prog)
	fmt.Fprintf(&b, "# or:      %[1]s completion bash | sudo tee /etc/bash_completion.d/%[1]s > /dev/null\n\n", prog)

	b.WriteString(renderChildrenFunc(fmt.Sprintf("_%s_children", ident)))
	b.WriteString("\n")

	fmt.Fprintf(&b, `_%[1]s_complete() {
  local cur words cword path i w children opts
  COMPREPLY=()
  cur="${COMP_WORDS[COMP_CWORD]}"
  words=("${COMP_WORDS[@]}")
  cword=$COMP_CWORD
  path=""
  i=1
  while [ "$i" -lt "$cword" ]; do
    w="${words[$i]}"
    case "$w" in
      -*) ;;
      *)
        children="$(_%[1]s_children "$path")"
        case " $children " in
          *" $w "*) path="${path:+$path }$w" ;;
        esac
        ;;
    esac
    i=$((i + 1))
  done
  children="$(_%[1]s_children "$path")"
  opts="$children %[2]s"
  COMPREPLY=( $(compgen -W "$opts" -- "$cur") )
  return 0
}
complete -F _%[1]s_complete %[3]s
`, ident, strings.Join(globalFlags, " "), prog)

	return b.String()
}
