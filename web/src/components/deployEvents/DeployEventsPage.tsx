import { lazy, Suspense } from 'react'
import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowLeftIcon,
  ClockCounterClockwiseIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { SkeletonList, StatusPill } from '@/components/kit'
import { PageTemplate } from '../shell/PageTemplate'
import { useDeployEventStream } from '../../hooks/useDeployEventStream'
import {
  deployAttemptKeys,
  fetchDeployAttempts,
} from '../../queries/deployAttempts'
import { verdictTone } from '../../lib/deployEvents'
import { EventList } from './EventList'
import { PhaseSummary } from './PhaseSummary'

const CompareView = lazy(() =>
  import('./CompareView').then((m) => ({ default: m.CompareView })),
)

export interface DeployEventsPageProps {
  id: string
  other: string
  onCompare: (other: string) => void
}

function CompareControl({
  app,
  id,
  other,
  onCompare,
}: {
  app: string
  id: string
  other: string
  onCompare: (other: string) => void
}) {
  const { t } = useTranslation('deployEvents')
  const attempts = useQuery({
    queryKey: deployAttemptKeys.list(app),
    queryFn: () => fetchDeployAttempts(app),
    enabled: app !== '',
  })
  const choices = (attempts.data ?? []).filter((a) => a.id !== id)
  return (
    <div className="flex flex-col gap-1">
      <label htmlFor="deploy-events-compare" className="text-sm font-medium">
        {t('compare.label')}
      </label>
      <select
        id="deploy-events-compare"
        value={other}
        onChange={(e) => {
          onCompare(e.target.value)
        }}
        className="h-9 max-w-full rounded-lg border bg-background px-2 text-sm outline-none focus-visible:ring-2 focus-visible:ring-ring/60"
      >
        <option value="">{t('compare.none')}</option>
        {choices.map((a) => (
          <option key={a.id} value={a.id}>
            {`${a.id} (${a.status})`}
          </option>
        ))}
      </select>
      <p className="text-xs text-muted-foreground">{t('compare.hint')}</p>
    </div>
  )
}

export function DeployEventsPage({
  id,
  other,
  onCompare,
}: DeployEventsPageProps) {
  const { t } = useTranslation('deployEvents')
  const { state, connection } = useDeployEventStream(id)
  const waiting = state.events.length === 0
  const last = [...state.events].reverse().find((e) => e.kind !== 'log')
  const connectionTone =
    connection === 'live'
      ? 'success'
      : connection === 'error'
        ? 'warning'
        : 'neutral'

  return (
    <PageTemplate
      title={t('title')}
      description={t('pageTitle', { id })}
      icon={<ClockCounterClockwiseIcon />}
      width="wide"
      status={
        <span className="flex flex-wrap items-center gap-2">
          <StatusPill
            tone={verdictTone(state.verdict)}
            label={t(`verdict.${state.verdict}`)}
            live={state.live}
          />
          <StatusPill
            size="sm"
            tone={connectionTone}
            label={t(`connection.${connection}`)}
            live={connection === 'live'}
          />
        </span>
      }
      actions={
        <Button variant="outline" size="sm" render={<Link to="/deployments" />}>
          <ArrowLeftIcon aria-hidden="true" />
          {t('back')}
        </Button>
      }
    >
      <div className="flex flex-col gap-4">
        <div role="status" aria-live="polite" className="sr-only">
          {state.live && last
            ? t('announce', {
                title: last.title,
                status: t(`status.${last.status}`),
              })
            : ''}
        </div>
        {state.app !== '' && (
          <CompareControl
            app={state.app}
            id={id}
            other={other}
            onCompare={onCompare}
          />
        )}
        {state.frozen && (
          <p className="text-sm text-muted-foreground">{t('frozenNote')}</p>
        )}
        {waiting && connection === 'error' ? (
          <EmptyState
            icon={<WarningCircleIcon />}
            title={t('error.title')}
            description={t('error.notFound')}
          />
        ) : waiting ? (
          <div role="status" aria-label={t('list.loading')}>
            <SkeletonList rows={5} />
          </div>
        ) : other !== '' ? (
          <Suspense fallback={<SkeletonList rows={4} />}>
            <CompareView id={id} other={other} />
          </Suspense>
        ) : (
          <div className="grid gap-4 lg:grid-cols-[1fr_20rem]">
            <section className="min-w-0 rounded-xl border p-4">
              {state.logsDropped > 0 && (
                <p className="mb-3 text-xs text-muted-foreground">
                  {t('logs.dropped', { count: state.logsDropped })}
                </p>
              )}
              <EventList events={state.events} />
            </section>
            <aside className="flex flex-col gap-4">
              <PhaseSummary phases={state.phases} />
            </aside>
          </div>
        )}
      </div>
    </PageTemplate>
  )
}
