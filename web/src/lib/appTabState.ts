// Pure visibility rules for app tab pages: show only what the user can act on.

// The editor only earns its space once there is a volume or the user starts adding one.
export function showVolumesEditor(count: number, adding: boolean): boolean {
  return count > 0 || adding
}

// Ingress limits and shielding only apply to an app that has a domain.
export function showDomainExtras(domains: string[] | undefined): boolean {
  return (domains?.length ?? 0) > 0
}

export type ErrorsPanelState = 'loading' | 'off' | 'empty' | 'issues'

// Filters belong to the issues state, or to an empty list the user narrowed themselves.
export function errorsPanelState(input: {
  trackingLoading: boolean
  trackingOn: boolean
  listLoading: boolean
  issueCount: number
  filtersChanged: boolean
}): ErrorsPanelState {
  if (input.trackingLoading) return 'loading'
  if (!input.trackingOn) return 'off'
  if (input.listLoading) return 'loading'
  if (input.issueCount > 0 || input.filtersChanged) return 'issues'
  return 'empty'
}

// Policy, TTL, contexts and lifecycle matter once PR previews are on or any exist.
export function showPreviewExtras(input: {
  loading: boolean
  prPreviewsOn: boolean
  previewCount: number
}): boolean {
  if (input.loading) return false
  return input.prPreviewsOn || input.previewCount > 0
}
