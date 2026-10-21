/** Parses "KEY=VALUE" lines. Returns the first bad line when one is malformed. */
export function parseKeyValueLines(
  text: string,
): { ok: true; values: Record<string, string> } | { ok: false; line: string } {
  const values: Record<string, string> = {}
  for (const raw of text.split('\n')) {
    const line = raw.trim()
    if (line === '') continue
    const eq = line.indexOf('=')
    if (eq <= 0) return { ok: false, line }
    values[line.slice(0, eq).trim()] = line.slice(eq + 1)
  }
  return { ok: true, values }
}

export function formatKeyValueLines(values: Record<string, string>): string {
  return Object.entries(values)
    .map(([k, v]) => `${k}=${v}`)
    .join('\n')
}

export const BRANCH_CONTEXT_PREFIX = 'branch:'
