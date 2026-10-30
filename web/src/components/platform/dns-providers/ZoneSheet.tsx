import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Sheet,
  SheetContent,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { DnsHostTile } from '@/components/dns/DnsHostChip'
import { RecordDialog } from '@/components/dns/RecordDialog'
import { useCopy } from '@/components/dns/useCopy'
import { dnsZoneDetailQueryOptions } from '@/queries/dns'
import { domainsQueryOptions } from '@/queries/domains'
import type { DnsRecordSet, DnsZone } from '@/types/dns'
import { RecordDeleteDialog } from './RecordDeleteDialog'
import { ZoneDelegationBanner } from './ZoneDelegationBanner'
import { ZoneRecords } from './ZoneRecords'
import { hostKey } from './providers'

type Modal =
  | { kind: 'none' }
  | { kind: 'record'; record?: DnsRecordSet }
  | { kind: 'delete'; record: DnsRecordSet }

function NameServers({ zone }: { zone: DnsZone }) {
  const { t } = useTranslation('dnsProviders')
  const { copied, copy } = useCopy()
  const detail = useQuery({
    ...dnsZoneDetailQueryOptions(zone.provider, zone.id),
  })
  const d = detail.data
  if (detail.error)
    return (
      <p role="alert" className="text-sm text-tone-danger">
        {t('zone.detailFailed')}
      </p>
    )
  if (!d) return <p className="text-sm text-muted-foreground">...</p>
  const assigned = d.nameservers ?? d.name_servers ?? []
  const current = d.current_nameservers ?? []
  return (
    <div className="space-y-4">
      <ZoneDelegationBanner
        delegated={d.delegated ?? 'unknown'}
        reason={d.delegation_reason}
        showSteps
        onRecheck={() => void detail.refetch()}
        checking={detail.isFetching}
      />
      {d.registrar_hint ? (
        <p className="flex flex-wrap items-center gap-2 text-sm">
          <span className="font-medium">{t('zone.registrar')}</span>
          <span>
            {t('zone.registrarHint', { name: d.registrar_hint.name })}
          </span>
          {d.registrar_hint.url ? (
            <a
              href={d.registrar_hint.url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-primary underline"
            >
              {t('zone.registrarOpen', { name: d.registrar_hint.name })}
            </a>
          ) : null}
        </p>
      ) : null}
      <section className="space-y-2">
        <div className="flex items-center justify-between gap-2">
          <h3 className="text-sm font-medium">{t('zone.assigned')}</h3>
          {assigned.length > 0 ? (
            <Button
              size="sm"
              variant="outline"
              onClick={() => copy('all', assigned.join('\n'))}
            >
              <CopyGlassIcon size="1.4em" />
              {copied === 'all' ? t('zone.copied') : t('zone.copyAll')}
            </Button>
          ) : null}
        </div>
        {assigned.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('zone.noServers')}</p>
        ) : (
          <ul className="divide-y divide-border rounded-md border border-border">
            {assigned.map((ns) => (
              <li
                key={ns}
                className="flex items-center justify-between gap-2 px-3 py-1.5"
              >
                <span className="truncate font-mono text-sm">{ns}</span>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={`${t('zone.copy')} ${ns}`}
                  onClick={() => copy(ns, ns)}
                >
                  <CopyGlassIcon size="1.4em" />
                  {copied === ns ? t('zone.copied') : null}
                </Button>
              </li>
            ))}
          </ul>
        )}
      </section>
      {zone.private ? null : (
        <section className="space-y-1">
          <h3 className="text-sm font-medium">{t('zone.current')}</h3>
          <p className="font-mono text-sm text-muted-foreground">
            {current.length > 0 ? current.join(', ') : t('zone.none')}
          </p>
        </section>
      )}
    </div>
  )
}

function UsedBy({ zone }: { zone: DnsZone }) {
  const { t } = useTranslation('dnsProviders')
  const domains = useQuery(domainsQueryOptions())
  const rows = (domains.data ?? []).filter(
    (d) => d.domain === zone.name || d.domain.endsWith(`.${zone.name}`),
  )
  if (rows.length === 0)
    return <p className="text-sm text-muted-foreground">{t('zone.usedNone')}</p>
  return (
    <ul className="divide-y divide-border rounded-md border border-border text-sm">
      {rows.map((d) => (
        <li key={d.domain} className="flex justify-between gap-2 px-3 py-2">
          <span className="truncate font-mono">{d.domain}</span>
          {d.managed_by_stack ? (
            <span className="text-muted-foreground">
              {t('zone.usedStack', { stack: d.managed_by_stack })}
            </span>
          ) : null}
        </li>
      ))}
    </ul>
  )
}

export function ZoneSheet({
  zone,
  onClose,
}: {
  zone: DnsZone | null
  onClose: () => void
}) {
  const { t } = useTranslation('dnsProviders')
  const detail = useQuery({
    ...dnsZoneDetailQueryOptions(zone?.provider ?? '', zone?.id ?? ''),
    enabled: zone !== null,
  })
  const caps = detail.data?.capabilities
  const [modal, setModal] = useState<Modal>({ kind: 'none' })
  const close = () => setModal({ kind: 'none' })
  return (
    <>
      <Sheet open={zone !== null} onOpenChange={(o) => !o && onClose()}>
        <SheetContent className="overflow-y-auto p-4 sm:max-w-3xl">
          {zone ? (
            <>
              <SheetHeader className="p-0">
                <SheetTitle className="flex items-center gap-2 font-mono">
                  <DnsHostTile provider={hostKey(zone.provider)} />
                  {zone.name}
                </SheetTitle>
              </SheetHeader>
              <Tabs defaultValue="records">
                <TabsList>
                  <TabsTrigger value="records">
                    {t('zone.tabRecords')}
                  </TabsTrigger>
                  <TabsTrigger value="ns">
                    {t('zone.tabNameServers')}
                  </TabsTrigger>
                  <TabsTrigger value="used">{t('zone.tabUsedBy')}</TabsTrigger>
                </TabsList>
                <TabsContent value="records">
                  <ZoneRecords
                    zone={zone.name}
                    provider={zone.provider}
                    canWrite={caps !== undefined}
                    onAdd={() => setModal({ kind: 'record' })}
                    onEdit={(record) => setModal({ kind: 'record', record })}
                    onDelete={(record) => setModal({ kind: 'delete', record })}
                  />
                </TabsContent>
                <TabsContent value="ns">
                  <NameServers zone={zone} />
                </TabsContent>
                <TabsContent value="used">
                  <UsedBy zone={zone} />
                </TabsContent>
              </Tabs>
            </>
          ) : null}
        </SheetContent>
      </Sheet>
      {zone && caps && modal.kind === 'record' ? (
        <RecordDialog
          zone={zone.name}
          zoneName={zone.name}
          caps={caps}
          record={modal.record}
          onClose={close}
        />
      ) : null}
      {zone && modal.kind === 'delete' ? (
        <RecordDeleteDialog
          zone={zone.name}
          record={modal.record}
          onClose={close}
        />
      ) : null}
    </>
  )
}
