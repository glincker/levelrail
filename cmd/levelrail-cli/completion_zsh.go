package main

import (
	"fmt"
	"strings"
)

// zshCompletionScript renders a zsh completion script for prog. It uses
// the compsys "words"/"CURRENT" state directly rather than the
// _arguments state-machine helpers, walking the same flattened
// cliCommandTree renderChildrenFunc already emits for bash.
func zshCompletionScript(prog string) string {
	ident := sanitizeIdent(prog)
	var b strings.Builder

	fmt.Fprintf(&b, "#compdef %[1]s\n# zsh completion for %[1]s\n# Install: source <(%[1]s completion zsh)\n", prog)
	fmt.Fprintf(&b, "# or place the output in a file named _%[1]s somewhere on $fpath\n\n", prog)

	b.WriteString(renderChildrenFunc(fmt.Sprintf("_%s_children", ident)))
	b.WriteString("\n")

	fmt.Fprintf(&b, `_%[1]s() {
  local path w children
  local -a opts
  path=""
  local i
  for (( i = 2; i < CURRENT; i++ )); do
    w="${words[i]}"
    case "$w" in
      -*) ;;
      *)
        children="$(_%[1]s_children "$path")"
        case " $children " in
          *" $w "*) path="${path:+$path }$w" ;;
        esac
        ;;
    esac
  done
  children="$(_%[1]s_children "$path")"
  opts=(${=children} %[2]s)
  _describe 'command' opts
}

compdef _%[1]s %[3]s
`, ident, strings.Join(globalFlags, " "), prog)

	return b.String()
}
