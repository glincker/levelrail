import { useState } from 'react'
import { BugGlassIcon } from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { ListSkeleton } from '@/components/ui/list-skeleton'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { cn } from '@/lib/utils'
import { useErrorIssues, useErrorTracking } from '../../queries/errorTracking'
import type { ErrorFilters } from '../../queries/errorTracking'
import type { ErrorStatus } from '../../types/errorTracking'
import { ErrorIssueDetail } from './ErrorIssueDetail'
import { ErrorTrackingSetupCard } from './ErrorTrackingSetupCard'

const SETUP_ID = 'error-tracking-setup'
const STATUSES = ['all', 'open', 'resolved', 'ignored'] as const
const WINDOWS = ['1', '24', '168', '720'] as const

type BadgeVariant = 'destructive' | 'warning' | 'info' | 'muted'

function levelVariant(level: string): BadgeVariant {
  if (level === 'fatal' || level === 'error') return 'destructive'
  if (level === 'warning') return 'warning'
  if (level === 'info') return 'info'
  return 'muted'
}

function statusVariant(status: ErrorStatus): 'success' | 'muted' | 'outline' {
  if (status === 'resolved') return 'success'
  if (status === 'ignored') return 'muted'
  return 'outline'
}

function whenNs(ns: number): string {
  return new Date(ns / 1_000_000).toLocaleString()
}

/** ErrorsPanel is the app's Errors section: setup, filters, issues, detail. */
export function ErrorsPanel({ app }: { app: string }) {
  const { t } = useTranslation('errors')
  const [status, setStatus] = useState<ErrorFilters['status']>('open')
  const [windowHours, setWindowHours] = useState<string>('24')
  const [selected, setSelected] = useState<string | null>(null)
  const tracking = useErrorTracking(app)
  const trackingOff = tracking.data ? !tracking.data.enabled : false
  const list = useErrorIssues(app, {
    status,
    sinceHours: Number(windowHours),
  })

  return (
    <div className="space-y-4">
      <p className="text-sm text-muted-foreground">{t('subtitle')}</p>
      <div id={SETUP_ID}>
        <ErrorTrackingSetupCard app={app} />
      </div>

      <Card>
        <CardHeader className="gap-3">
          <CardTitle>{t('title')}</CardTitle>
          <div className="flex flex-wrap items-end gap-4">
            <label className="space-y-1 text-xs text-muted-foreground">
              {t('filters.status')}
              <Select
                value={status}
                onValueChange={(v) => {
                  if (STATUSES.some((s) => s === v)) {
                    setStatus(v as ErrorFilters['status'])
                  }
                }}
              >
                <SelectTrigger className="w-44">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {STATUSES.map((s) => (
                    <SelectItem key={s} value={s}>
                      {t(`filters.${s}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
            <label className="space-y-1 text-xs text-muted-foreground">
              {t('filters.window')}
              <Select
                value={windowHours}
                onValueChange={(v) => {
                  if (typeof v === 'string') setWindowHours(v)
                }}
              >
                <SelectTrigger className="w-44">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {WINDOWS.map((w) => (
                    <SelectItem key={w} value={w}>
                      {t(`filters.last${w}`)}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </label>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          {list.error ? (
            <Alert variant="destructive">
              <AlertDescription>
                {t('list.loadError', { message: list.error.message })}
              </AlertDescription>
            </Alert>
          ) : null}
          {list.isPending ? <ListSkeleton rows={5} /> : null}
          {list.data && list.data.length === 0 ? (
            <EmptyState
              compact
              icon={<BugGlassIcon className="size-4.5" />}
              title={t('list.emptyTitle')}
              description={trackingOff ? t('list.emptyOff') : t('list.empty')}
              action={
                trackingOff ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    onClick={() => {
                      document
                        .getElementById(SETUP_ID)
                        ?.scrollIntoView({ behavior: 'smooth' })
                    }}
                  >
                    {t('list.emptyAction')}
                  </Button>
                ) : undefined
              }
            />
          ) : null}
          {list.data && list.data.length > 0 ? (
            <ul className="divide-y divide-border">
              {list.data.map((issue) => (
                <li key={issue.fingerprint}>
                  <button
                    type="button"
                    onClick={() => {
                      setSelected((cur) =>
                        cur === issue.fingerprint ? null : issue.fingerprint,
                      )
                    }}
                    aria-expanded={selected === issue.fingerprint}
                    className={cn(
                      'grid w-full grid-cols-1 items-center gap-2 px-2 py-2 text-left text-sm outline-none hover:bg-muted/60 focus-visible:ring-2 focus-visible:ring-ring sm:grid-cols-[minmax(0,1fr)_auto] sm:gap-4',
                      selected === issue.fingerprint && 'bg-muted/60',
                    )}
                  >
                    <span className="min-w-0">
                      <span className="block truncate font-medium text-foreground">
                        {issue.title}
                      </span>
                      <span className="block text-xs text-muted-foreground">
                        {t('list.firstSeen')} {whenNs(issue.first_seen_ns)} ·{' '}
                        {t('list.lastSeen')} {whenNs(issue.last_seen_ns)}
                        {issue.last_release
                          ? ` · ${t('list.release', { release: issue.last_release })}`
                          : ''}
                        {issue.last_deploy_id
                          ? ` · ${t('list.deploy', { id: issue.last_deploy_id.slice(0, 8) })}`
                          : ''}
                      </span>
                    </span>
                    <span className="flex flex-wrap items-center gap-2 sm:justify-end">
                      <span className="text-xs tabular-nums text-muted-foreground">
                        {t('list.count', { count: issue.count })}
                      </span>
                      <Badge variant={levelVariant(issue.level)}>
                        {t(`level.${issue.level}`, {
                          defaultValue: issue.level,
                        })}
                      </Badge>
                      <Badge variant={statusVariant(issue.status)}>
                        {t(`filters.${issue.status}`)}
                      </Badge>
                      {issue.regressed ? (
                        <Badge variant="warning">{t('list.regressed')}</Badge>
                      ) : null}
                    </span>
                  </button>
                  {selected === issue.fingerprint ? (
                    <div className="px-2 pb-4 pt-2">
                      <ErrorIssueDetail
                        app={app}
                        fingerprint={issue.fingerprint}
                      />
                    </div>
                  ) : null}
                </li>
              ))}
            </ul>
          ) : null}
        </CardContent>
      </Card>
    </div>
  )
}
