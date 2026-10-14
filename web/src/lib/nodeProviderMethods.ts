import type { NodeProviderMethod } from '../types/nodeProvision'

// Short URL spellings accepted in ?method= so a deep link stays readable.
const ALIASES: Record<string, NodeProviderMethod> = {
  token: 'api_token',
  key: 'access_key',
  role: 'assume_role',
  identity: 'ambient',
  federation: 'workload_identity_federation',
  wif: 'workload_identity_federation',
  json: 'service_account_json',
}

const ALL: readonly NodeProviderMethod[] = [
  'api_token',
  'access_key',
  'assume_role',
  'ambient',
  'workload_identity_federation',
  'service_account_json',
  'manual',
]

// Methods shown when the API did not send a list (older control plane).
const FALLBACK: Record<string, NodeProviderMethod[]> = {
  hetzner: ['api_token'],
  digitalocean: ['api_token'],
  aws: ['access_key', 'assume_role', 'ambient'],
  azure: ['service_account_json', 'workload_identity_federation'],
  gcp: ['service_account_json'],
  custom: ['manual'],
}

export function supportedMethods(
  provider: string,
  fromApi: NodeProviderMethod[] | undefined,
): NodeProviderMethod[] {
  if (fromApi && fromApi.length > 0) return fromApi
  return FALLBACK[provider] ?? ['api_token']
}

// Picks the method for the dialog: the URL value (or its alias) when the
// provider supports it, else the stored one, else the first supported.
export function resolveMethod(
  raw: string | undefined,
  supported: NodeProviderMethod[],
  stored?: NodeProviderMethod,
): NodeProviderMethod {
  const candidate = raw ? (ALIASES[raw] ?? raw) : undefined
  const match = ALL.find((m) => m === candidate)
  if (match && supported.includes(match)) return match
  if (stored && supported.includes(stored)) return stored
  return supported[0] ?? 'api_token'
}
