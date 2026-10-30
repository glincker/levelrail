// Single typed index of every platform setting. The overview search, the
// command palette and the card anchors all read from here so they cannot drift.
import { DASHBOARD_URL_ANCHOR } from '../components/DashboardUrlCard'
import type platformEn from '../locales/en/platformSettings.json'

export type PlatformSettingId = keyof (typeof platformEn)['index']

export type PlatformPageId = 'https' | 'ingress' | 'addresses' | 'dns-providers'

export const PLATFORM_PAGE_PATH = {
  https: '/settings/platform/https',
  ingress: '/settings/platform/ingress',
  addresses: '/settings/platform/addresses',
  'dns-providers': '/settings/platform/dns-providers',
} as const satisfies Record<PlatformPageId, string>

export const PLATFORM_OVERVIEW_PATH = '/settings/platform'

export interface PlatformSettingEntry {
  id: PlatformSettingId
  page: PlatformPageId
  /** Element id of the card section on that page. */
  anchor: string
  /** Extra match words, ranked with the translated title and description. */
  keywords: string
}

export const PLATFORM_ANCHORS = {
  enableHttps: 'enable-https',
  certificates: 'certificates',
  ingressSettings: 'ingress-settings',
  ingressEdge: 'ingress-edge',
  connectivity: 'connectivity',
  reverseProxy: 'reverse-proxy-guide',
  dashboardUrl: DASHBOARD_URL_ANCHOR,
  appsBaseDomain: 'apps-base-domain',
  fallbackDomains: 'fallback-domains',
  domainAutomation: 'domain-automation',
  cloudflareDns: 'cloudflare-dns',
  route53Dns: 'route53-dns',
  googleCloudDns: 'google-cloud-dns',
} as const

export const PLATFORM_SETTINGS: readonly PlatformSettingEntry[] = [
  {
    id: 'enableHttps',
    page: 'https',
    anchor: PLATFORM_ANCHORS.enableHttps,
    keywords: 'https ssl tls lets encrypt acme secure padlock certificate',
  },
  {
    id: 'certificates',
    page: 'https',
    anchor: PLATFORM_ANCHORS.certificates,
    keywords: 'certificate expiry renew tls ssl issuer',
  },
  {
    id: 'ingressSettings',
    page: 'ingress',
    anchor: PLATFORM_ANCHORS.ingressSettings,
    keywords: 'ingress primary domain acme email hsts proxy',
  },
  {
    id: 'ingressEdge',
    page: 'ingress',
    anchor: PLATFORM_ANCHORS.ingressEdge,
    keywords: 'edge ingress listener ports 80 443 proxy',
  },
  {
    id: 'connectivity',
    page: 'ingress',
    anchor: PLATFORM_ANCHORS.connectivity,
    keywords: 'connectivity ports reachable firewall check 80 443',
  },
  {
    id: 'reverseProxy',
    page: 'ingress',
    anchor: PLATFORM_ANCHORS.reverseProxy,
    keywords: 'reverse proxy nginx caddy traefik behind guide',
  },
  {
    id: 'dashboardUrl',
    page: 'addresses',
    anchor: PLATFORM_ANCHORS.dashboardUrl,
    keywords: 'dashboard address url domain host public link',
  },
  {
    id: 'appsBaseDomain',
    page: 'addresses',
    anchor: PLATFORM_ANCHORS.appsBaseDomain,
    keywords: 'apps base domain wildcard subdomain default',
  },
  {
    id: 'fallbackDomains',
    page: 'addresses',
    anchor: PLATFORM_ANCHORS.fallbackDomains,
    keywords: 'fallback sslip nip domains automatic hostname ip',
  },
  {
    id: 'domainAutomation',
    page: 'addresses',
    anchor: PLATFORM_ANCHORS.domainAutomation,
    keywords: 'automation dns records auto create domains wildcard',
  },
  {
    id: 'cloudflareDns',
    page: 'dns-providers',
    anchor: PLATFORM_ANCHORS.cloudflareDns,
    keywords: 'cloudflare dns provider token zone',
  },
  {
    id: 'route53Dns',
    page: 'dns-providers',
    anchor: PLATFORM_ANCHORS.route53Dns,
    keywords: 'route 53 route53 aws amazon dns provider hosted zone',
  },
  {
    id: 'googleCloudDns',
    page: 'dns-providers',
    anchor: PLATFORM_ANCHORS.googleCloudDns,
    keywords: 'google cloud gcp dns provider service account',
  },
]

export interface PlatformSearchText {
  title: string
  description: string
}

/** Entries whose text or keywords contain every typed word. */
export function filterPlatformSettings(
  query: string,
  text: (id: PlatformSettingId) => PlatformSearchText,
): PlatformSettingEntry[] {
  const words = query.toLowerCase().split(/\s+/).filter(Boolean)
  if (words.length === 0) return [...PLATFORM_SETTINGS]
  return PLATFORM_SETTINGS.filter((entry) => {
    const t = text(entry.id)
    const hay =
      `${t.title} ${t.description} ${entry.keywords} ${entry.page}`.toLowerCase()
    return words.every((w) => hay.includes(w))
  })
}
