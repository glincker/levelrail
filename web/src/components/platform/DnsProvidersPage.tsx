import { useCallback, useEffect, useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { SearchGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Input } from '@/components/ui/input'
import { DnsHostTile } from '@/components/dns/DnsHostChip'
import { ApiError } from '@/lib/apiError'
import { dnsZonesQueryOptions } from '@/queries/dns'
import type { DnsZone } from '@/types/dns'
import { DomainsUsing } from './dns-providers/DomainsUsing'
import { ProviderSheet } from './dns-providers/ProviderSheet'
import { ZoneSheet } from './dns-providers/ZoneSheet'
import { ZonesTable } from './dns-providers/ZonesTable'
import {
  PROVIDER_HASH,
  hostKey,
  PROVIDER_IDS,
  providerFromHash,
  type ProviderId,
} from './dns-providers/providers'
import { useProviderStates } from './dns-providers/useProviderStates'

const STATUS_VARIANT = {
  connected: 'success',
  attention: 'destructive',
  notConnected: 'muted',
} as const

const forbidden = (e: unknown) => e instanceof ApiError && e.status === 403

/** One calm page for DNS hosts: provider strip, hosted zones, and the domains each host serves. */
export function DnsProvidersPage() {
  const { t } = useTranslation('dnsProviders')
  const zonesQuery = useQuery(dnsZonesQueryOptions())
  const [testErrors, setTestErrors] = useState<
    Partial<Record<ProviderId, string>>
  >({})
  const { states } = useProviderStates(testErrors)
  const [query, setQuery] = useState('')
  const [filter, setFilter] = useState<'all' | ProviderId>('all')
  const [sheet, setSheet] = useState<ProviderId | null>(() =>
    providerFromHash(typeof window === 'undefined' ? '' : window.location.hash),
  )
  const [zone, setZone] = useState<DnsZone | null>(null)

  useEffect(() => {
    const onHash = () => setSheet(providerFromHash(window.location.hash))
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  const openProvider = (id: ProviderId | null) => {
    setSheet(id)
    const hash = id ? `#${PROVIDER_HASH[id]}` : ''
    window.history.replaceState(
      null,
      '',
      `${window.location.pathname}${window.location.search}${hash}`,
    )
  }

  const allZones = useMemo(
    () => zonesQuery.data?.zones ?? [],
    [zonesQuery.data],
  )
  const q = query.trim().toLowerCase()
  const zoneCount = (id: ProviderId) =>
    allZones.filter((z) => z.provider === id).length
  const tiles = PROVIDER_IDS.filter(
    (id) =>
      !q ||
      t(`providers.${id}`).toLowerCase().includes(q) ||
      allZones.some((z) => z.provider === id && z.name.includes(q)) ||
      'dns'.includes(q),
  )
  const zones = allZones.filter(
    (z) =>
      (filter === 'all' || z.provider === filter) &&
      z.name.toLowerCase().includes(q),
  )
  const refetchZones = useCallback(
    async () => (await zonesQuery.refetch()).data?.zones,
    [zonesQuery],
  )
  const anyConnected = PROVIDER_IDS.some(
    (id) => states[id].status !== 'notConnected',
  )

  return (
    <div className="space-y-5">
      <div className="relative max-w-md">
        <SearchGlassIcon
          className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground"
          aria-hidden="true"
        />
        <Input
          type="search"
          aria-label={t('searchLabel')}
          placeholder={t('search')}
          className="pl-8"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
      </div>

      {!anyConnected ? (
        <p className="text-sm text-muted-foreground">{t('empty')}</p>
      ) : null}

      <ul
        className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4"
        aria-label={t('title')}
      >
        {tiles.map((id) => {
          const s = states[id]
          const count = zoneCount(id)
          return (
            <li key={id}>
              <button
                type="button"
                id={PROVIDER_HASH[id]}
                aria-label={t('tile.open', { provider: t(`providers.${id}`) })}
                className="flex w-full items-center gap-3 rounded-lg border border-border bg-card p-3 text-left hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring motion-reduce:transition-none"
                onClick={() => openProvider(id)}
              >
                <DnsHostTile provider={hostKey(id)} size="md" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">
                    {t(`providers.${id}`)}
                  </span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {s.status === 'connected'
                      ? (s.label ?? t('tile.zones', { count }))
                      : s.status === 'attention'
                        ? (s.error ?? t('attention.noCredential'))
                        : ''}
                  </span>
                </span>
                <Badge variant={STATUS_VARIANT[s.status]}>
                  {t(`status.${s.status}`)}
                </Badge>
              </button>
            </li>
          )
        })}
        <li className="flex items-center justify-center rounded-lg border border-dashed border-border p-3 text-xs text-muted-foreground">
          {t('more')}
        </li>
      </ul>

      <section className="space-y-2" aria-labelledby="dnsp-zones">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 id="dnsp-zones" className="text-sm font-medium">
            {t('zones.heading')}
          </h2>
          <select
            aria-label={t('zones.filter')}
            className="h-8 rounded-md border border-input bg-background px-2 text-sm"
            value={filter}
            onChange={(e) => setFilter(e.target.value as 'all' | ProviderId)}
          >
            <option value="all">{t('zones.allProviders')}</option>
            {PROVIDER_IDS.map((id) => (
              <option key={id} value={id}>
                {t(`providers.${id}`)}
              </option>
            ))}
          </select>
        </div>
        {zonesQuery.error && forbidden(zonesQuery.error) ? (
          <div role="status" className="space-y-0.5 text-sm">
            <p className="font-medium">{t('zones.adminOnly')}</p>
            <p className="text-muted-foreground">{t('zones.adminOnlyBody')}</p>
          </div>
        ) : zonesQuery.error ? (
          <p role="alert" className="text-sm text-tone-danger">
            {t('loadFailed')}
          </p>
        ) : zones.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {q ? t('noMatch') : t('zones.none')}
          </p>
        ) : (
          <ZonesTable zones={zones} onOpen={setZone} />
        )}
      </section>

      <DomainsUsing query={query} onConnect={openProvider} />

      <ProviderSheet
        id={sheet}
        state={sheet ? states[sheet] : null}
        zones={allZones}
        refetchZones={refetchZones}
        onOpenZone={(z) => {
          openProvider(null)
          setZone(z)
        }}
        onClose={() => openProvider(null)}
        onTestResult={(id, error) =>
          setTestErrors((prev) => {
            const next = { ...prev }
            if (error) next[id] = error
            else delete next[id]
            return next
          })
        }
      />
      <ZoneSheet zone={zone} onClose={() => setZone(null)} />
    </div>
  )
}
