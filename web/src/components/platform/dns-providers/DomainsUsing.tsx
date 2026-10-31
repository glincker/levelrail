import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { DnsHostTile } from '@/components/dns/DnsHostChip'
import { normalizeDomain, providerLabel } from '@/components/dns/dnsSetup'
import { dnsZonesQueryOptions } from '@/queries/dns'
import { domainsQueryOptions } from '@/queries/domains'
import {
  DETECT_AUTO_MAX,
  providerFromDetection,
  type ProviderId,
} from './providers'
import { useDetections } from './useDetections'

export function DomainsUsing({
  query,
  onConnect,
}: {
  query: string
  onConnect: (id: ProviderId) => void
}) {
  const { t } = useTranslation('dnsProviders')
  const zones = useQuery(dnsZonesQueryOptions())
  const doms = useQuery(domainsQueryOptions())
  const names = useMemo(() => {
    const all = [
      ...(zones.data?.zones ?? []).map((z) => z.name),
      ...(doms.data ?? []).map((d) => d.domain),
    ]
      .filter((d) => !d.startsWith('*'))
      .map(normalizeDomain)
      .filter(Boolean)
    return [...new Set(all)].sort()
  }, [zones.data, doms.data])
  const tooMany = names.length > DETECT_AUTO_MAX
  const [manual, setManual] = useState(false)
  const results = useDetections(names, !tooMany || manual)
  const q = query.trim().toLowerCase()
  const visible = names.filter((n) => n.includes(q))

  return (
    <section className="space-y-2" aria-labelledby="dnsp-domains">
      <div className="flex items-center justify-between gap-2">
        <h2 id="dnsp-domains" className="text-sm font-medium">
          {t('domains.heading')}
        </h2>
        {tooMany && !manual ? (
          <span className="flex items-center gap-2 text-xs text-muted-foreground">
            {t('domains.tooMany', { count: names.length })}
            <Button size="sm" variant="outline" onClick={() => setManual(true)}>
              {t('domains.detect')}
            </Button>
          </span>
        ) : null}
      </div>
      {visible.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {q ? t('noMatch') : t('domains.none')}
        </p>
      ) : (
        <ul className="divide-y divide-border rounded-lg border border-border text-sm">
          {visible.map((n) => {
            const r = results[n]
            const det = r && r !== 'error' ? r : undefined
            const id = providerFromDetection(det)
            const waiting = (!tooMany || manual) && r === undefined
            return (
              <li
                key={n}
                className="flex flex-wrap items-center gap-2 px-3 py-2"
              >
                <span className="min-w-0 flex-1 truncate font-mono">{n}</span>
                {waiting ? (
                  <span className="text-xs text-muted-foreground">
                    {t('domains.detecting')}
                  </span>
                ) : r === 'error' ? (
                  <Badge variant="muted">{t('domains.detectFailed')}</Badge>
                ) : det && det.provider && det.provider !== 'manual' ? (
                  <>
                    <span className="flex items-center gap-1.5">
                      <DnsHostTile provider={det.provider} size="xs" />
                      {providerLabel(det.provider)}
                    </span>
                    <Badge variant={det.connected ? 'success' : 'warning'}>
                      {det.connected
                        ? t('domains.connectedVia')
                        : t('domains.notConnectedVia')}
                    </Badge>
                    {!det.connected && id ? (
                      <Button size="sm" onClick={() => onConnect(id)}>
                        {t('domains.connect', {
                          provider: t(`providers.${id}`),
                        })}
                      </Button>
                    ) : null}
                  </>
                ) : det ? (
                  <Badge variant="muted">{t('domains.unknownHost')}</Badge>
                ) : null}
              </li>
            )
          })}
        </ul>
      )}
    </section>
  )
}
