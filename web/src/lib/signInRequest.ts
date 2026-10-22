export type DeviceKind = 'browser' | 'cli' | 'mcp' | 'unknown'

export interface DeviceInfo {
  kind: DeviceKind
  label: string
  browser: string
  os: string
}

const CLI_LABEL = 'Command line client'
const MCP_LABEL = 'MCP client'
const UNKNOWN_BROWSER = 'Unknown browser'

// The server already reduces the user agent to "Browser on OS" or a client label.
export function parseDeviceLabel(label: string): DeviceInfo {
  const trimmed = label.trim()
  if (trimmed === CLI_LABEL) {
    return { kind: 'cli', label: trimmed, browser: '', os: '' }
  }
  if (trimmed === MCP_LABEL) {
    return { kind: 'mcp', label: trimmed, browser: '', os: '' }
  }
  const [browser = '', os = ''] = trimmed.split(' on ')
  if (browser === '' || browser === UNKNOWN_BROWSER) {
    return { kind: 'unknown', label: trimmed, browser: '', os }
  }
  return { kind: 'browser', label: trimmed, browser, os }
}

export type IpScope = 'this-computer' | 'local-network' | 'public'

export function ipScope(ip: string): IpScope {
  const v = ip.trim().toLowerCase()
  if (v === '::1' || v === 'localhost' || /^127\./.test(v)) {
    return 'this-computer'
  }
  const m = /^(\d{1,3})\.(\d{1,3})\.\d{1,3}\.\d{1,3}$/.exec(v)
  if (m) {
    const a = Number(m[1])
    const b = Number(m[2])
    if (
      a === 10 ||
      (a === 172 && b >= 16 && b <= 31) ||
      (a === 192 && b === 168) ||
      (a === 169 && b === 254)
    ) {
      return 'local-network'
    }
    return 'public'
  }
  if (/^f[cd][0-9a-f]{2}:/.test(v) || /^fe[89ab][0-9a-f]:/.test(v)) {
    return 'local-network'
  }
  return 'public'
}

export function secondsLeft(expiresAt: string, now: number): number {
  const end = new Date(expiresAt).getTime()
  if (Number.isNaN(end)) return 0
  return Math.max(0, Math.ceil((end - now) / 1000))
}

export function formatCountdown(seconds: number): string {
  const m = Math.floor(seconds / 60)
  const s = seconds % 60
  return `${m}:${String(s).padStart(2, '0')}`
}

export const MATCH_PATTERN = /^[1-9]\d$/

export function isValidMatch(value: string): boolean {
  return MATCH_PATTERN.test(value)
}

export function sanitizeDigits(raw: string): string {
  return raw.replace(/\D/g, '').slice(0, 2)
}
