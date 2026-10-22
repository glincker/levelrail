export type AppTabGroupId =
  'basics' | 'network' | 'data' | 'observe' | 'advanced'

export interface AppTabGroupDef {
  id: AppTabGroupId
  /** Route slugs under /apps/$name/, in display order. */
  slugs: readonly string[]
}

// New tab: add its route slug to one group here. Anything unlisted lands in
// Advanced, so a tab can never go missing from the nav.
export const APP_TAB_GROUPS: readonly AppTabGroupDef[] = [
  { id: 'basics', slugs: ['overview', 'deploys', 'environment', 'logs'] },
  {
    id: 'network',
    slugs: [
      'go-live',
      'domains',
      'static-routes',
      'cache',
      'network',
      'loadbalancer',
    ],
  },
  { id: 'data', slugs: ['volumes'] },
  {
    id: 'observe',
    slugs: [
      'metrics',
      'traces',
      'streams',
      'alerts',
      'requests',
      'traffic',
      'errors',
    ],
  },
  { id: 'advanced', slugs: [] },
]

// Tabs with nothing to show for a static site (no container, volume or shell).
export const STATIC_HIDDEN_SLUGS: readonly string[] = [
  'volumes',
  'exec',
  'scheduled-tasks',
]

export const FALLBACK_GROUP: AppTabGroupId = 'advanced'

export function groupForSlug(slug: string): AppTabGroupId {
  return (
    APP_TAB_GROUPS.find((g) => g.slugs.includes(slug))?.id ?? FALLBACK_GROUP
  )
}

export interface GroupedTabs<T> {
  id: AppTabGroupId
  items: T[]
}

export interface GroupTabsOptions {
  isStatic?: boolean
  showAll?: boolean
}

function rank(group: AppTabGroupDef, slug: string): number {
  const i = group.slugs.indexOf(slug)
  return i === -1 ? group.slugs.length : i
}

/** Buckets tabs into the five groups; empty groups are dropped. */
export function groupTabs<T extends { slug: string }>(
  tabs: readonly T[],
  { isStatic = false, showAll = false }: GroupTabsOptions = {},
): GroupedTabs<T>[] {
  const hidden = isStatic && !showAll ? STATIC_HIDDEN_SLUGS : []
  const visible = tabs.filter((t) => !hidden.includes(t.slug))
  return APP_TAB_GROUPS.map((g) => ({
    id: g.id,
    items: visible
      .filter((t) => groupForSlug(t.slug) === g.id)
      .sort((a, b) => rank(g, a.slug) - rank(g, b.slug)),
  })).filter((g) => g.items.length > 0)
}

export function hiddenTabCount(
  tabs: readonly { slug: string }[],
  isStatic: boolean,
): number {
  if (!isStatic) return 0
  return tabs.filter((t) => STATIC_HIDDEN_SLUGS.includes(t.slug)).length
}

export interface NarrowSplit<T extends { id: string }> {
  visible: T[]
  more: T[]
}

/** On a narrow screen only the active group stays in the bar; the rest go under More. */
export function splitForNarrow<T extends { id: string }>(
  groups: readonly T[],
  activeId: string | undefined,
  narrow: boolean,
): NarrowSplit<T> {
  if (!narrow) return { visible: [...groups], more: [] }
  const visible = groups.filter((g) => g.id === activeId)
  const first = groups[0]
  if (visible.length === 0 && first) visible.push(first)
  return { visible, more: groups.filter((g) => !visible.includes(g)) }
}
