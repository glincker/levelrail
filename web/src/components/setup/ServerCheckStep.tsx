import { useMemo } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowsClockwiseIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { systemDoctorQueryOptions } from '../../queries/systemDoctor'
import { serverCheckGate } from '../../lib/setupWizard'
import {
  READINESS_CATEGORIES,
  buildReadiness,
  parseCapacity,
} from '../../lib/setupReadiness'
import { StepFooter } from './StepChrome'
import { ReadinessRing } from './ReadinessRing'
import { CategoryRow } from './CategoryRow'
import { WarningCard } from './WarningCard'
import { CapacityPanel } from './CapacityPanel'
import { useStaggeredReveal } from './useStaggeredReveal'
import type { StepProps } from './types'

/** ServerCheckStep runs the doctor bundle and presents it as a readiness story; it blocks only on checks nothing can deploy without. */
export function ServerCheckStep({ onContinue, pending }: StepProps) {
  const { t } = useTranslation('setup')
  const { data, isFetching, refetch, error } = useQuery(
    systemDoctorQueryOptions(),
  )
  const gate = serverCheckGate(isFetching ? undefined : data)
  const readiness = useMemo(() => (data ? buildReadiness(data) : null), [data])
  const capacity = useMemo(() => (data ? parseCapacity(data) : null), [data])

  const rowCount = readiness?.categories.length ?? READINESS_CATEGORIES.length
  const revealed = useStaggeredReveal(rowCount, Boolean(data) && !isFetching)
  const running = isFetching || (Boolean(data) && revealed < rowCount)
  const shown = readiness && !running

  let headline = t('server.running')
  let note = t('server.runningNote')
  if (shown) {
    const warnCount = readiness.warnings.length
    const blockCount = readiness.blocking.length
    if (readiness.verdict === 'ready') {
      headline = t('server.headline.ready')
    } else if (readiness.verdict === 'attention') {
      headline = t('server.headline.attention', { count: warnCount })
    } else {
      headline = t('server.headline.blocked', { count: blockCount })
    }
    note = t(`server.headlineNote.${readiness.verdict}`)
  }

  const ringLabel =
    shown && readiness.score !== null
      ? t('server.scoreOf', {
          passed: readiness.passed,
          total: readiness.total,
        })
      : t('server.scoreUnavailable')

  return (
    <div className="space-y-6">
      <p className="max-w-prose text-sm text-muted-foreground">
        {t('server.intro')}
      </p>

      {error && !data ? (
        <Alert variant="destructive">
          <XCircleIcon />
          <AlertTitle>{t('server.error.title')}</AlertTitle>
          <AlertDescription>
            <p>{t('server.error.body')}</p>
            <p className="mt-1 break-words text-xs">{error.message}</p>
            <Button
              type="button"
              size="sm"
              variant="outline"
              className="mt-2"
              onClick={() => {
                void refetch()
              }}
            >
              {t('server.error.retry')}
            </Button>
          </AlertDescription>
        </Alert>
      ) : (
        <div className="flex flex-col gap-5 rounded-xl border border-border bg-gradient-to-b from-tone-accent-soft/40 to-transparent p-4 sm:flex-row sm:items-center">
          <ReadinessRing
            score={shown ? readiness.score : null}
            verdict={shown ? readiness.verdict : 'attention'}
            label={ringLabel}
          />
          <div className="min-w-0 flex-1 space-y-1" aria-live="polite">
            <p className="text-xs font-medium uppercase tracking-wider text-muted-foreground">
              {t('server.scoreLabel')}
            </p>
            <h3 className="text-xl font-semibold tracking-tight text-foreground">
              {headline}
            </h3>
            <p className="text-sm text-muted-foreground">{note}</p>
          </div>
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              void refetch()
            }}
            disabled={isFetching}
          >
            <ArrowsClockwiseIcon
              className={
                isFetching ? 'animate-spin motion-reduce:animate-none' : ''
              }
            />
            {t('server.rerun')}
          </Button>
        </div>
      )}

      <div className="space-y-2" aria-busy={running}>
        {readiness && !isFetching
          ? readiness.categories.map((c, i) => (
              <CategoryRow key={c.id} category={c} pending={i >= revealed} />
            ))
          : null}
        {!readiness || isFetching
          ? READINESS_CATEGORIES.map((id) => <PendingRow key={id} id={id} />)
          : null}
      </div>

      {shown && readiness.blocking.length > 0 ? (
        <section className="space-y-2">
          <h3 className="text-sm font-medium text-foreground">
            {t('warnings.blockingHeading')}
          </h3>
          {readiness.blocking.map((c) => (
            <WarningCard key={c.code} check={c} blocking />
          ))}
        </section>
      ) : null}

      {shown && readiness.warnings.length > 0 ? (
        <section className="space-y-2">
          <h3 className="text-sm font-medium text-foreground">
            {t('warnings.heading')}
          </h3>
          {readiness.warnings.map((c) => (
            <WarningCard key={c.code} check={c} blocking={false} />
          ))}
        </section>
      ) : null}

      {shown && data && capacity ? (
        <CapacityPanel report={data} capacity={capacity} />
      ) : null}

      <StepFooter gate={gate} onContinue={onContinue} pending={pending} />
    </div>
  )
}

function PendingRow({ id }: { id: (typeof READINESS_CATEGORIES)[number] }) {
  const { t } = useTranslation('setup')
  return (
    <div className="flex items-center gap-3 rounded-xl border border-border bg-card p-3">
      <span className="kit-shimmer size-9 shrink-0 rounded-lg bg-muted" />
      <span className="min-w-0">
        <span className="block text-sm font-medium text-foreground">
          {t(`server.categories.${id}.name`)}
        </span>
        <span className="block text-xs text-muted-foreground">
          {t('server.running')}
        </span>
      </span>
    </div>
  )
}
