export interface OciConfig {
  tenancy: string
  user: string
  fingerprint: string
  region: string
  keyFile: string
}

const EMPTY: OciConfig = {
  tenancy: '',
  user: '',
  fingerprint: '',
  region: '',
  keyFile: '',
}

function unquote(v: string): string {
  const s = v.trim()
  if (s.length >= 2) {
    const q = s[0]
    if ((q === '"' || q === "'") && s.endsWith(q)) return s.slice(1, -1).trim()
  }
  return s
}

// Pure and client-side only: the pasted text of ~/.oci/config never leaves
// the browser. DEFAULT wins when present, else the first profile is used.
export function parseOciConfig(text: string): OciConfig {
  const profiles = new Map<string, Record<string, string>>()
  let current: Record<string, string> | null = null
  for (const raw of text.split(/\r?\n/)) {
    const line = raw.trim()
    if (line === '' || line.startsWith('#') || line.startsWith(';')) continue
    const head = /^\[([^\]]*)\]$/.exec(line)
    if (head) {
      const name = (head[1] ?? '').trim()
      current = profiles.get(name) ?? {}
      profiles.set(name, current)
      continue
    }
    const eq = line.indexOf('=')
    if (eq < 1) continue
    // Keys outside any header are treated as an implicit DEFAULT block.
    if (!current) {
      current = profiles.get('DEFAULT') ?? {}
      profiles.set('DEFAULT', current)
    }
    current[line.slice(0, eq).trim().toLowerCase()] = unquote(
      line.slice(eq + 1),
    )
  }
  const first = profiles.values().next().value
  const block = profiles.get('DEFAULT') ?? first
  if (!block) return { ...EMPTY }
  return {
    tenancy: block.tenancy ?? '',
    user: block.user ?? '',
    fingerprint: block.fingerprint ?? '',
    region: block.region ?? '',
    keyFile: block.key_file ?? '',
  }
}

// Shortens an OCID to its type prefix and last characters for a summary line.
export function shortOcid(ocid: string): string {
  const v = ocid.trim()
  return v.length <= 14 ? v : `ocid1...${v.slice(-6)}`
}

export const OCI_RE = {
  tenancy: /^ocid1\.tenancy\.[\w.-]+$/,
  user: /^ocid1\.user\.[\w.-]+$/,
  compartment: /^ocid1\.(compartment|tenancy)\.[\w.-]+$/,
  subnet: /^ocid1\.subnet\.[\w.-]+$/,
  fingerprint: /^([0-9a-fA-F]{2}:){15}[0-9a-fA-F]{2}$/,
  region: /^[a-z]{2,}-[a-z0-9-]+-\d+$/,
  key: /-----BEGIN [A-Z ]*PRIVATE KEY-----/,
} as const
