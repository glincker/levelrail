import {
  useStatusComponents,
  useStatusPageSettings,
} from '../queries/statusPage'

export type StatusPageTab = 'settings' | 'components' | 'incidents' | 'webhooks'

// Off: no tabs, just the enable card. Incidents and webhooks need components.
export function statusPageTabs(input: {
  enabled: boolean
  componentCount: number
}): StatusPageTab[] {
  if (!input.enabled) return []
  if (input.componentCount === 0) return ['settings', 'components']
  return ['settings', 'components', 'incidents', 'webhooks']
}

export const showRevokedTokens = (revokedCount: number): boolean =>
  revokedCount > 0

export const showHeartbeat = (channelCount: number): boolean => channelCount > 0

// Hidden while loading so nothing flashes in and out.
export function showRegistryExtras(input: {
  loading: boolean
  credentialCount: number
  builtInEnabled: boolean
  packageRegistryCount: number
}): boolean {
  if (input.loading) return false
  return (
    input.credentialCount > 0 ||
    input.builtInEnabled ||
    input.packageRegistryCount > 0
  )
}

// Reuses the panels' own query keys so nothing is fetched twice.
export function useStatusPageTabs(): {
  loading: boolean
  tabs: StatusPageTab[]
} {
  const settings = useStatusPageSettings()
  const components = useStatusComponents()
  const enabled = settings.data?.enabled ?? false
  const loading = settings.isLoading || (enabled && components.isLoading)
  return {
    loading,
    tabs: statusPageTabs({
      enabled,
      componentCount: components.data?.length ?? 0,
    }),
  }
}
