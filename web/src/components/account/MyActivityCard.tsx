import { useMemo, useRef, useState } from 'react'
import { useInfiniteQuery } from '@tanstack/react-query'
import { useVirtualizer } from '@tanstack/react-virtual'
import { useTranslation } from 'react-i18next'
import {
  CircleUserGlassIcon,
  KeyGlassIcon,
  LogInGlassIcon,
  RestoreGlassIcon,
  ShieldCheckGlassIcon,
  TabletSmartphoneGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { EmptyState } from '@/components/ui/empty-state'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { SettingsCardSkeleton } from '../settings/SettingsSkeletons'
import {
  activityQueryOptions,
  type ActivityEntry,
  type ActivityKind,
} from '../../queries/accountSelf'
import { formatUserDateTime, useUserTimezone } from '../../lib/userPreferences'

const FILTERS = [
  'important',
  'all',
  'sign_in',
  'security',
  'sessions',
  'tokens',
  'token_use',
  'account',
  'failed',
] as const

type Filter = (typeof FILTERS)[number]

function isFilter(v: unknown): v is Filter {
  return typeof v === 'string' && (FILTERS as readonly string[]).includes(v)
}

const ROW_HEIGHT = 56

function KindIcon({ kind }: { kind: ActivityKind }) {
  const cls = 'size-4 text-muted-foreground'
  switch (kind) {
    case 'sign_in':
      return <LogInGlassIcon size="1.4em" className={cls} aria-hidden="true" />
    case 'security':
      return (
        <ShieldCheckGlassIcon size="1.4em" className={cls} aria-hidden="true" />
      )
    case 'sessions':
      return (
        <TabletSmartphoneGlassIcon
          size="1.4em"
          className={cls}
          aria-hidden="true"
        />
      )
    case 'tokens':
    case 'token_use':
      return <KeyGlassIcon size="1.4em" className={cls} aria-hidden="true" />
    default:
      return (
        <CircleUserGlassIcon size="1.4em" className={cls} aria-hidden="true" />
      )
  }
}

function eventKey(event: string): string {
  return event.replace(/[^A-Za-z0-9]+/g, '_')
}

function ActivityRow({ entry }: { entry: ActivityEntry }) {
  const { t, i18n } = useTranslation('account')
  const zone = useUserTimezone()
  const key = `activity.events.${eventKey(entry.event)}`
  const label = i18n.exists(key, { ns: 'account' })
    ? t(key as 'activity.events.account_sign_in')
    : t('activity.request', { method: entry.method, path: entry.path })
  const failed = entry.status_code >= 400
  return (
    <div className="flex h-full items-center gap-3 px-4">
      <KindIcon kind={entry.kind} />
      <div className="min-w-0 flex-1">
        <p className="truncate text-sm font-medium">
          {label}
          {entry.actor_type === 'token' && entry.token_name ? (
            <span className="ml-2 font-normal text-muted-foreground">
              {t('activity.viaToken', { name: entry.token_name })}
            </span>
          ) : null}
        </p>
        <p className="truncate text-xs text-muted-foreground">
          <time dateTime={entry.at}>
            {formatUserDateTime(entry.at, '', zone)}
          </time>
          {entry.remote_addr
            ? ` · ${t('activity.from', { address: entry.remote_addr })}`
            : ''}
        </p>
      </div>
      {failed ? (
        <Badge variant="destructive">{t('activity.failed')}</Badge>
      ) : null}
    </div>
  )
}

function ActivityList({ rows }: { rows: ActivityEntry[] }) {
  const { t } = useTranslation('account')
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: rows.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 8,
  })
  return (
    <div
      ref={parentRef}
      role="list"
      aria-label={t('activity.listLabel')}
      className="max-h-[28rem] overflow-auto rounded-lg border border-border"
    >
      <div
        className="relative w-full"
        style={{ height: virtualizer.getTotalSize() }}
      >
        {virtualizer.getVirtualItems().map((v) => {
          const entry = rows[v.index]
          if (!entry) return null
          return (
            <div
              key={entry.id}
              role="listitem"
              className="absolute top-0 left-0 w-full border-b border-border last:border-b-0"
              style={{ height: v.size, transform: `translateY(${v.start}px)` }}
            >
              <ActivityRow entry={entry} />
            </div>
          )
        })}
      </div>
    </div>
  )
}

export function MyActivityCard() {
  const { t } = useTranslation('account')
  const [filter, setFilter] = useState<Filter>('important')
  const query = useInfiniteQuery(activityQueryOptions(filter))
  const rows = useMemo(
    () => query.data?.pages.flatMap((p) => p.entries) ?? [],
    [query.data],
  )
  const filterLabel = (f: Filter) => t(`activity.filters.${f}`)

  if (query.isPending) return <SettingsCardSkeleton rows={3} />

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('activity.title')}</CardTitle>
        <CardDescription>{t('activity.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-3">
          <Label htmlFor="activity-filter">{t('activity.filterLabel')}</Label>
          <Select
            value={filter}
            onValueChange={(v) => setFilter(isFilter(v) ? v : 'important')}
          >
            <SelectTrigger id="activity-filter" className="w-56">
              <SelectValue>
                {(v: string) => (isFilter(v) ? filterLabel(v) : v)}
              </SelectValue>
            </SelectTrigger>
            <SelectContent>
              {FILTERS.map((f) => (
                <SelectItem key={f} value={f}>
                  {filterLabel(f)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        {query.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.4em"
            />
            <AlertDescription className="flex items-center justify-between gap-3">
              <span>{t('common.loadFailed')}</span>
              <Button
                type="button"
                size="sm"
                variant="outline"
                onClick={() => void query.refetch()}
              >
                {t('common.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : rows.length === 0 ? (
          <EmptyState
            compact
            icon={<RestoreGlassIcon className="size-6" />}
            title={
              filter === 'all'
                ? t('activity.emptyTitle')
                : t('activity.emptyFilteredTitle')
            }
            description={
              filter === 'all'
                ? t('activity.emptyDescription')
                : t('activity.emptyFilteredDescription')
            }
            action={
              filter === 'all' ? (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => void query.refetch()}
                >
                  {t('activity.refresh')}
                </Button>
              ) : (
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  onClick={() => setFilter('all')}
                >
                  {t('activity.showAll')}
                </Button>
              )
            }
          />
        ) : (
          <>
            <ActivityList rows={rows} />
            {query.hasNextPage ? (
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={query.isFetchingNextPage}
                onClick={() => void query.fetchNextPage()}
              >
                {query.isFetchingNextPage
                  ? t('activity.loadingMore')
                  : t('activity.loadMore')}
              </Button>
            ) : null}
          </>
        )}
      </CardContent>
    </Card>
  )
}
