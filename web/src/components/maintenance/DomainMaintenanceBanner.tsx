import { Link } from '@tanstack/react-router'
import { useEffect, useState } from 'react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { domainKeys, domainsQueryOptions } from '../../queries/domains'
import {
  fleetMaintenanceQueryOptions,
  useClearDomainMaintenance,
  type FleetMaintenanceEntry,
} from '../../queries/domainMaintenance'
import { isEnforced } from '../../lib/domainEnforcement'
import { elapsedSince, formatElapsed } from '../../lib/maintenanceElapsed'

const MAX_SHOWN = 3
const TICK_MS = 30_000

function useNow(): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = setInterval(() => {
      setNow(Date.now())
    }, TICK_MS)
    return () => {
      clearInterval(id)
    }
  }, [])
  return now
}

function DomainRow({
  app,
  domain,
  enforced,
  detail,
  now,
  overdueSeconds,
}: {
  app: string
  domain: string
  enforced: boolean
  detail?: FleetMaintenanceEntry
  now: number
  overdueSeconds: number
}) {
  const { t } = useTranslation('maintenance')
  const queryClient = useQueryClient()
  const clear = useClearDomainMaintenance(app, domain)
  const elapsed = elapsedSince(detail?.started_at, now)
  const endNote = detail?.until
    ? t('domainBanner.endsAt', {
        date: new Date(detail.until).toLocaleString(),
      })
    : detail?.auto_off_at
      ? t('domainBanner.autoOff', {
          date: new Date(detail.auto_off_at).toLocaleString(),
        })
      : t('domainBanner.noEnd')
  const long = overdueSeconds > 0 && (elapsed ?? 0) > overdueSeconds
  return (
    <Banner
      variant="warning"
      title={
        enforced
          ? t('domainBanner.title', { domain })
          : t('domainBanner.titleNotEnforced', { domain })
      }
      action={
        <div className="flex items-center gap-2">
          <Button
            size="sm"
            variant="ghost"
            className="text-muted-foreground"
            render={<Link to="/apps/$name/domains" params={{ name: app }} />}
          >
            {t('domainBanner.manage')}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={clear.isPending}
            onClick={() => {
              clear.mutate(undefined, {
                onSuccess: () => {
                  void queryClient.invalidateQueries({
                    queryKey: domainKeys.all,
                  })
                  toast.add({
                    title: t('domainBanner.ended', { domain }),
                    type: 'success',
                  })
                },
                onError: (err) => {
                  toast.add({ title: err.message, type: 'error' })
                },
              })
            }}
          >
            {clear.isPending ? t('domainBanner.ending') : t('domainBanner.end')}
          </Button>
        </div>
      }
    >
      <p>
        {enforced ? t('domainBanner.body') : t('domainBanner.bodyNotEnforced')}
      </p>
      {detail ? (
        <p className="mt-1 text-xs">
          {elapsed !== null
            ? t('domainBanner.elapsed', { time: formatElapsed(elapsed) })
            : null}{' '}
          {endNote}
          {long ? ` ${t('domainBanner.long')}` : ''}
        </p>
      ) : null}
    </Banner>
  )
}

// Sticky for as long as any domain is in maintenance mode: it cannot be
// dismissed, so an operator never leaves a site down by accident.
export function DomainMaintenanceBanner() {
  const { t } = useTranslation('maintenance')
  const { data } = useQuery({
    ...domainsQueryOptions(),
    refetchInterval: 60_000,
  })
  const { data: fleet } = useQuery(fleetMaintenanceQueryOptions())
  const now = useNow()
  const active = (data ?? []).filter((d) => d.maintenance_enabled)
  if (active.length === 0) return null
  return (
    <div className="flex flex-col gap-2 px-4 pt-2 md:px-6">
      {active.slice(0, MAX_SHOWN).map((d) => (
        <DomainRow
          key={d.domain}
          app={d.service_name}
          domain={d.domain}
          enforced={isEnforced(d.enforcement, 'maintenance')}
          detail={fleet?.items.find((e) => e.domain === d.domain)}
          now={now}
          overdueSeconds={fleet?.attention_after_seconds ?? 0}
        />
      ))}
      {active.length > MAX_SHOWN ? (
        <p className="text-xs text-muted-foreground">
          {t('domainBanner.more', { count: active.length - MAX_SHOWN })}
        </p>
      ) : null}
    </div>
  )
}
