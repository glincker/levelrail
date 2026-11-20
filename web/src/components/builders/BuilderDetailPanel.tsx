import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import {
  FlaskConicalGlassIcon,
  HammerGlassIcon,
  PauseGlassIcon,
  PlayGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Progress } from '@/components/ui/progress'
import { StatusBadge } from '@/components/ui/status-badge'
import { RelativeTime, Sparkline } from '@/components/kit'
import { InfoTip } from '../kit/InfoTip'
import { DeleteNodeDialog } from '../DeleteNodeDialog'
import {
  builderState,
  durationBarPercent,
  durationSeries,
  formatDurationMs,
} from '../../lib/builderFormat'
import {
  useBuilder,
  useBuilders,
  useSetBuilderPaused,
  useTestBuilder,
  type BuilderDetail,
} from '../../queries/builders'
import { BuilderTestOutcome } from './BuilderTestResult'

export function BuilderActions({
  builder,
}: Readonly<{ builder: BuilderDetail }>) {
  const { t } = useTranslation('nodes')
  const navigate = useNavigate()
  const pause = useSetBuilderPaused(builder.id)
  const test = useTestBuilder(builder.id)
  const PauseIcon = builder.paused ? PlayGlassIcon : PauseGlassIcon
  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          variant="outline"
          disabled={pause.isPending || builder.accepts_app_workloads}
          onClick={() => {
            pause.mutate(!builder.paused)
          }}
        >
          <PauseIcon aria-hidden="true" size="1.4em" />
          {builder.paused
            ? t('builders.detail.resume')
            : t('builders.detail.pause')}
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={test.isPending}
          onClick={() => {
            test.mutate()
          }}
        >
          <FlaskConicalGlassIcon aria-hidden="true" size="1.4em" />
          {t('builders.detail.test')}
        </Button>
        <InfoTip label={t('builders.detail.testTipLabel')}>
          {t('builders.detail.testTip')}
        </InfoTip>
        <span className="ml-auto">
          <DeleteNodeDialog
            id={builder.id}
            name={builder.name}
            onDeleted={() => {
              void navigate({ to: '/nodes' })
            }}
          />
        </span>
      </div>
      {builder.accepts_app_workloads ? (
        <p className="text-xs text-muted-foreground">
          {t('builders.detail.acceptsApps')}
        </p>
      ) : null}
      {pause.isError ? (
        <Alert variant="destructive">
          <AlertDescription>
            {t('builders.detail.pauseError', { message: pause.error.message })}
          </AlertDescription>
        </Alert>
      ) : null}
      <BuilderTestOutcome
        pending={test.isPending}
        result={test.data}
        error={test.error}
      />
    </div>
  )
}

function RecentBuilds({ builder }: Readonly<{ builder: BuilderDetail }>) {
  const { t } = useTranslation('nodes')
  const builds = builder.builds ?? []
  const series = durationSeries(builds)
  const max = Math.max(0, ...series)
  if (builds.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">
        {t('builders.detail.recentEmpty')}
      </p>
    )
  }
  return (
    <div className="space-y-2">
      {series.length > 1 ? (
        <Sparkline
          values={series}
          width="fill"
          height={28}
          ariaLabel={t('builders.detail.trend', { count: series.length })}
        />
      ) : null}
      <ul className="divide-y divide-border text-sm">
        {builds.map((b) => (
          <li
            key={`${b.app}-${b.tag}-${b.started_at}`}
            className="flex flex-wrap items-center gap-x-3 gap-y-1 py-1.5"
          >
            <span className="min-w-0 flex-1 basis-32 truncate font-medium">
              {b.app}
              <span className="ml-1.5 font-mono text-xs text-muted-foreground">
                {b.tag}
              </span>
            </span>
            <Progress
              value={durationBarPercent(b.duration_ms, max)}
              aria-label={formatDurationMs(b.duration_ms)}
              className="w-24"
            />
            <span className="w-16 text-right text-xs tabular-nums text-muted-foreground">
              {formatDurationMs(b.duration_ms)}
            </span>
            <Badge variant={b.result === 'success' ? 'success' : 'destructive'}>
              {t(`builders.detail.${b.result}`)}
            </Badge>
          </li>
        ))}
      </ul>
    </div>
  )
}

function Panel({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('nodes')
  const { data: b } = useBuilder(id)
  if (!b) return null
  return (
    <Card>
      <CardHeader className="flex-row flex-wrap items-center gap-2">
        <CardTitle className="flex items-center gap-2 text-sm">
          <HammerGlassIcon aria-hidden="true" className="size-4" />
          {t('builders.detail.title')}
          <InfoTip label={t('builders.detail.tipLabel')}>
            {t('builders.detail.tip')}
          </InfoTip>
        </CardTitle>
        <StatusBadge
          state={builderState(b.status)}
          label={t(`builders.status.${b.status}`, { defaultValue: b.status })}
        />
        <Badge variant="muted">
          {t(`builderTrust.tier.${b.tier}`, { defaultValue: b.tier })}
        </Badge>
        {b.agent_version ? (
          <span className="text-xs text-muted-foreground">
            {t('builders.detail.agent', { value: b.agent_version })}
          </span>
        ) : null}
        {b.last_seen_at ? (
          <span className="ml-auto text-xs text-muted-foreground">
            <RelativeTime at={b.last_seen_at} />
          </span>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3">
        <BuilderActions builder={b} />
        <h3 className="text-xs font-medium text-muted-foreground">
          {t('builders.detail.recent')}
        </h3>
        <RecentBuilds builder={b} />
      </CardContent>
    </Card>
  )
}

// Only nodes the builder list knows get the panel; other nodes render nothing.
export default function BuilderDetailPanel({
  nodeId,
}: Readonly<{ nodeId: string }>) {
  const { data } = useBuilders()
  if (!(data?.builders ?? []).some((b) => b.id === nodeId)) return null
  return <Panel id={nodeId} />
}
