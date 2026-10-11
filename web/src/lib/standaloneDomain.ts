import type { Domain } from '../queries/domains'
import type {
  StandaloneDomain,
  StandaloneDomainInput,
} from '../queries/standaloneDomains'

// The {name} the per-domain endpoints take for a standalone domain.
export const STANDALONE_SCOPE = '_standalone'

export function isStandaloneDomain(d: Pick<Domain, 'kind'>): boolean {
  return d.kind === 'standalone'
}

// domainScope is the app segment for per-domain API calls and links.
export function domainScope(d: Pick<Domain, 'kind' | 'service_name'>): string {
  return isStandaloneDomain(d) ? STANDALONE_SCOPE : d.service_name
}

export type TargetMode = 'port' | 'address' | 'url' | 'app' | 'container'

export interface TargetDraft {
  mode: TargetMode
  port: string
  address: string
  url: string
  app: string
  appPort: string
  container: string
  containerPort: string
}

export const EMPTY_TARGET: TargetDraft = {
  mode: 'port',
  port: '',
  address: '',
  url: '',
  app: '',
  appPort: '',
  container: '',
  containerPort: '',
}

// targetString builds the canonical target the API accepts, '' when incomplete.
export function targetString(d: TargetDraft): string {
  switch (d.mode) {
    case 'port':
      return d.port.trim() ? `127.0.0.1:${d.port.trim()}` : ''
    case 'address':
      return d.address.trim()
    case 'url':
      return d.url.trim()
    case 'app':
      if (!d.app) return ''
      return d.appPort.trim()
        ? `app:${d.app}:${d.appPort.trim()}`
        : `app:${d.app}`
    case 'container':
      return d.container && d.containerPort.trim()
        ? `container:${d.container}:${d.containerPort.trim()}`
        : ''
  }
}

// draftFromTarget is targetString's inverse, for editing a saved target.
export function draftFromTarget(target: string): TargetDraft {
  const d = { ...EMPTY_TARGET }
  if (/^https?:\/\//i.test(target)) return { ...d, mode: 'url', url: target }
  if (target.startsWith('app:')) {
    const [, app = '', port = ''] = target.split(':')
    return { ...d, mode: 'app', app, appPort: port }
  }
  if (target.startsWith('container:')) {
    const [, container = '', port = ''] = target.split(':')
    return { ...d, mode: 'container', container, containerPort: port }
  }
  const local = /^127\.0\.0\.1:(\d+)$/.exec(target)
  if (local?.[1]) return { ...d, mode: 'port', port: local[1] }
  return { ...d, mode: 'address', address: target }
}

export interface AdvancedDraft {
  upstreamTls: boolean
  verifyTls: boolean
  serverName: string
  hostMode: 'default' | 'preserve' | 'upstream' | 'custom'
  hostCustom: string
  websocket: boolean
  connectTimeout: string
  readTimeout: string
  stripPrefix: string
}

export const DEFAULT_ADVANCED: AdvancedDraft = {
  upstreamTls: false,
  verifyTls: true,
  serverName: '',
  hostMode: 'default',
  hostCustom: '',
  websocket: true,
  connectTimeout: '',
  readTimeout: '',
  stripPrefix: '',
}

export function hostHeaderValue(a: AdvancedDraft): string {
  if (a.hostMode === 'custom') return a.hostCustom.trim()
  return a.hostMode === 'default' ? '' : a.hostMode
}

export function advancedFrom(d: StandaloneDomain): AdvancedDraft {
  const known = d.host_header === 'preserve' || d.host_header === 'upstream'
  return {
    upstreamTls: d.upstream_tls,
    verifyTls: !d.tls_skip_verify,
    serverName: d.tls_server_name ?? '',
    hostMode: known
      ? (d.host_header as 'preserve' | 'upstream')
      : d.host_header
        ? 'custom'
        : 'default',
    hostCustom: known ? '' : d.host_header,
    websocket: d.websocket,
    connectTimeout: d.dial_timeout_seconds
      ? String(d.dial_timeout_seconds)
      : '',
    readTimeout: d.read_timeout_seconds ? String(d.read_timeout_seconds) : '',
    stripPrefix: d.strip_prefix ?? '',
  }
}

export function buildStandaloneInput(
  domain: string,
  draft: TargetDraft,
  adv: AdvancedDraft,
): StandaloneDomainInput {
  const urlTarget = draft.mode === 'url'
  const tlsOn = urlTarget
    ? draft.url.toLowerCase().startsWith('https://')
    : adv.upstreamTls
  return {
    domain: domain.trim().toLowerCase(),
    target: targetString(draft),
    upstream_tls: urlTarget ? undefined : adv.upstreamTls,
    tls_skip_verify: tlsOn ? !adv.verifyTls : false,
    tls_server_name: tlsOn ? adv.serverName : '',
    host_header: hostHeaderValue(adv),
    websocket: adv.websocket,
    dial_timeout_seconds: Number(adv.connectTimeout || 0),
    read_timeout_seconds: Number(adv.readTimeout || 0),
    strip_prefix: adv.stripPrefix,
  }
}
