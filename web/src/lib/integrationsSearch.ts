export interface IntegrationsSearch {
  view?: 'catalog'
  connection?: string
  reauthorize?: boolean
}

// Shared by the hub and the legacy settings redirect so links keep their params.
export function validateIntegrationsSearch(
  search: Record<string, unknown>,
): IntegrationsSearch {
  const out: IntegrationsSearch = {}
  if (search.view === 'catalog') out.view = 'catalog'
  if (typeof search.connection === 'string' && search.connection !== '') {
    out.connection = search.connection
  }
  if (
    search.reauthorize === true ||
    search.reauthorize === 1 ||
    search.reauthorize === '1'
  ) {
    out.reauthorize = true
  }
  return out
}
