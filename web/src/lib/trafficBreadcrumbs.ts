export type CrumbLabel = { key: string } | { text: string }

export interface Crumb {
  id: string
  label: CrumbLabel
  to?: string
  params?: Record<string, string>
  search?: Record<string, string>
  current?: boolean
}

export type TrafficList = 'domains' | 'dns'

export interface TrafficCrumbInput {
  pathname: string
  from?: string
  listSearch?: Partial<Record<TrafficList, Record<string, string>>>
}

const DOMAIN_TABS = new Set([
  'headers',
  'forwarding',
  'access',
  'cache',
  'ports',
  'redirects',
  'activity',
])
const ZONE_TABS = new Set(['activity', 'settings'])

function decode(segment: string): string {
  try {
    return decodeURIComponent(segment)
  } catch {
    return segment
  }
}

function listCrumb(
  list: TrafficList,
  input: TrafficCrumbInput,
  current: boolean,
): Crumb {
  const search = input.listSearch?.[list]
  return {
    id: list,
    label: { key: `nav.${list}` },
    to: current ? undefined : `/${list}`,
    search: search && Object.keys(search).length > 0 ? search : undefined,
    current,
  }
}

function tabCrumb(tab: string): Crumb {
  return { id: `tab-${tab}`, label: { key: `tab.${tab}` }, current: true }
}

function domainCrumbs(
  domain: string,
  tab: string | undefined,
  input: TrafficCrumbInput,
): Crumb[] {
  const app = /^app:(.+)$/.exec(input.from ?? '')?.[1]
  const lead: Crumb[] = app
    ? [
        { id: 'apps', label: { key: 'nav.apps' }, to: '/apps' },
        {
          id: 'app',
          label: { text: app },
          to: '/apps/$name/overview',
          params: { name: app },
        },
        {
          id: 'app-domains',
          label: { key: 'nav.domains' },
          to: '/apps/$name/domains',
          params: { name: app },
        },
      ]
    : [listCrumb('domains', input, false)]
  const self: Crumb = {
    id: 'domain',
    label: { text: domain },
    to: tab ? '/domains/$domain' : undefined,
    params: tab ? { domain } : undefined,
    current: !tab,
  }
  return tab ? [...lead, self, tabCrumb(tab)] : [...lead, self]
}

function zoneCrumbs(segments: string[], input: TrafficCrumbInput): Crumb[] {
  const [first = '', tab] = segments
  if (first === 'add') {
    return [
      listCrumb('dns', input, false),
      { id: 'add', label: { key: 'tab.add' }, current: true },
    ]
  }
  const zone = decode(first)
  const hasTab = tab !== undefined && ZONE_TABS.has(tab)
  const self: Crumb = {
    id: 'zone',
    label: { text: zone },
    to: hasTab ? '/dns/$zone' : undefined,
    params: hasTab ? { zone } : undefined,
    current: !hasTab,
  }
  const lead = listCrumb('dns', input, false)
  return hasTab && tab ? [lead, self, tabCrumb(tab)] : [lead, self]
}

/**
 * Crumb trail for the Traffic area, or null outside it. List pages return a
 * single current crumb so the component can decide to hide it.
 */
export function trafficCrumbs(input: TrafficCrumbInput): Crumb[] | null {
  const segments = input.pathname.split('/').filter(Boolean)
  const [root, ...rest] = segments
  if (root === 'domains') {
    if (rest.length === 0) return [listCrumb('domains', input, true)]
    const [name, tab] = rest
    return domainCrumbs(
      decode(name ?? ''),
      tab !== undefined && DOMAIN_TABS.has(tab) ? tab : undefined,
      input,
    )
  }
  if (root === 'dns') {
    if (rest.length === 0) return [listCrumb('dns', input, true)]
    return zoneCrumbs(rest, input)
  }
  if (root === 'proxy' || input.pathname === '/network/proxy') {
    return [{ id: 'proxy', label: { key: 'nav.proxy' }, current: true }]
  }
  return null
}

/** The crumb a "Back" link or the mobile back affordance points at. */
export function parentCrumb(crumbs: readonly Crumb[]): Crumb | undefined {
  const linked = crumbs.filter((c) => c.to !== undefined)
  return linked[linked.length - 1]
}

const STORAGE_PREFIX = 'traffic.listSearch.'

export function readListSearch(
  list: TrafficList,
): Record<string, string> | undefined {
  try {
    const raw = window.sessionStorage.getItem(STORAGE_PREFIX + list)
    if (!raw) return undefined
    const parsed: unknown = JSON.parse(raw)
    if (typeof parsed !== 'object' || parsed === null) return undefined
    const out: Record<string, string> = {}
    for (const [k, v] of Object.entries(parsed)) {
      if (typeof v === 'string') out[k] = v
    }
    return out
  } catch {
    return undefined
  }
}

export function rememberListSearch(
  list: TrafficList,
  search: Record<string, string>,
): void {
  try {
    window.sessionStorage.setItem(STORAGE_PREFIX + list, JSON.stringify(search))
  } catch {
    // Storage unavailable: Back just returns to the unfiltered list.
  }
}
