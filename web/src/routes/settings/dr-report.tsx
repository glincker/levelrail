import { useState } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  ClipboardListGlassIcon,
  PlayGlassIcon,
  RotateCcwGlassIcon,
} from '@thesvg/ui-react'
import { PageHeader } from '@/components/shell/PageHeader'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Banner } from '@/components/ui/banner'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import {
  useRehydratePlan,
  useRehydrateReport,
  useRehydrateRun,
  type RehydrateItem,
  type RehydrateReport,
} from '../../queries/rehydrate'

export const Route = createFileRoute('/settings/dr-report')({
  component: DrReportPage,
})

const SECONDS_PER_DAY = 86_400
const SECONDS_PER_HOUR = 3_600

function formatDuration(seconds: number): string {
  if (seconds >= SECONDS_PER_DAY)
    return `${Math.floor(seconds / SECONDS_PER_DAY)}d`
  if (seconds >= SECONDS_PER_HOUR)
    return `${Math.floor(seconds / SECONDS_PER_HOUR)}h`
  return `${Math.max(0, Math.floor(seconds / 60))}m`
}

function itemLabel(it: RehydrateItem): string {
  if (it.kind === 'volume') return `${it.app ?? ''} / ${it.volume ?? ''}`
  return it.database ?? it.id
}

function reasonLabel(reason: string, t: TFunction<'settings'>): string {
  switch (reason) {
    case 'no_backup':
      return t('rehydrate.reason.no_backup')
    case 'backup_too_old':
      return t('rehydrate.reason.backup_too_old')
    case 'engine_keeps_no_data':
      return t('rehydrate.reason.engine_keeps_no_data')
    default:
      return reason
  }
}

function DrReportPage() {
  const { t } = useTranslation('settings')
  const { data: saved, isLoading } = useRehydrateReport()
  const plan = useRehydratePlan()
  const run = useRehydrateRun()
  const [allowStale, setAllowStale] = useState(false)
  const [confirming, setConfirming] = useState(false)

  // The live run wins over a stale dry run so progress is never hidden.
  const report: RehydrateReport | null | undefined =
    saved?.state === 'running' ? saved : (plan.data ?? saved)
  const running = saved?.state === 'running'

  const onRun = () => {
    run.mutate(allowStale, {
      onSuccess: () => {
        setConfirming(false)
        toast.add({ title: t('rehydrate.started'), type: 'success' })
      },
      onError: (err) => toast.add({ title: err.message, type: 'error' }),
    })
  }

  return (
    <div className="space-y-6">
      <div className="flex items-start gap-3">
        <div className="mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg bg-muted text-muted-foreground">
          <ClipboardListGlassIcon className="size-5" />
        </div>
        <PageHeader
          title={t('rehydrate.title')}
          description={t('rehydrate.description')}
          helpPath="/whole-server-recovery"
          helpLabel={t('rehydrate.helpLabel')}
        />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>{t('rehydrate.actionsTitle')}</CardTitle>
          <CardDescription>{t('rehydrate.actionsDescription')}</CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <label className="flex items-center gap-2 text-sm">
            <input
              type="checkbox"
              checked={allowStale}
              onChange={(e) => setAllowStale(e.target.checked)}
            />
            {t('rehydrate.allowStale')}
          </label>
          <div className="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={plan.isPending || running}
              onClick={() =>
                plan.mutate(allowStale, {
                  onError: (err) =>
                    toast.add({ title: err.message, type: 'error' }),
                })
              }
            >
              <RotateCcwGlassIcon size="1.4em" data-icon="inline-start" />
              {t('rehydrate.plan')}
            </Button>
            {!confirming ? (
              <Button disabled={running} onClick={() => setConfirming(true)}>
                <PlayGlassIcon size="1.4em" data-icon="inline-start" />
                {t('rehydrate.run')}
              </Button>
            ) : null}
          </div>
          {confirming ? (
            <Banner variant="warning" title={t('rehydrate.confirmTitle')}>
              <p className="text-sm">{t('rehydrate.confirmBody')}</p>
              <div className="mt-3 flex gap-2">
                <Button
                  variant="destructive"
                  disabled={run.isPending}
                  onClick={onRun}
                >
                  {t('rehydrate.confirm')}
                </Button>
                <Button variant="ghost" onClick={() => setConfirming(false)}>
                  {t('rehydrate.cancel')}
                </Button>
              </div>
            </Banner>
          ) : null}
        </CardContent>
      </Card>

      {isLoading ? null : report ? (
        <ReportCard report={report} />
      ) : (
        <p className="text-sm text-muted-foreground">{t('rehydrate.empty')}</p>
      )}
    </div>
  )
}

function ReportCard({ report }: { report: RehydrateReport }) {
  const { t } = useTranslation('settings')
  const s = report.summary
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {t(`rehydrate.mode.${report.mode}`)}
          {': '}
          {t(`rehydrate.state.${report.state}`)}
        </CardTitle>
        <CardDescription>
          {t('rehydrate.summary', {
            restorable: s.restorable,
            succeeded: s.succeeded,
            failed: s.failed,
            blocked: s.blocked,
            skipped: s.skipped,
            window: formatDuration(s.worst_backup_age_seconds),
          })}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {report.items.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('rehydrate.noItems')}
          </p>
        ) : (
          <ul className="divide-y divide-border">
            {report.items.map((it) => (
              <li
                key={it.id}
                className="flex flex-wrap items-center justify-between gap-2 py-2 text-sm"
              >
                <div className="min-w-0">
                  <p className="truncate font-medium">{itemLabel(it)}</p>
                  <p className="text-xs text-muted-foreground">
                    {t(`rehydrate.kind.${it.kind}`)}
                    {it.backup
                      ? ` · ${t('rehydrate.backupAge', { age: formatDuration(it.backup.age_seconds) })}`
                      : ''}
                    {it.reason ? ` · ${reasonLabel(it.reason, t)}` : ''}
                    {it.error ? ` · ${it.error}` : ''}
                  </p>
                </div>
                <Badge variant="outline">
                  {t(`rehydrate.itemStatus.${it.status}`)}
                </Badge>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}
