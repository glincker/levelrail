package main

import (
	"fmt"
	"strings"
)

// fishCompletionScript renders a fish completion script for prog. Unlike
// bash/zsh it needs only two "complete" registrations total (not one per
// tree node), since fish's -a accepts a command substitution that is
// re-evaluated on every completion request.
func fishCompletionScript(prog string) string {
	ident := sanitizeIdent(prog)
	var b strings.Builder

	fmt.Fprintf(&b, "# fish completion for %[1]s\n# Install: %[1]s completion fish | source\n", prog)
	fmt.Fprintf(&b, "# or:      %[1]s completion fish > ~/.config/fish/completions/%[1]s.fish\n\n", prog)

	fmt.Fprintf(&b, "function __%s_children\n    switch \"$argv[1]\"\n", ident)
	for _, e := range walkCommandTree() {
		fmt.Fprintf(&b, "        case %s\n            echo %q\n", fishCasePattern(e.path), strings.Join(e.children, " "))
	}
	b.WriteString("        case '*'\n            echo \"\"\n    end\nend\n\n")

	fmt.Fprintf(&b, `function __%[1]s_path
    set -l cmd (commandline -opc)
    set -l path ""
    for i in (seq 2 (count $cmd))
        set -l w $cmd[$i]
        if not string match -q -- "-*" $w
            set -l children (__%[1]s_children $path)
            if contains -- $w $children
                set path (string trim -- "$path $w")
            end
        end
    end
    echo $path
end

complete -c %[2]s -f -a '(__%[1]s_children (__%[1]s_path))'
complete -c %[2]s -f -a %[3]q -d "global flags"
`, ident, prog, strings.Join(globalFlags, " "))

	return b.String()
}

// fishCasePattern quotes path for a fish "switch" case clause; path is
// always plain space-joined command words with no characters that need
// escaping, so single-quoting (or ” for the empty root path) is enough.
func fishCasePattern(path string) string {
	if path == "" {
		return "''"
	}
	return "'" + path + "'"
}
