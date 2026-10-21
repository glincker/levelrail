import type { NetworkShareProtocol } from '../../types/networkShare'

export interface ShareValues {
  protocol: NetworkShareProtocol
  name: string
  host: string
  path: string
  username: string
  password: string
  options: string
}

export type ShareErrorKey =
  | 'nameRequired'
  | 'nameFormat'
  | 'nameTaken'
  | 'hostRequired'
  | 'hostFormat'
  | 'pathRequired'
  | 'nfsPathFormat'
  | 'smbPathFormat'
  | 'usernameRequired'
  | 'passwordRequired'

export type ShareErrors = Partial<Record<keyof ShareValues, ShareErrorKey>>

const NAME_PATTERN = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/
const HOSTNAME_LABEL = /^[a-z0-9]([a-z0-9-]*[a-z0-9])?$/i
const IPV4_SHAPE = /^\d{1,3}(\.\d{1,3}){3}$/
const HEX_GROUP = /^[0-9a-f]{1,4}$/i
const HOST_MAX_LENGTH = 253
const IPV6_MAX_GROUPS = 8
const OPTION_SEPARATOR = ','

function isIPv4(host: string): boolean {
  return (
    IPV4_SHAPE.test(host) &&
    host.split('.').every((part) => Number(part) <= 255)
  )
}

function isIPv6(raw: string): boolean {
  const host = raw.replace(/^\[|\]$/g, '')
  if (!host.includes(':')) return false
  const halves = host.split('::')
  if (halves.length > 2) return false
  const groups = halves.flatMap((half) => (half === '' ? [] : half.split(':')))
  const last = groups[groups.length - 1]
  const tail = last !== undefined && last.includes('.') ? last : null
  const hexGroups = tail === null ? groups : groups.slice(0, -1)
  if (!hexGroups.every((g) => HEX_GROUP.test(g))) return false
  if (tail !== null && !isIPv4(tail)) return false
  const count = hexGroups.length + (tail === null ? 0 : 2)
  return halves.length === 2
    ? count < IPV6_MAX_GROUPS
    : count === IPV6_MAX_GROUPS
}

export function isValidHost(raw: string): boolean {
  const host = raw.trim()
  if (host === '' || host.length > HOST_MAX_LENGTH) return false
  if (isIPv6(host)) return true
  if (/^[\d.]+$/.test(host)) return isIPv4(host)
  return host.split('.').every((label) => HOSTNAME_LABEL.test(label))
}

// SMB devices are built as "//host" + remote_path, so the stored path keeps its slash.
export function normalizePath(
  protocol: NetworkShareProtocol,
  raw: string,
): string {
  const path = raw.trim()
  if (protocol === 'nfs') return path
  const share = path.replace(/^\/+/, '')
  return share === '' ? '' : `/${share}`
}

function isValidPath(protocol: NetworkShareProtocol, raw: string): boolean {
  const path = raw.trim()
  if (/\s/.test(path)) return false
  if (protocol === 'nfs') return path.startsWith('/')
  const share = path.replace(/^\/+/, '')
  return share !== '' && !share.includes('/') && !share.includes('\\')
}

export function kebab(input: string): string {
  return input
    .toLowerCase()
    .replace(/[^a-z0-9]+/g, '-')
    .replace(/^-+|-+$/g, '')
}

export function suggestName(host: string, path: string): string {
  const trimmedHost = host.trim()
  const hostPart = isIPv4(trimmedHost)
    ? trimmedHost
    : (trimmedHost.split('.')[0] ?? '')
  const segments = path.split(/[\\/]/).filter((s) => s.trim() !== '')
  const last = segments[segments.length - 1] ?? ''
  return [kebab(last), kebab(hostPart)].filter(Boolean).join('-')
}

function parts(options: string): string[] {
  return options
    .split(OPTION_SEPARATOR)
    .map((o) => o.trim())
    .filter(Boolean)
}

export function hasOption(options: string, token: string): boolean {
  const have = parts(options)
  return token.split(OPTION_SEPARATOR).every((p) => have.includes(p))
}

// NFS protocol versions are mutually exclusive, so picking one drops the other.
export function toggleOption(options: string, token: string): string {
  const wanted = token.split(OPTION_SEPARATOR)
  let next = parts(options)
  if (hasOption(options, token)) {
    next = next.filter((p) => !wanted.includes(p))
  } else {
    if (token.startsWith('nfsvers=')) {
      next = next.filter((p) => !p.startsWith('nfsvers='))
    }
    next = [...next, ...wanted.filter((p) => !next.includes(p))]
  }
  return next.join(OPTION_SEPARATOR)
}

export function remoteAddress(
  protocol: NetworkShareProtocol,
  host: string,
  path: string,
): string {
  const h = host.trim()
  const p = normalizePath(protocol, path)
  return protocol === 'nfs' ? `${h}:${p}` : `//${h}${p}`
}

export function isReadOnly(options: string): boolean {
  return parts(options).includes('ro')
}

export function validate(
  v: ShareValues,
  takenNames: readonly string[],
): ShareErrors {
  const errors: ShareErrors = {}
  const name = v.name.trim()
  if (name === '') errors.name = 'nameRequired'
  else if (!NAME_PATTERN.test(name)) errors.name = 'nameFormat'
  else if (takenNames.some((n) => n.toLowerCase() === name)) {
    errors.name = 'nameTaken'
  }
  if (v.host.trim() === '') errors.host = 'hostRequired'
  else if (!isValidHost(v.host)) errors.host = 'hostFormat'
  if (v.path.trim() === '') errors.path = 'pathRequired'
  else if (!isValidPath(v.protocol, v.path)) {
    errors.path = v.protocol === 'nfs' ? 'nfsPathFormat' : 'smbPathFormat'
  }
  if (v.protocol === 'cifs') {
    if (v.username.trim() === '') errors.username = 'usernameRequired'
    if (v.password === '') errors.password = 'passwordRequired'
  }
  return errors
}
