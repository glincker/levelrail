// Parses a single pasted `.env`-format block into key/value pairs. Scope is
// deliberately limited to what Coolify/Dokploy-style paste-import needs:
// blank lines and `#` comments are skipped, a leading `export ` (as seen in
// .env files copied from shell scripts) is stripped, and a value wrapped in
// matching quotes has them removed. Matching-only quote stripping means a
// stray unmatched quote (e.g. a value that is just `'`) is left as-is rather
// than mangled. Multi-line values and shell variable expansion are out of
// scope on purpose, this is a paste convenience, not a full dotenv parser.
//
// Shared by EnvEditor (the "Paste .env" dialog) and EnvDevView (the
// developer-view textarea), split out of EnvEditor's own file since a
// component file can only export components under this project's Fast
// Refresh lint rule.
export function parseEnvBlock(text: string): { key: string; value: string }[] {
  const results: { key: string; value: string }[] = []
  for (const rawLine of text.split(/\r?\n/)) {
    const line = rawLine.trim()
    if (!line || line.startsWith('#')) continue

    const withoutExport = line.startsWith('export ')
      ? line.slice('export '.length).trim()
      : line

    const eqIndex = withoutExport.indexOf('=')
    if (eqIndex === -1) continue

    const key = withoutExport.slice(0, eqIndex).trim()
    if (!key) continue

    let value = withoutExport.slice(eqIndex + 1).trim()
    const isDoubleQuoted = value.startsWith('"') && value.endsWith('"')
    const isSingleQuoted = value.startsWith("'") && value.endsWith("'")
    if (value.length >= 2 && (isDoubleQuoted || isSingleQuoted)) {
      value = value.slice(1, -1)
    }

    results.push({ key, value })
  }
  return results
}
