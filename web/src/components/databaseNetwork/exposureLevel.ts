import type { DatabaseNetwork } from '../../types/databaseAccess'

export type ExposureLevel = 'private' | 'tunnel' | 'public'
export type BindChoice = 'private' | 'public'

const LOOPBACK = new Set(['127.0.0.1', '::1', 'localhost'])
export const LEVELS: ExposureLevel[] = ['private', 'tunnel', 'public']

// Derived only from the published port data the API already returns.
// A tunnel is a way to reach a private database, so it is never "current".
export function currentLevel(network: DatabaseNetwork): ExposureLevel {
  return network.published ? 'public' : 'private'
}

export function currentBind(network: DatabaseNetwork): BindChoice {
  const bind = network.published?.bind ?? []
  return bind.length > 0 && bind.every((b) => LOOPBACK.has(b))
    ? 'private'
    : bind.length > 0
      ? 'public'
      : 'private'
}

/** needsTypedConfirm: opening a port to the world, or publishing at all from private, makes the owner type the name. */
export function needsTypedConfirm(
  current: ExposureLevel,
  bind: BindChoice,
): boolean {
  return current !== 'public' || bind === 'public'
}

/** publishDisabledReason is non-null when the host cannot publish safely, so the UI refuses instead of hiding it. */
export function publishDisabledReason(network: DatabaseNetwork): string | null {
  if (network.rules.can_restrict) return null
  return network.rules.cannot_restrict_reason ?? ''
}
