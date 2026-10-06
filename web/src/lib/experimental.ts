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

export function isFeatureVisible(
  feature: ExperimentalFeature | undefined,
  enabled: readonly string[],
): boolean {
  return feature === undefined || enabled.includes(feature)
}

export function filterByFeature<T extends { feature?: ExperimentalFeature }>(
  items: readonly T[],
  enabled: readonly string[],
): T[] {
  return items.filter((i) => isFeatureVisible(i.feature, enabled))
}

// Route guard: sends the visitor home when the feature is switched off, before
// any loader for the gated page fires a request that would 404.
export async function requireExperimental(
  queryClient: QueryClient,
  feature: ExperimentalFeature,
): Promise<void> {
  const status = await queryClient.ensureQueryData(devModeQueryOptions())
  if (!isFeatureVisible(feature, status.experimental)) {
    // eslint-disable-next-line @typescript-eslint/only-throw-error
    throw redirect({ to: '/' })
  }
}
