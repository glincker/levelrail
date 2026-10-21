import type {
  ProxyProviderKind,
  ProxySettings,
} from '../queries/proxyIntegration'

export interface ManualFields {
  container: string
  dynamic_dir: string
  entrypoint_http: string
  entrypoint_https: string
  cert_resolver: string
  upstream_host: string
}

export type ManualFieldName = keyof ManualFields
export type ManualErrorKey = 'name' | 'absolute' | 'path' | 'host'
export type ManualErrors = Partial<Record<ManualFieldName, ManualErrorKey>>

const DEFAULTS: Record<'traefik' | 'nginx', ManualFields> = {
  traefik: {
    container: 'traefik',
    dynamic_dir: '/etc/traefik/dynamic',
    entrypoint_http: 'web',
    entrypoint_https: 'websecure',
    cert_resolver: 'letsencrypt',
    upstream_host: 'host.docker.internal',
  },
  nginx: {
    container: 'nginx',
    dynamic_dir: '/etc/nginx/conf.d',
    entrypoint_http: '',
    entrypoint_https: '',
    cert_resolver: '',
    upstream_host: 'host.docker.internal',
  },
}

const NAME = /^[A-Za-z0-9][A-Za-z0-9_.-]*$/
const HOST = /^[A-Za-z0-9][A-Za-z0-9.:-]*$/

export function manualDefaults(kind: ProxyProviderKind): ManualFields {
  return { ...(kind === 'nginx' ? DEFAULTS.nginx : DEFAULTS.traefik) }
}

// Error keys are i18n ids under proxyMissing.manual.errors.
export function validateManual(
  kind: ProxyProviderKind,
  f: ManualFields,
): ManualErrors {
  const errors: ManualErrors = {}
  if (!NAME.test(f.container.trim())) errors.container = 'name'
  const dir = f.dynamic_dir.trim()
  if (!dir.startsWith('/')) errors.dynamic_dir = 'absolute'
  else if (dir.includes('..') || /\s/.test(dir)) errors.dynamic_dir = 'path'
  if (kind === 'traefik') {
    if (!NAME.test(f.entrypoint_http.trim())) errors.entrypoint_http = 'name'
    if (!NAME.test(f.entrypoint_https.trim())) errors.entrypoint_https = 'name'
    if (f.cert_resolver.trim() && !NAME.test(f.cert_resolver.trim())) {
      errors.cert_resolver = 'name'
    }
  }
  if (!HOST.test(f.upstream_host.trim())) errors.upstream_host = 'host'
  return errors
}

export function toSettings(
  kind: ProxyProviderKind,
  f: ManualFields,
  base: ProxySettings,
): ProxySettings {
  return {
    ...base,
    integration: kind === 'nginx' ? 'nginx_file' : 'traefik_file',
    container: f.container.trim(),
    dynamic_dir: f.dynamic_dir.trim(),
    entrypoint_http: f.entrypoint_http.trim(),
    entrypoint_https: f.entrypoint_https.trim(),
    cert_resolver: f.cert_resolver.trim(),
    upstream_host: f.upstream_host.trim(),
  }
}

export function isAbsoluteDir(dir: string): boolean {
  return dir.trim().startsWith('/')
}
