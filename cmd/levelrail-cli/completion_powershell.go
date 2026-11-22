package main

import (
	"fmt"
	"sort"
	"strings"
)

// psQuote single-quotes s for PowerShell, where only the quote itself needs doubling.
func psQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// Native argument completer for PowerShell 5.1 and 7+, rendered from the same tables as the other shells.
func powershellCompletionScript(prog string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# PowerShell completion for %[1]s\n# Install: %[1]s completion powershell | Out-String | Invoke-Expression\n", prog)
	fmt.Fprintf(&b, "# or add that line to your $PROFILE\n\n")

	b.WriteString("$script:LrChildren = @{\n")
	for _, e := range walkCommandTree() {
		fmt.Fprintf(&b, "    %s = %s\n", psQuote(e.path), psQuote(strings.Join(e.children, " ")))
	}
	b.WriteString("}\n\n$script:LrFlags = @{\n")
	table := verbFlagTable()
	paths := make([]string, 0, len(table))
	for p := range table {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	for _, p := range paths {
		fmt.Fprintf(&b, "    %s = %s\n", psQuote(p), psQuote(strings.Join(table[p], " ")))
	}
	fmt.Fprintf(&b, "}\n\n$script:LrGlobalFlags = %s\n\n", psQuote(strings.Join(globalFlags, " ")))

	fmt.Fprintf(&b, `Register-ArgumentCompleter -Native -CommandName %[1]s -ScriptBlock {
    param($wordToComplete, $commandAst, $cursorPosition)
    $words = @($commandAst.CommandElements | ForEach-Object { $_.ToString() })
    $typed = @()
    for ($i = 1; $i -lt $words.Count; $i++) { $typed += $words[$i] }
    if ($wordToComplete -ne '' -and $typed.Count -gt 0) {
        $typed = @($typed | Select-Object -First ($typed.Count - 1))
    }
    $path = ''
    foreach ($w in $typed) {
        if ($w.StartsWith('-')) { continue }
        $known = $script:LrChildren[$path]
        if ($known -and (($known -split ' ') -contains $w)) { $path = ($path + ' ' + $w).Trim() }
    }
    $cur = $wordToComplete
    if ($cur -eq '' -and $PSVersionTable.PSVersion.Major -lt 7) { $cur = '""' }
    $names = @(& %[1]s __complete @typed $cur 2>$null | Where-Object { $_ })
    if ($names.Count -eq 0) {
        if ($wordToComplete.StartsWith('-')) {
            $pool = "$($script:LrFlags[$path]) $script:LrGlobalFlags"
        } else {
            $pool = "$($script:LrChildren[$path]) $script:LrGlobalFlags"
        }
        $names = @($pool -split ' ' | Where-Object { $_ })
    }
    $names | Where-Object { $_ -like "$wordToComplete*" } | Sort-Object -Unique | ForEach-Object {
        [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_)
    }
}
`, psQuote(prog))
	return b.String()
}
