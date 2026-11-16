export type EnvParse =
  { ok: true; env: Record<string, string> } | { ok: false; line: number }

const KEY_RE = /^[A-Za-z_][A-Za-z0-9_]*$/

// Errors carry only the line number so a pasted value is never echoed back.
export function parseEnvLines(text: string): EnvParse {
  const env: Record<string, string> = {}
  const lines = text.split(/\r?\n/)
  for (let i = 0; i < lines.length; i++) {
    const raw = (lines[i] ?? '').trim()
    if (raw === '' || raw.startsWith('#')) continue
    const eq = raw.indexOf('=')
    const key = eq > 0 ? raw.slice(0, eq).trim() : ''
    if (!KEY_RE.test(key)) return { ok: false, line: i + 1 }
    env[key] = raw.slice(eq + 1)
  }
  return { ok: true, env }
}
