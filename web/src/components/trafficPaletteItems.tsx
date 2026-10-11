import * as React from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  CheckCircleIcon,
  GlobeIcon,
  ListBulletsIcon,
  PlusIcon,
  StethoscopeIcon,
  TrafficSignalIcon,
} from '@phosphor-icons/react/dist/ssr'
import { toast } from '@/components/ui/toast'
import { fuzzyFilter } from '@/lib/fuzzy'
import { useRouteAvailable } from '@/lib/routeAvailability'
import { deriveDomainStatus } from '@/lib/trafficStatus'
import {
  domainCheckKeys,
  fetchDomainCheck,
  type DomainCheckStatus,
} from '@/queries/domainCheck'
import type { Domain } from '@/queries/domains'
import { DomainStatusBadge } from './traffic/DomainStatusBadge'
import { chordFor } from './shell/navModel'
import type { PaletteItem } from './commandPaletteData'

const MAX_VERIFY_MATCHES = 3
const MAX_DNS_RECORD_MATCHES = 5
const MIN_DNS_QUERY_LENGTH = 2
const DOMAIN_ROUTE = '/domains/$domain'

type NavigateFn = (opts: {
  to: string
  params?: Record<string, string>
  search?: Record<string, string>
}) => void

type TrafficT = TFunction<'traffic'>

export interface TrafficPaletteInput {
  domains: readonly Domain[]
  query: string
  currentDomain?: string
  routeAvailable: (to: string) => boolean
  t: TrafficT
  navigate: NavigateFn
  verify: (appName: string, domain: string) => void
}

export interface TrafficPaletteItems {
  /** Filtered by the palette's own fuzzy pass like every other base item. */
  base: PaletteItem[]
  /** Already query-matched and capped; appended only while searching. */
  search: PaletteItem[]
}

/** Domain currently open at /domains/$domain, decoded, or undefined. */
export function domainFromPath(pathname: string): string | undefined {
  const raw = /^\/domains\/([^/]+)/.exec(pathname)?.[1]
  if (!raw) return undefined
  try {
    return decodeURIComponent(raw)
  } catch {
    return raw
  }
}

function statusBadge(domain: Domain): React.ReactNode {
  const derived = deriveDomainStatus({
    maintenanceEnabled: domain.maintenance_enabled,
    acmeFailure: domain.acme_failure,
  })
  if (derived.reason === 'noCheck') return undefined
  return <DomainStatusBadge status={derived.status} size="sm" />
}

function domainItems(input: TrafficPaletteInput): PaletteItem[] {
  return input.domains.map((d) => ({
    key: `domain-${d.domain}`,
    label: d.domain,
    group: 'Domains',
    icon: <GlobeIcon />,
    keywords: d.service_name,
    badge: statusBadge(d),
    run: () =>
      input.navigate({
        to: '/apps/$name/domains',
        params: { name: d.service_name },
      }),
  }))
}

function navItems(input: TrafficPaletteInput): PaletteItem[] {
  const { t, navigate, routeAvailable } = input
  const items: PaletteItem[] = [
    {
      key: 'action-add-domain',
      label: t('palette.addDomain'),
      group: 'Actions',
      icon: <PlusIcon />,
      run: () => navigate({ to: '/domains', search: { add: '1' } }),
    },
    {
      key: 'nav-proxy',
      label: t('nav.proxy'),
      group: 'Navigate',
      icon: <TrafficSignalIcon />,
      hint: chordFor('/network/proxy'),
      run: () => navigate({ to: '/network/proxy' }),
    },
  ]
  if (routeAvailable('/dns')) {
    items.push({
      key: 'nav-dns',
      label: t('nav.dns'),
      group: 'Navigate',
      icon: <ListBulletsIcon />,
      hint: chordFor('/dns'),
      run: () => navigate({ to: '/dns' }),
    })
  }
  if (routeAvailable('/settings/domains')) {
    items.push({
      key: 'settings-domains',
      label: t('palette.domainSettings'),
      group: 'Settings',
      icon: <GlobeIcon />,
      run: () => navigate({ to: '/settings/domains' }),
    })
  }
  return items
}

