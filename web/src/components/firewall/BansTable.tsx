import { useMemo, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowDownIcon,
  ArrowUpIcon,
  CountdownGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { Input } from '@/components/ui/input'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { RelativeTime } from '@/components/kit'
import {
  useFirewallBans,
  useLiftFirewallBan,
  type FwBan,
} from '../../queries/firewallEdge'
import { formatCount } from './firewallFormat'
import { useFirewallLabels } from './useFirewallLabels'
import { VirtualRows } from './VirtualRows'

const COLS =
  'grid grid-cols-[1rem_minmax(0,1.2fr)_minmax(0,1fr)_4rem_minmax(0,1fr)_minmax(0,1fr)_auto] items-center gap-3 px-2 py-2 text-sm'

type SortKey = 'ip' | 'hits' | 'started' | 'ends'
type SortState = { key: SortKey; dir: 'asc' | 'desc' }

/** filterAndSortBans matches the query against IP and reason, then orders by one column. */
function filterAndSortBans(
  bans: FwBan[],
  query: string,
  sort: SortState,
): FwBan[] {
  const q = query.trim().toLowerCase()
  const kept = q
    ? bans.filter(
        (b) =>
          b.ip.toLowerCase().includes(q) || b.reason.toLowerCase().includes(q),
      )
    : bans
  const sign = sort.dir === 'asc' ? 1 : -1
  return [...kept].sort((a, b) => {
    const cmp =
      sort.key === 'hits'
        ? a.hits - b.hits
        : sort.key === 'ip'
          ? a.ip.localeCompare(b.ip, undefined, { numeric: true })
          : Date.parse(a[sort.key]) - Date.parse(b[sort.key])
    return cmp * sign
  })
}

function SortHeader({
  label,
  field,
  sort,
  onSort,
  className,
}: {
  label: string
  field: SortKey
  sort: SortState
  onSort: (key: SortKey) => void
  className?: string
}) {
  const active = sort.key === field
  const Arrow = sort.dir === 'asc' ? ArrowUpIcon : ArrowDownIcon
  return (
    <button
      type="button"
      onClick={() => onSort(field)}
      aria-label={label}
      className={`flex items-center gap-1 rounded-sm outline-none hover:text-foreground focus-visible:outline-2 focus-visible:outline-ring ${className ?? ''}`}
    >
      <span>{label}</span>
      {active ? <Arrow className="size-3" aria-hidden="true" /> : null}
    </button>
  )
}

function BanRow({
  ban,
  onLift,
  lifting,
  selected,
  onToggle,
}: {
  ban: FwBan
  onLift: () => void
  lifting: boolean
  selected: boolean
  onToggle: () => void
}) {
  const { t } = useTranslation('firewall')
  const { reason: reasons } = useFirewallLabels()
  return (
    <div className={COLS}>
      <Checkbox
        checked={selected}
        onCheckedChange={onToggle}
        aria-label={t('bans.select', { ip: ban.ip })}
      />
      <span className="truncate font-mono text-[13px]" title={ban.ip}>
        {ban.ip}
      </span>
      <span className="truncate text-muted-foreground">
        {reasons[ban.reason] ?? ban.reason}
      </span>
      <span className="text-right tabular-nums">{formatCount(ban.hits)}</span>
      <span className="truncate text-muted-foreground">
        <RelativeTime at={ban.started} />
      </span>
      <span className="truncate text-muted-foreground">
        <RelativeTime at={ban.ends} />
      </span>
      <Button
        type="button"
        variant="outline"
        size="xs"
        disabled={lifting}
        onClick={onLift}
        aria-label={t('bans.liftFor', { ip: ban.ip })}
      >
        {t('bans.lift')}
      </Button>
    </div>
  )
}

/** BansTable lists automatic temporary bans (persistent actions) with start, end and a lift button. */
export function BansTable({
  limit,
  retention,
}: {
  limit?: number
  retention?: string
}) {
  const { t } = useTranslation('firewall')
  const { data, error, isLoading, refetch } = useFirewallBans()
  const lift = useLiftFirewallBan()
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<SortState>({ key: 'started', dir: 'desc' })
  const [selected, setSelected] = useState<Set<string>>(new Set())
  const [bulkBusy, setBulkBusy] = useState(false)
  const all = useMemo(
    () => (limit ? (data ?? []).slice(0, limit) : (data ?? [])),
    [data, limit],
  )
  const rows = useMemo(
    () => filterAndSortBans(all, query, sort),
    [all, query, sort],
  )
  const chosen = rows.filter((b) => selected.has(b.ip))
  const allChosen = rows.length > 0 && chosen.length === rows.length

  function onSort(key: SortKey) {
    setSort((s) =>
      s.key === key
        ? { key, dir: s.dir === 'asc' ? 'desc' : 'asc' }
        : { key, dir: key === 'ip' ? 'asc' : 'desc' },
    )
  }

  function toggle(ip: string) {
    setSelected((cur) => {
      const next = new Set(cur)
      if (!next.delete(ip)) next.add(ip)
      return next
    })
  }

  function toggleAll() {
    setSelected(allChosen ? new Set() : new Set(rows.map((b) => b.ip)))
  }

  async function liftSelected() {
    const ips = chosen.map((b) => b.ip)
    setBulkBusy(true)
    const results = await Promise.allSettled(
      ips.map((ip) => lift.mutateAsync(ip)),
    )
    setBulkBusy(false)
    const failed = results.filter((r) => r.status === 'rejected').length
    setSelected(new Set())
    toast.add(
      failed === 0
        ? {
            title: t('bans.liftedMany', { count: ips.length }),
            type: 'success',
          }
        : { title: t('bans.liftManyFailed', { count: failed }), type: 'error' },
    )
  }

  function doLift(ip: string) {
    lift.mutate(ip, {
      onSuccess: () =>
        toast.add({ title: t('bans.lifted', { ip }), type: 'success' }),
      onError: (e) =>
        toast.add({
          title: t('actions.failed'),
          description: e.message,
          type: 'error',
        }),
    })
  }

  return (
    <Card className="min-w-0">
      <CardHeader className="border-b">
        <CardTitle>{t('bans.title')}</CardTitle>
        <CardDescription>{retention ?? t('bans.description')}</CardDescription>
      </CardHeader>
      <CardContent className="px-2">
        {isLoading ? (
          <div aria-hidden="true" className="space-y-2 px-2 py-2">
            {Array.from({ length: 3 }, (_, i) => (
              <Skeleton key={i} className="h-5 w-full" />
            ))}
          </div>
        ) : error ? (
          <EmptyState
            compact
            icon={<CountdownGlassIcon className="size-6" />}
            title={t('bans.loadFailed')}
            description={error.message}
            action={
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void refetch()}
              >
                {t('retry')}
              </Button>
            }
          />
        ) : all.length === 0 ? (
          <EmptyState
            compact
            icon={<CountdownGlassIcon className="size-6" />}
            title={t('bans.emptyTitle')}
            description={t('bans.emptyHelp')}
          />
        ) : (
          <>
            <div className="flex items-center gap-2 px-2 pb-2">
              <Input
                type="search"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder={t('bans.searchPlaceholder')}
                aria-label={t('bans.search')}
                className="h-8 max-w-64"
              />
              {chosen.length > 0 ? (
                <>
                  <span className="text-sm text-muted-foreground">
                    {t('bans.selected', { count: chosen.length })}
                  </span>
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    disabled={bulkBusy}
                    onClick={() => void liftSelected()}
                  >
                    {t('bans.liftSelected')}
                  </Button>
                </>
              ) : null}
            </div>
            <div className={`${COLS} text-xs text-muted-foreground`}>
              <Checkbox
                checked={allChosen}
                indeterminate={chosen.length > 0 && !allChosen}
                onCheckedChange={toggleAll}
                aria-label={t('bans.selectAll')}
              />
              <SortHeader
                label={t('bans.ip')}
                field="ip"
                sort={sort}
                onSort={onSort}
              />
              <span>{t('bans.reason')}</span>
              <SortHeader
                label={t('bans.requests')}
                field="hits"
                sort={sort}
                onSort={onSort}
                className="justify-end"
              />
              <SortHeader
                label={t('bans.started')}
                field="started"
                sort={sort}
                onSort={onSort}
              />
              <SortHeader
                label={t('bans.ends')}
                field="ends"
                sort={sort}
                onSort={onSort}
              />
              <span className="w-12" />
            </div>
            {rows.length === 0 ? (
              <p className="px-2 py-4 text-sm text-muted-foreground">
                {t('bans.noMatch')}
              </p>
            ) : null}
            <VirtualRows
              items={rows}
              rowHeight={40}
              label={t('bans.title')}
              getKey={(b) => b.ip}
              renderRow={(b) => (
                <BanRow
                  ban={b}
                  lifting={lift.isPending}
                  selected={selected.has(b.ip)}
                  onToggle={() => toggle(b.ip)}
                  onLift={() => doLift(b.ip)}
                />
              )}
            />
          </>
        )}
      </CardContent>
    </Card>
  )
}
