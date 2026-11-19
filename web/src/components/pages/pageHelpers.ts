export const PAGE_TABS = [
  'deployments',
  'domains',
  'routes',
  'settings',
] as const
export type PageTab = (typeof PAGE_TABS)[number]

export function suggestSiteName(taken: readonly string[]): string {
  if (!taken.includes('my-site')) return 'my-site'
  for (let i = 2; i < 1000; i++) {
    if (!taken.includes(`my-site-${i}`)) return `my-site-${i}`
  }
  return 'my-site'
}
