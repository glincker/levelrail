import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { PlusGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ZonesBanner } from './ZonesBanner'
import { dnsZonesQueryOptions } from '../../queries/dns'
import type { DnsZone } from '../../types/dns'
import { DelegationWizard } from './DelegationWizard'
import { VirtualRows } from './VirtualRows'

const GRID =
  'grid grid-cols-[minmax(0,2fr)_minmax(0,1fr)_minmax(0,1fr)_minmax(0,3fr)] items-center gap-3 px-4'

function ZoneRow({ zone }: { zone: DnsZone }) {
  const { t } = useTranslation('dns')
  return (
    <div
      className={`${GRID} h-12 border-b border-border text-sm last:border-b-0`}
    >
      <Link
        to="/dns/$zone"
        params={{ zone: zone.name }}
        className="truncate font-mono font-medium text-foreground hover:underline"
      >
        {zone.name}
      </Link>
      <span className="flex gap-1">
        <Badge variant={zone.status === 'active' ? 'success' : 'muted'}>
          {zone.status ?? t('zones.unknown')}
        </Badge>
        {zone.private ? (
          <Badge variant="outline">{t('zones.private')}</Badge>
        ) : null}
      </span>
      <span className="text-muted-foreground">
        {zone.record_count ?? t('zones.notReported')}
      </span>
      <span className="truncate font-mono text-xs text-muted-foreground">
        {zone.name_servers.join(', ')}
      </span>
    </div>
  )
}

/** ZonesPage lists every zone the connected provider can see, with the onboarding wizard. */
export function ZonesPage() {
  const { t } = useTranslation('dns')
  const { data } = useSuspenseQuery(dnsZonesQueryOptions())
  const [wizard, setWizard] = useState(false)
  const connected = data.provider !== 'none'

  const empty = !connected || data.zones.length === 0
  const addZone = (
    <Button size="sm" onClick={() => setWizard(true)}>
      <PlusGlassIcon size="1.2em" />
      {t('page.addZone')}
    </Button>
  )

  return (
    <div className="mx-auto flex w-full min-w-0 max-w-(--glinr-content-max) flex-col gap-(--glinr-page-gap)">
      <ZonesBanner
        empty={empty}
        helpLabel={t('page.help')}
        action={
          connected ? (
            <>
              <Badge variant="outline">
                {t('page.provider', { provider: data.provider })}
              </Badge>
              {addZone}
            </>
          ) : (
            <Button
              size="sm"
              render={<Link to="/domains" />}
              nativeButton={false}
            >
              {t('empty.action')}
            </Button>
          )
        }
      />
      {empty ? null : (
        <VirtualRows
          items={data.zones}
          rowHeight={48}
          getKey={(z) => z.id}
          renderRow={(z) => <ZoneRow zone={z} />}
          header={
            <div
              className={`${GRID} sticky top-0 z-10 border-b border-border bg-card py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
            >
              <span>{t('zones.col.name')}</span>
              <span>{t('zones.col.status')}</span>
              <span>{t('zones.col.records')}</span>
              <span>{t('zones.col.nameServers')}</span>
            </div>
          }
        />
      )}
      {wizard ? <DelegationWizard open onOpenChange={setWizard} /> : null}
    </div>
  )
}
