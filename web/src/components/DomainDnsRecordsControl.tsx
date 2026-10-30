import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  CircleAlertGlassIcon,
  GlobeGlassIcon,
  PlusGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Table,
  TableBody,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { ApiError } from '../lib/apiError'
import {
  useCreateDnsRecord,
  useDeleteDnsRecord,
  useDnsRecords,
  useUpdateDnsRecord,
} from '../queries/dnsRecords'
import { RecordFields, RecordRow } from './DnsRecordRow'
import {
  EMPTY_DRAFT,
  recordKey,
  toRecord,
  type DraftRecord,
} from '../lib/dnsRecordDraft'

// A per-domain DNS records view: lists the whole best-effort zone's
// A/AAAA/CNAME/TXT/MX/SRV/CAA records (not just this one domain's own
// host) from whichever ACME DNS-01 provider is already configured, with
// add/edit/delete and a live resolved/pending/mismatch check per record.
// Collapsed by default, the same pattern DomainWafControl establishes
// for another per-domain panel that shouldn't dominate DomainEditor's
// list when a domain has nothing unusual configured.
export function DomainDnsRecordsControl({
  appName,
  domain,
}: {
  appName: string
  domain: string
}) {
  const { t } = useTranslation('dns')
  const [open, setOpen] = useState(false)
  const { data, isLoading, isError, error } = useDnsRecords(appName, domain)
  const createRecord = useCreateDnsRecord(appName, domain)
  const updateRecord = useUpdateDnsRecord(appName, domain)
  const deleteRecord = useDeleteDnsRecord(appName, domain)
  const [editingKey, setEditingKey] = useState<string | null>(null)
  const [addDraft, setAddDraft] = useState<DraftRecord>(EMPTY_DRAFT)
  const [adding, setAdding] = useState(false)

  const records = data?.records ?? []
  const notConfigured =
    isError && error instanceof ApiError && error.status === 501

  const parentRef = useMemo(
    () => ({ current: null as HTMLDivElement | null }),
    [],
  )
  const virtualizer = useVirtualizer({
    count: records.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => 44,
    overscan: 10,
  })

  if (!open) {
    return (
      <div className="flex flex-wrap items-center justify-between gap-2 rounded-md border border-border bg-muted/30 p-3 text-sm">
        <button
          type="button"
          onClick={() => setOpen(true)}
          aria-expanded={false}
          className="flex items-center gap-1.5 text-left"
        >
          <Badge variant="muted" className="shrink-0">
            <GlobeGlassIcon className="size-4.5" aria-hidden="true" />
            {isLoading
              ? t('recordsControl.title')
              : notConfigured
                ? t('recordsControl.titleNoProvider')
                : t('recordsControl.count', { count: records.length })}
          </Badge>
        </button>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-expanded={false}
          onClick={() => setOpen(true)}
        >
          <GlobeGlassIcon className="size-5" aria-hidden="true" />
          {t('recordsControl.manage')}
        </Button>
      </div>
    )
  }

  return (
    <div className="space-y-3 rounded-md border border-border bg-muted/30 p-3 text-sm">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <span className="flex items-center gap-1.5 font-medium text-foreground">
          <GlobeGlassIcon className="size-5" aria-hidden="true" />
          {data?.zone
            ? t('recordsControl.titleForZone', {
                zone: data.zone.replace(/\.$/, ''),
              })
            : t('recordsControl.title')}
        </span>
        <Button
          type="button"
          variant="ghost"
          size="sm"
          aria-expanded={true}
          onClick={() => setOpen(false)}
        >
          {t('recordsControl.hide')}
        </Button>
      </div>

      {isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : notConfigured ? (
        <Alert>
          <CircleAlertGlassIcon
            className="size-5 text-tone-danger"
            aria-hidden="true"
          />
          <AlertDescription>{t('recordsControl.noProvider')}</AlertDescription>
        </Alert>
      ) : isError ? (
        <Alert variant="destructive">
          <AlertDescription>{error?.message}</AlertDescription>
        </Alert>
      ) : (
        <>
          {records.length === 0 ? (
            <p className="text-xs text-muted-foreground">
              {t('recordsControl.noRecords')}
            </p>
          ) : (
            <div
              ref={(el) => {
                parentRef.current = el
              }}
              className="max-h-80 overflow-auto rounded-md border border-border"
            >
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>{t('recordsControl.colType')}</TableHead>
                    <TableHead>{t('recordsControl.colName')}</TableHead>
                    <TableHead>{t('recordsControl.colValue')}</TableHead>
                    <TableHead>{t('recordsControl.colTtl')}</TableHead>
                    <TableHead className="text-right">
                      {t('recordsControl.colStatus')}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody
                  role="rowgroup"
                  style={{
                    display: 'grid',
                    height: virtualizer.getTotalSize(),
                  }}
                >
                  {virtualizer.getVirtualItems().map((row) => {
                    const record = records[row.index]
                    if (!record) {
                      return null
                    }
                    const key = recordKey(record)
                    return (
                      <div
                        key={key}
                        role="presentation"
                        style={{
                          display: 'grid',
                          position: 'absolute',
                          top: 0,
                          left: 0,
                          width: '100%',
                          transform: `translateY(${row.start}px)`,
                        }}
                      >
                        <RecordRow
                          record={record}
                          editing={editingKey === key}
                          saving={updateRecord.isPending}
                          deleting={deleteRecord.isPending}
                          onStartEdit={() => setEditingKey(key)}
                          onCancelEdit={() => setEditingKey(null)}
                          onSave={(draft) => {
                            updateRecord.mutate(
                              { original: record, record: toRecord(draft) },
                              { onSuccess: () => setEditingKey(null) },
                            )
                          }}
                          onDelete={() => {
                            deleteRecord.mutate(record)
                          }}
                        />
                      </div>
                    )
                  })}
                </TableBody>
              </Table>
            </div>
          )}

          <div className="space-y-2 border-t border-border pt-3">
            <FieldLabel className="text-xs">
              {t('recordsControl.addRecord')}
            </FieldLabel>
            <Field orientation="horizontal" className="items-start">
              <RecordFields
                draft={addDraft}
                onChange={setAddDraft}
                idPrefix="add"
              />
              <Button
                type="button"
                size="icon-sm"
                disabled={
                  adding ||
                  createRecord.isPending ||
                  addDraft.value.trim().length === 0
                }
                onClick={() => {
                  setAdding(true)
                  createRecord.mutate(toRecord(addDraft), {
                    onSuccess: () => {
                      setAddDraft(EMPTY_DRAFT)
                    },
                    onSettled: () => setAdding(false),
                  })
                }}
              >
                <PlusGlassIcon size="1.4em" aria-hidden="true" />
                <span className="sr-only">{t('recordsControl.addRecord')}</span>
              </Button>
            </Field>
            {createRecord.isError ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {createRecord.error.message}
                </AlertDescription>
              </Alert>
            ) : null}
            {updateRecord.isError ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {updateRecord.error.message}
                </AlertDescription>
              </Alert>
            ) : null}
            {deleteRecord.isError ? (
              <Alert variant="destructive">
                <AlertDescription>
                  {deleteRecord.error.message}
                </AlertDescription>
              </Alert>
            ) : null}
          </div>
        </>
      )}
    </div>
  )
}
