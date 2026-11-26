import type {
  ExternalDatabaseRequest,
  ExternalEngine,
  ExternalParsedConnection,
  ExternalProbeResult,
  ExternalProvider,
  ExternalTlsMode,
} from '../types/externalDatabase'
import {
  DEFAULT_PORT,
  ENGINES,
  defaultTls,
  slug,
} from '../components/connectDatabaseForm'

export const GENERIC_PROVIDER = 'generic'

export interface HostedForm {
  name: string
  engine: ExternalEngine
  host: string
  port: string
  username: string
  password: string
  database: string
  authDatabase: string
  tls: ExternalTlsMode
}

export const EMPTY_HOSTED_FORM: HostedForm = {
  name: '',
  engine: 'postgres',
  host: '',
  port: String(DEFAULT_PORT.postgres),
  username: '',
  password: '',
  database: '',
  authDatabase: '',
  tls: 'prefer',
}

// Engines a provider offers; the generic choice offers every engine.
export function providerEngines(
  provider: ExternalProvider | null,
): ExternalEngine[] {
  return provider && provider.engines.length > 0 ? provider.engines : ENGINES
}

// Prefills engine, port and TLS mode from the preset so the form starts valid.
export function formFromProvider(
  provider: ExternalProvider | null,
): HostedForm {
  const engine = providerEngines(provider)[0] ?? 'postgres'
  return {
    ...EMPTY_HOSTED_FORM,
    engine,
    port: String(provider?.port || DEFAULT_PORT[engine]),
    tls: provider?.tls_mode ?? defaultTls(engine),
  }
}

// Mirrors the server's suggestion so a typed host also gets a name.
export function suggestHostedName(providerId: string, host: string): string {
  const label = host.toLowerCase().split('.')[0] ?? ''
  const base =
    providerId && providerId !== GENERIC_PROVIDER
      ? `${providerId}-${label}`
      : label
  return slug(base)
}

export function applyParsed(
  form: HostedForm,
  parsed: ExternalParsedConnection,
  keepName: boolean,
): HostedForm {
  return {
    ...form,
    name: keepName && form.name ? form.name : parsed.suggested_name,
    engine: parsed.engine,
    host: parsed.host,
    port: String(parsed.port || DEFAULT_PORT[parsed.engine]),
    username: parsed.username ?? '',
    database: parsed.database ?? '',
    authDatabase: parsed.auth_database ?? '',
    tls: parsed.tls_mode,
  }
}

// The connection string rides along so the server fills the password from it;
// typed fields win over the string.
export function hostedRequest(
  form: HostedForm,
  providerId: string,
  url: string,
  saveAnyway: boolean,
): ExternalDatabaseRequest {
  return {
    name: form.name,
    engine: form.engine,
    host: form.host,
    port: Number(form.port) || undefined,
    username: form.username || undefined,
    password: form.password || undefined,
    database: form.database || undefined,
    auth_database: form.authDatabase || undefined,
    tls_mode: form.tls,
    provider: providerId,
    url: url.trim() || undefined,
    save_anyway: saveAnyway || undefined,
  }
}

export function probeReached(probe: ExternalProbeResult): boolean {
  return probe.status === 'reachable' || probe.status === 'slow'
}

export type TlsOutcome = 'verified' | 'unverified' | 'notChecked' | 'off'

// The probe carries no certificate flag, so this reads the mode the probe
// ran with: a verify mode that reached the server proves the certificate.
export function tlsOutcome(
  mode: ExternalTlsMode,
  probe: ExternalProbeResult,
): TlsOutcome {
  if (mode === 'disable') return 'off'
  if (mode === 'verify-ca' || mode === 'verify-full') {
    return probeReached(probe) ? 'verified' : 'unverified'
  }
  return mode === 'require' ? 'unverified' : 'notChecked'
}
