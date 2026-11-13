// Redis-compatible engines whose create flow takes TLS and password options.
const CACHE_ENGINES = new Set(['redis', 'valkey', 'keydb'])

export type CacheTlsMode = 'required' | 'optional' | 'off'
export type CacheAuthMode = 'auto' | 'none'

export const CACHE_TLS_MODES: readonly CacheTlsMode[] = [
  'required',
  'optional',
  'off',
]
export const CACHE_AUTH_MODES: readonly CacheAuthMode[] = ['auto', 'none']
export const DEFAULT_CACHE_TLS: CacheTlsMode = 'required'
export const DEFAULT_CACHE_AUTH: CacheAuthMode = 'auto'

// The TLS listener sits beside the plain port in the server image.
export const CACHE_TLS_PORT = 6380

export function isCacheEngine(engine: string): boolean {
  return CACHE_ENGINES.has(engine)
}

/** Create-request fields for the engine, empty for engines without them. */
export function cacheCreateOptions(
  engine: string,
  tls: string | undefined,
  auth: string | undefined,
): { tls?: CacheTlsMode; auth?: CacheAuthMode } {
  if (!isCacheEngine(engine)) return {}
  return {
    tls: CACHE_TLS_MODES.find((m) => m === tls) ?? DEFAULT_CACHE_TLS,
    auth: CACHE_AUTH_MODES.find((m) => m === auth) ?? DEFAULT_CACHE_AUTH,
  }
}
