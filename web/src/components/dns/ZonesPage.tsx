import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import {
  GlobeHemisphereWestIcon,
  PlusIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { PageHeader } from '../shell/PageHeader'
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

  return (
    <div className="space-y-6">
      <PageHeader
        title={t('page.title')}
        description={t('page.description')}
        helpPath="/dns"
        helpLabel={t('page.help')}
        status={
          connected ? (
            <Badge variant="outline">
              {t('page.provider', { provider: data.provider })}
            </Badge>
          ) : null
        }
        actions={
          connected ? (
            <Button size="sm" onClick={() => setWizard(true)}>
              <PlusIcon />
              {t('page.addZone')}
            </Button>
          ) : null
        }
      />
      {!connected ? (
        <EmptyState
          icon={<GlobeHemisphereWestIcon className="size-5" />}
          title={t('empty.title')}
          description={t('empty.body')}
          action={
            <Button
              size="sm"
              render={<Link to="/domains" />}
              nativeButton={false}
            >
              {t('empty.action')}
            </Button>
          }
          helpPath="/dns"
          helpLabel={t('page.help')}
        />
      ) : data.zones.length === 0 ? (
        <EmptyState
          icon={<GlobeHemisphereWestIcon className="size-5" />}
          title={t('empty.noZonesTitle')}
          description={t('empty.noZonesBody')}
          action={
            <Button size="sm" onClick={() => setWizard(true)}>
              <PlusIcon />
              {t('page.addZone')}
            </Button>
          }
        />
      ) : (
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