function contextItems(input: TrafficPaletteInput): PaletteItem[] {
  const { currentDomain, domains, t, navigate, routeAvailable, verify } = input
  if (!currentDomain) return []
  const owner = domains.find((d) => d.domain === currentDomain)
  const items: PaletteItem[] = []
  if (owner) {
    items.push({
      key: `domain-action-verify-${currentDomain}`,
      label: t('palette.verify', { domain: currentDomain }),
      group: 'Domain actions',
      icon: <CheckCircleIcon />,
      run: () => verify(owner.service_name, owner.domain),
    })
  }
  if (routeAvailable(DOMAIN_ROUTE)) {
    items.push({
      key: `domain-action-doctor-${currentDomain}`,
      label: t('palette.doctor', { domain: currentDomain }),
      group: 'Domain actions',
      icon: <StethoscopeIcon />,
      run: () =>
        navigate({
          to: DOMAIN_ROUTE,
          params: { domain: currentDomain },
          search: { doctor: '1' },
        }),
    })
  }
  return items
}

function searchItems(input: TrafficPaletteInput): PaletteItem[] {
  const q = input.query.trim()
  if (q.length < MIN_DNS_QUERY_LENGTH) return []
  const matches = fuzzyFilter(
    input.domains,
    q,
    (d) => d.domain,
    (d) => d.service_name,
  )
  const verifies: PaletteItem[] = matches
    .slice(0, MAX_VERIFY_MATCHES)
    .map((d) => ({
      key: `domain-action-verify-${d.domain}`,
      label: input.t('palette.verify', { domain: d.domain }),
      group: 'Domain actions',
      icon: <CheckCircleIcon />,
      run: () => input.verify(d.service_name, d.domain),
    }))
  const records: PaletteItem[] = matches
    .slice(0, MAX_DNS_RECORD_MATCHES)
    .map((d) => ({
      key: `dns-records-${d.domain}`,
      label: input.t('palette.dnsRecordsFor', { domain: d.domain }),
      group: 'DNS records',
      icon: <ListBulletsIcon />,
      run: () =>
        input.navigate({
          to: '/apps/$name/domains',
          params: { name: d.service_name },
        }),
    }))
  return [...verifies, ...records]
}

/** Pure palette additions for the Traffic area, kept apart for testing. */
export function buildTrafficPaletteItems(
  input: TrafficPaletteInput,
): TrafficPaletteItems {
  return {
    base: [...contextItems(input), ...navItems(input), ...domainItems(input)],
    search: searchItems(input),
  }
}

const CHECK_RESULT_KEYS: Record<DomainCheckStatus, string> = {
  connected: 'palette.checkResult.connected',
  propagating: 'palette.checkResult.propagating',
  not_resolving: 'palette.checkResult.not_resolving',
  resolves_elsewhere: 'palette.checkResult.resolves_elsewhere',
  unconfigured: 'palette.checkResult.unconfigured',
}

export function useTrafficPaletteItems({
  domains,
  query,
  currentDomain,
}: {
  domains: readonly Domain[]
  query: string
  currentDomain?: string
}): TrafficPaletteItems {
  const { t } = useTranslation('traffic')
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const routeAvailable = useRouteAvailable()

  return React.useMemo(() => {
    const verify = (appName: string, domain: string) => {
      fetchDomainCheck(appName, domain)
        .then((result) => {
          queryClient.setQueryData(
            domainCheckKeys.detail(appName, domain),
            result,
          )
          toast.add({
            title: t('palette.checked', {
              domain,
              result: t(CHECK_RESULT_KEYS[result.status] as 'nav.dns'),
            }),
            type: 'success',
          })
        })
        .catch((error: unknown) => {
          toast.add({
            title: error instanceof Error ? error.message : String(error),
            type: 'error',
          })
        })
    }
    return buildTrafficPaletteItems({
      domains,
      query,
      currentDomain,
      routeAvailable,
      t,
      verify,
      navigate: (opts) => void navigate(opts),
    })
  }, [domains, query, currentDomain, routeAvailable, t, navigate, queryClient])
}
