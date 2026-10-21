import { DOMAIN_TABS, type DomainTab } from './domainTabs'

export type DomainFix = 'renew' | 'dns'

export interface DomainSearch {
  app: string
  tab?: DomainTab
  fix?: DomainFix
}

// Plain function rather than zod so validateSearch stays out of the eagerly loaded bundle.
export function validateDomainSearch(
  search: Record<string, unknown>,
): DomainSearch {
  const app = typeof search.app === 'string' ? search.app : ''
  const tab = DOMAIN_TABS.find((t) => t === search.tab)
  const fix =
    search.fix === 'renew' || search.fix === 'dns' ? search.fix : undefined
  return { app, ...(tab ? { tab } : {}), ...(fix ? { fix } : {}) }
}
