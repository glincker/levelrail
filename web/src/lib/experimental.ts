import { redirect } from '@tanstack/react-router'
import type { QueryClient } from '@tanstack/react-query'
import { devModeQueryOptions } from '../queries/devMode'

export type ExperimentalFeature =
  | 'ai-chat'
  | 'ai-models'
  | 'load-balancer'
  | 'iac'
  | 'cloudflare-tunnel'
  | 'access-roles'
  | 'global-environments'
  | 'resource-advisor'
  | 'cli-console'
  | 'mobile-api'
  | 'saml-scim'
  | 'plugins'
  | 'git-push'
  | 'canvas'

export const BETA_FEATURES_PATH = '/settings/beta-features'

// A type alias, not an interface: router search params need the implicit index signature.
export type BetaFeaturesSearch = { feature?: string; from?: string }

export function isFeatureVisible(
  feature: ExperimentalFeature | undefined,
  enabled: readonly string[],
): boolean {
  return feature === undefined || enabled.includes(feature)
}

// A gated destination that is switched off: shown, but leads to Beta features.
export function isFeatureLocked(
  feature: ExperimentalFeature | undefined,
  enabled: readonly string[],
): feature is ExperimentalFeature {
  return feature !== undefined && !enabled.includes(feature)
}

export function filterByFeature<T extends { feature?: ExperimentalFeature }>(
  items: readonly T[],
  enabled: readonly string[],
): T[] {
  return items.filter((i) => isFeatureVisible(i.feature, enabled))
}

// Only same-origin absolute paths may be carried as a return target.
export function safeReturnPath(from: string | undefined): string | undefined {
  if (!from || !from.startsWith('/') || from.startsWith('//')) return undefined
  return from
}

export function lockedSearch(
  feature: ExperimentalFeature,
  from?: string,
): BetaFeaturesSearch {
  const back = safeReturnPath(from)
  return back ? { feature, from: back } : { feature }
}

// Route guard: sends the visitor to the Beta features page for this feature
// when it is switched off, before any loader for the gated page fires a
// request that would 404.
export async function requireExperimental(
  queryClient: QueryClient,
  feature: ExperimentalFeature,
  from?: string,
): Promise<void> {
  const status = await queryClient.ensureQueryData(devModeQueryOptions())
  if (!isFeatureVisible(feature, status.experimental)) {
    // eslint-disable-next-line @typescript-eslint/only-throw-error
    throw redirect({
      to: BETA_FEATURES_PATH,
      search: lockedSearch(feature, from),
    })
  }
}
