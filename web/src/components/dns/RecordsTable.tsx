import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  MagnifyingGlassIcon,
  PencilSimpleIcon,
  PlusIcon,
  TrashIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { filterRecords, RECORD_TYPES, recordId } from '../../lib/dnsRecords'
import type { DnsRecordSet } from '../../types/dns'
import { VirtualRows } from './VirtualRows'

const GRID =
  'grid grid-cols-[minmax(0,2fr)_4rem_4.5rem_minmax(0,4fr)_minmax(0,1.5fr)_4.5rem] items-center gap-3 px-4'
const ALL = 'all'

function RecordRow({
  record,
  onEdit,
  onDelete,
}: {
  record: DnsRecordSet
  onEdit: (r: DnsRecordSet) => void
  onDelete: (r: DnsRecordSet) => void
}) {
  const { t } = useTranslation('dns')
  const values = record.alias
    ? [t('records.alias', { target: record.alias.dns_name })]
    : record.values
  return (
    <div
      className={`${GRID} min-h-12 border-b border-border py-2 text-sm last:border-b-0`}
    >
      <span className="truncate font-mono" title={record.name}>
        {record.name}
      </span>
      <span className="font-mono">{record.type}</span>
      <span className="text-muted-foreground">
        {record.ttl === 1 ? t('records.auto') : record.ttl}
      </span>
      <span className="min-w-0 font-mono text-xs break-all">
        {values.map((v) => (
          <span key={v} className="block truncate" title={v}>
            {v}
          </span>
        ))}
      </span>
      <span className="flex flex-wrap gap-1">
        {record.proxied ? (
          <Badge variant="warning">{t('records.proxied')}</Badge>
        ) : null}
        {record.routing && record.routing !== 'simple' ? (
          <Badge variant="outline">
            {t('records.routed', {
              routing: record.routing,
              id: record.set_identifier ?? '',
            })}
          </Badge>
        ) : null}
        {record.managed ? (
          <Badge variant="muted">{t('records.managed')}</Badge>
        ) : null}
      </span>
      <span className="flex justify-end gap-1">
        {record.managed ? null : (
          <>
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={t('records.edit', {
                name: record.name,
                type: record.type,
              })}
              onClick={() => onEdit(record)}
            >
              <PencilSimpleIcon />
            </Button>
            <Button
              size="icon-sm"
              variant="ghost"
              aria-label={t('records.delete', {
                name: record.name,
                type: record.type,
              })}
              onClick={() => onDelete(record)}
            >
              <TrashIcon />
            </Button>
          </>
        )}
      </span>
    </div>
  )
}

/** RecordsTable searches and filters a zone's record sets; it virtualizes past 50 rows. */
export function RecordsTable({
  records,
  onAdd,
  onEdit,
  onDelete,
}: {
  records: DnsRecordSet[]
  onAdd: () => void
  onEdit: (r: DnsRecordSet) => void
  onDelete: (r: DnsRecordSet) => void
}) {
  const { t } = useTranslation('dns')
  const [query, setQuery] = useState('')
  const [type, setType] = useState(ALL)
  const visible = useMemo(
    () => filterRecords(records, type === ALL ? '' : type, query),
    [records, type, query],
  )

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-full max-w-xs">
          <MagnifyingGlassIcon
            className="pointer-events-none absolute top-1/2 left-2.5 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            className="pl-8"
            value={query}
            placeholder={t('records.search')}
            aria-label={t('records.search')}
            onChange={(e) => setQuery(e.target.value)}
          />
        </div>
        <Select value={type} onValueChange={(v) => setType(String(v))}>
          <SelectTrigger aria-label={t('records.typeFilter')} className="w-32">
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value={ALL}>{t('records.allTypes')}</SelectItem>
            {RECORD_TYPES.map((rt) => (
              <SelectItem key={rt} value={rt}>
                {rt}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
        <span className="text-xs text-muted-foreground">
          {t('records.count', { count: visible.length })}
        </span>
        <Button size="sm" className="ml-auto" onClick={onAdd}>
          <PlusIcon />
          {t('records.add')}
        </Button>
      </div>
      {visible.length === 0 ? (
        <EmptyState
          icon={<MagnifyingGlassIcon className="size-5" />}
          title={
            records.length === 0
              ? t('records.emptyTitle')
              : t('records.noMatch')
          }
          description={
            records.length === 0
              ? t('records.emptyBody')
              : t('records.noMatchBody')
          }
          action={
            records.length === 0 ? (
              <Button size="sm" onClick={onAdd}>
                <PlusIcon />
                {t('records.add')}
              </Button>
            ) : (
              <Button
                size="sm"
                variant="outline"
                onClick={() => {
                  setQuery('')
                  setType(ALL)
                }}
              >
                {t('records.clear')}
              </Button>
            )
          }
        />
      ) : (
        <VirtualRows
          items={visible}
          rowHeight={56}
          getKey={recordId}
          renderRow={(r) => (
            <RecordRow record={r} onEdit={onEdit} onDelete={onDelete} />
          )}
          header={
            <div
              className={`${GRID} sticky top-0 z-10 border-b border-border bg-card py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
            >
              <span>{t('records.col.name')}</span>
              <span>{t('records.col.type')}</span>
              <span>{t('records.col.ttl')}</span>
              <span>{t('records.col.values')}</span>
              <span>{t('records.col.flags')}</span>
              <span aria-hidden="true" />
            </div>
          }
        />
      )}
    </div>
  )
}
