import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { Skeleton } from '@/components/ui/skeleton'
import { toast } from '@/components/ui/toast'
import { JanitorPolicyCard } from '../janitor/JanitorPolicyCard'
import { JanitorReportView } from '../janitor/JanitorReportView'
import { useIsRoot } from '../../hooks/useIsRoot'
import { formatSize } from '../../lib/format'
import {
  useJanitorRuns,
  useJanitorStatus,
  useRunJanitor,
  type JanitorReport,
  type JanitorStatus,
} from '../../queries/janitor'

function modeKey(s: JanitorStatus): 'off' | 'preview' | 'removing' {
  if (!s.policy.enabled) return 'off'
  return s.policy.dry_run ? 'preview' : 'removing'
}

function StatusCard({ status }: { status: JanitorStatus }) {
  const { t } = useTranslation('janitor')
  const isRoot = useIsRoot()
  const run = useRunJanitor()
  const last = status.last_run
  const usedPercent =
    status.total_bytes > 0 ? Math.round(100 - status.free_percent) : 0

  function start(dryRun: boolean) {
    run.mutate(dryRun, {
      onSuccess: (r) => {
        if (r.error) {
          toast.add({
            title: t('toast.failed', { error: r.error }),
            type: 'error',
          })
          return
        }
        toast.add({
          title: dryRun
            ? t('toast.previewed')
            : t('toast.ran', { size: formatSize(r.reclaimed_bytes) }),
          type: 'success',
        })
      },
      onError: (e) => toast.add({ title: e.message, type: 'error' }),
    })
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          {t('status.title')}
          <Badge variant={status.policy.enabled ? 'info' : 'muted'}>
            {t(`mode.${modeKey(status)}`)}
          </Badge>
        </CardTitle>
        <CardDescription>
          {t('status.budget', {
            percent: status.policy.disk_free_budget_percent,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {status.total_bytes > 0 ? (
          <div className="space-y-1.5">
            <Progress value={usedPercent} aria-label={t('status.title')} />
            <p className="text-sm text-muted-foreground">
              {t('status.free', {
                free: formatSize(status.free_bytes),
                total: formatSize(status.total_bytes),
                percent: status.free_percent.toFixed(1),
              })}
            </p>
          </div>
        ) : (
          <p className="text-sm text-muted-foreground">
            {t('status.freeUnknown')}
          </p>
        )}
        <p className="text-sm text-muted-foreground">
          {t('status.reclaimed', {
            size: formatSize(status.total_reclaimed_bytes),
          })}
        </p>
        <p className="text-sm text-muted-foreground">
          {last
            ? t('status.lastPass', {
                when: new Date(last.started_at).toLocaleString(),
                trigger: t(`status.trigger.${last.trigger}`),
                size: formatSize(last.reclaimed_bytes),
              })
            : t('status.noPass')}
        </p>
        <div className="flex gap-2">
          <Button
            size="sm"
            variant="outline"
            disabled={!isRoot || run.isPending}
            onClick={() => start(true)}
          >
            {t('actions.preview')}
          </Button>
          <Button
            size="sm"
            disabled={!isRoot || run.isPending}
            onClick={() => start(false)}
          >
            {run.isPending ? t('actions.running') : t('actions.runNow')}
          </Button>
        </div>
        {run.data ? <JanitorReportView report={run.data} /> : null}
      </CardContent>
    </Card>
  )
}

function QuarantineCard({ status }: { status: JanitorStatus }) {
  const { t } = useTranslation('janitor')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('quarantine.title')}</CardTitle>
        <CardDescription>{t('quarantine.description')}</CardDescription>
      </CardHeader>
      <CardContent>
        {status.quarantine.length === 0 ? (
          <p className="text-sm text-muted-foreground">
            {t('quarantine.empty')}
          </p>
        ) : (
          <ul className="divide-y divide-border">
            {status.quarantine.slice(0, 50).map((q) => (
              <li
                key={q.name}
                className="flex flex-wrap items-center gap-2 py-2 text-sm"
              >
                <span className="font-medium text-foreground">{q.name}</span>
                <span className="text-muted-foreground">
                  {q.size_bytes > 0 ? formatSize(q.size_bytes) : ''}
                </span>
                <span className="ml-auto text-muted-foreground">
                  {t('quarantine.until', {
                    date: new Date(q.deletable_at).toLocaleDateString(),
                  })}
                </span>
                <Badge variant={q.ready ? 'warning' : 'muted'}>
                  {q.delete_enabled
                    ? q.ready
                      ? t('quarantine.ready')
                      : t('action.quarantined')
                    : t('quarantine.deleteOff')}
                </Badge>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

function HistoryCard({ runs }: { runs: JanitorReport[] }) {
  const { t } = useTranslation('janitor')
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('history.title')}</CardTitle>
      </CardHeader>
      <CardContent>
        {runs.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('history.empty')}</p>
        ) : (
          <ul className="divide-y divide-border">
            {runs.map((r) => (
              <li
                key={r.id}
                className="flex flex-wrap items-center gap-2 py-2 text-sm"
              >
                <span className="text-foreground">
                  {t('history.row', {
                    when: new Date(r.started_at).toLocaleString(),
                    trigger: t(`status.trigger.${r.trigger}`),
                  })}
                </span>
                {r.error ? (
                  <Badge variant="destructive">{t('history.failed')}</Badge>
                ) : null}
                <span className="ml-auto font-medium text-foreground">
                  {formatSize(r.reclaimed_bytes)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  )
}

/** The former Janitor page, now the Disk and Docker tab of the Cleanup center. */
export function DiskDockerTab() {
  const { t } = useTranslation('janitor')
  const isRoot = useIsRoot()
  const status = useJanitorStatus()
  const runs = useJanitorRuns()
  const fix = useRunJanitor()
  const data = status.data

  return (
    <div className="space-y-6">
      <p className="text-sm text-muted-foreground">{t('description')}</p>
      {status.isPending ? <Skeleton className="h-48 w-full" /> : null}
      {status.error ? (
        <Banner variant="critical" title={t('loadError')} />
      ) : null}
      {status.isSuccess && data === null ? (
        <Banner variant="info" title={t('notConfigured')} />
      ) : null}
      {data?.pressure ? (
        <Banner
          variant="warning"
          title={t('pressure.title')}
          action={
            <Button
              size="sm"
              disabled={!isRoot || fix.isPending}
              onClick={() =>
                fix.mutate(false, {
                  onSuccess: (r) =>
                    toast.add({
                      title: t('toast.ran', {
                        size: formatSize(r.reclaimed_bytes),
                      }),
                      type: r.error ? 'error' : 'success',
                    }),
                  onError: (e) =>
                    toast.add({ title: e.message, type: 'error' }),
                })
              }
            >
              {t('pressure.action')}
            </Button>
          }
        >
          {t('pressure.body')}
        </Banner>
      ) : null}
      {data ? (
        <>
          <StatusCard status={data} />
          <QuarantineCard status={data} />
          <JanitorPolicyCard
            key={JSON.stringify(data.policy)}
            policy={data.policy}
          />
          <HistoryCard runs={runs.data ?? []} />
        </>
      ) : null}
    </div>
  )
}
