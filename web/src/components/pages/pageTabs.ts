// Kept apart from pageHelpers so the route file can validate search without pulling zip code into first load.
export const PAGE_TABS = [
  'overview',
  'deployments',
  'domains',
  'routes',
  'settings',
] as const
export type PageTab = (typeof PAGE_TABS)[number]
