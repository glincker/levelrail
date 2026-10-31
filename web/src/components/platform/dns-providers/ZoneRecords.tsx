import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  LockGlassIcon,
  PencilGlassIcon,
  PlusGlassIcon,
  TrashGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Skeleton } from '@/components/ui/skeleton'
import { RECORD_TYPES } from '@/lib/dnsRecords'
import { useDnsZoneRecords } from '@/queries/dns'
import type { DnsRecordSet } from '@/types/dns'
import { RECORD_SEARCH_DEBOUNCE_MS } from './providers'

const GRID =
  'grid grid-cols-[minmax(0,2fr)_3.5rem_minmax(0,3fr)_auto] items-center gap-3 px-3 sm:grid-cols-[minmax(0,2fr)_3.5rem_4rem_minmax(0,3fr)_auto]'

export function ZoneRecords({
  zone,
  provider,
  canWrite,
  onAdd,
  onEdit,
  onDelete,
}: {
  zone: string
  provider: string
  canWrite: boolean
  onAdd: () => void
  onEdit: (r: DnsRecordSet) => void
  onDelete: (r: DnsRecordSet) => void
}) {
  const { t } = useTranslation('dnsProviders')
  const [type, setType] = useState('')
  const [input, setInput] = useState('')
  const [search, setSearch] = useState('')
  useEffect(() => {
    const id = setTimeout(
      () => setSearch(input.trim()),
      RECORD_SEARCH_DEBOUNCE_MS,
    )
    return () => clearTimeout(id)
  }, [input])
  const q = useDnsZoneRecords(zone, { provider, type, search }, true)
  const pages = q.data?.pages ?? []
  const rows = pages.flatMap((p) => p.records)
  const matched = pages[0]?.matched ?? pages[0]?.total ?? 0

  return (
    <div className="space-y-3">
      <div className="flex flex-wrap items-center gap-2">
        <Input
          type="search"
          className="w-full max-w-xs"
          value={input}
          placeholder={t('records.search')}
          aria-label={t('records.search')}
          onChange={(e) => setInput(e.target.value)}
        />
        <select
          aria-label={t('records.type')}
          className="h-8 rounded-md border border-input bg-background px-2 text-sm"
          value={type}
          onChange={(e) => setType(e.target.value)}
        >
          <option value="">{t('records.all')}</option>
          {RECORD_TYPES.map((rt) => (
            <option key={rt} value={rt}>
              {rt}
            </option>
          ))}
        </select>
        {pages.length > 0 ? (
          <span className="text-xs text-muted-foreground">
            {t('records.count', { shown: rows.length, matched })}
          </span>
        ) : null}
        {canWrite ? (
          <Button size="sm" className="ml-auto" onClick={onAdd}>
            <PlusGlassIcon size="1.4em" />
            {t('records.add')}
          </Button>
        ) : null}
      </div>
      {q.error ? (
        <p role="alert" className="text-sm text-tone-danger">
          {q.error.message}
        </p>
      ) : q.isPending ? (
        <Skeleton className="h-32 w-full" />
      ) : rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('records.empty')}</p>
      ) : (
        <div className="rounded-lg border border-border text-sm">
          {rows.map((r) => (
            <div
              key={`${r.name}|${r.type}|${r.set_identifier ?? ''}`}
              className={`${GRID} min-h-11 border-b border-border py-2 last:border-b-0`}
            >
              <span className="truncate font-mono" title={r.name}>
                {r.name}
              </span>
              <span className="font-mono">{r.type}</span>
              <span className="hidden text-muted-foreground sm:block">
                {r.ttl === 1 ? t('records.auto') : r.ttl}
              </span>
              <span className="min-w-0 font-mono text-xs">
                {(r.alias
                  ? [t('records.alias', { target: r.alias.dns_name })]
                  : r.values
                ).map((v) => (
                  <span key={v} className="block truncate" title={v}>
                    {v}
                  </span>
                ))}
              </span>
              <span className="flex items-center justify-end gap-1">
                {r.managed_by_levelrail ? (
                  <Badge variant="info" title={t('records.levelrailTip')}>
                    {t('records.levelrail')}
                  </Badge>
                ) : null}
                {r.read_only ? (
                  <LockGlassIcon
                    size="1.4em"
                    role="img"
                    aria-label={t('records.readOnly')}
                    className="text-muted-foreground"
                  />
                ) : canWrite ? (
                  <>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={t('records.edit', {
                        name: r.name,
                        type: r.type,
                      })}
                      onClick={() => onEdit(r)}
                    >
                      <PencilGlassIcon size="1.4em" />
                    </Button>
                    <Button
                      size="icon-sm"
                      variant="ghost"
                      aria-label={t('records.delete', {
                        name: r.name,
                        type: r.type,
                      })}
                      onClick={() => onDelete(r)}
                    >
                      <TrashGlassIcon size="1.4em" />
                    </Button>
                  </>
                ) : null}
              </span>
            </div>
          ))}
        </div>
      )}
      {q.hasNextPage ? (
        <Button
          size="sm"
          variant="outline"
          disabled={q.isFetchingNextPage}
          onClick={() => void q.fetchNextPage()}
        >
          {t('records.more')}
        </Button>
      ) : null}
    </div>
  )
}
