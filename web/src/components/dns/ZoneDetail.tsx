import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import {
  ArrowsDownUpIcon,
  ListPlusIcon,
  SignpostIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { PageHeader } from '../shell/PageHeader'
import {
  dnsRecordsQueryOptions,
  dnsZoneQueryOptions,
  useDnsDelegation,
} from '../../queries/dns'
import type { DnsRecordSet } from '../../types/dns'
import { DeleteRecordDialog, DeleteZoneDialog } from './ConfirmDialogs'
import { DelegationBadge, DelegationPanel } from './DelegationPanel'
import { DelegationWizard } from './DelegationWizard'
import { HealthChecksPanel } from './HealthChecksPanel'
import { ImportExportDialog } from './ImportExportDialog'
import { PropagationChecker } from './PropagationChecker'
import { RecordDialog } from './RecordDialog'
import { RecordsTable } from './RecordsTable'
import { TemplateDialog } from './TemplateDialog'

type Modal =
  | { kind: 'none' }
  | { kind: 'record'; record?: DnsRecordSet }
  | { kind: 'deleteRecord'; record: DnsRecordSet }
  | { kind: 'import' }
  | { kind: 'template' }
  | { kind: 'wizard' }
  | { kind: 'deleteZone' }

/** ZoneDetail is one zone: overview, records, delegation, propagation and health checks. */
export function ZoneDetail({ zoneRef }: { zoneRef: string }) {
  const { t } = useTranslation('dns')
  const { data: ov } = useSuspenseQuery(dnsZoneQueryOptions(zoneRef))
  const { data: recs } = useSuspenseQuery(dnsRecordsQueryOptions(zoneRef))
  const delegation = useDnsDelegation(zoneRef, !ov.zone.private)
  const [modal, setModal] = useState<Modal>({ kind: 'none' })
  const close = () => setModal({ kind: 'none' })
  const zone = ov.zone
  const userRecords = recs.records.filter((r) => !r.managed).length
  const counts = Object.entries(ov.counts).sort(([a], [b]) =>
    a.localeCompare(b),
  )

  return (
    <div className="space-y-6">
      <PageHeader
        breadcrumb={
          <Link
            to="/dns"
            className="text-sm text-muted-foreground hover:underline"
          >
            {t('page.title')}
          </Link>
        }
        title={<span className="font-mono">{zone.name}</span>}
        status={
          delegation.data ? (
            <DelegationBadge state={delegation.data.delegation.state} />
          ) : null
        }
        actions={
          <>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setModal({ kind: 'wizard' })}
            >
              <SignpostIcon />
              {t('zone.nameservers')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setModal({ kind: 'template' })}
            >
              <ListPlusIcon />
              {t('zone.templates')}
            </Button>
            <Button
              size="sm"
              variant="outline"
              onClick={() => setModal({ kind: 'import' })}
            >
              <ArrowsDownUpIcon />
              {t('zone.importExport')}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              aria-label={t('zone.delete')}
              onClick={() => setModal({ kind: 'deleteZone' })}
            >
              <TrashIcon />
            </Button>
          </>
        }
      />
      <div className="grid gap-4 md:grid-cols-3">
        <Card>
          <CardHeader>
            <CardTitle>{t('zone.recordsByType')}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-wrap gap-1.5">
            {counts.map(([type, n]) => (
              <Badge key={type} variant="outline">
                {type} {n}
              </Badge>
            ))}
            <span className="text-sm text-muted-foreground">
              {t('zone.total', { count: ov.total })}
            </span>
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t('zone.lastChanged')}</CardTitle>
          </CardHeader>
          <CardContent className="text-sm">
            {ov.last_changed ? (
              <>
                <p>{new Date(ov.last_changed).toLocaleString()}</p>
                <p className="text-muted-foreground">
                  {ov.last_changed_by
                    ? t('zone.by', {
                        who: ov.last_changed_by,
                        action: ov.last_action ?? '',
                      })
                    : null}
                </p>
              </>
            ) : (
              <p className="text-muted-foreground">{t('zone.noChanges')}</p>
            )}
          </CardContent>
        </Card>
        <Card>
          <CardHeader>
            <CardTitle>{t('zone.nameservers')}</CardTitle>
          </CardHeader>
          <CardContent className="space-y-1 font-mono text-xs">
            {zone.name_servers.map((ns) => (
              <p key={ns}>{ns}</p>
            ))}
            <p className="font-sans text-muted-foreground">
              {t('zone.provider', {
                provider: recs.provider,
                status: zone.status ?? '',
              })}
            </p>
          </CardContent>
        </Card>
      </div>
      <Tabs defaultValue="records">
        <TabsList>
          <TabsTrigger value="records">{t('zone.tab.records')}</TabsTrigger>
          {zone.private ? null : (
            <TabsTrigger value="delegation">
              {t('zone.tab.delegation')}
            </TabsTrigger>
          )}
          <TabsTrigger value="propagation">
            {t('zone.tab.propagation')}
          </TabsTrigger>
          {recs.capabilities.health_checks ? (
            <TabsTrigger value="health">{t('zone.tab.health')}</TabsTrigger>
          ) : null}
        </TabsList>
        <TabsContent value="records">
          <RecordsTable
            records={recs.records}
            onAdd={() => setModal({ kind: 'record' })}
            onEdit={(record) => setModal({ kind: 'record', record })}
            onDelete={(record) => setModal({ kind: 'deleteRecord', record })}
          />
        </TabsContent>
        <TabsContent value="delegation">
          <DelegationPanel zone={zoneRef} />
        </TabsContent>
        <TabsContent value="propagation">
          <PropagationChecker zone={zone.id} zoneName={zone.name} />
        </TabsContent>
        <TabsContent value="health">
          <HealthChecksPanel />
        </TabsContent>
      </Tabs>

      {modal.kind === 'record' ? (
        <RecordDialog
          zone={zoneRef}
          zoneName={zone.name}
          caps={recs.capabilities}
          record={modal.record}
          onClose={close}
        />
      ) : null}
      {modal.kind === 'deleteRecord' ? (
        <DeleteRecordDialog
          zone={zoneRef}
          record={modal.record}
          onClose={close}
        />
      ) : null}
      {modal.kind === 'import' ? (
        <ImportExportDialog
          zone={zoneRef}
          zoneName={zone.name}
          onClose={close}
        />
      ) : null}
      {modal.kind === 'template' ? (
        <TemplateDialog zone={zoneRef} onClose={close} />
      ) : null}
      {modal.kind === 'wizard' ? (
        <DelegationWizard
          open
          existing={zone}
          onOpenChange={(o) => (o ? undefined : close())}
        />
      ) : null}
      {modal.kind === 'deleteZone' ? (
        <DeleteZoneDialog
          zone={zoneRef}
          zoneName={zone.name}
          recordCount={userRecords}
          onClose={close}
        />
      ) : null}
    </div>
  )
}
