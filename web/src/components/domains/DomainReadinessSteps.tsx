import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  ClockGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import {
  allStepsDone,
  longestTtl,
  readinessSteps,
  routeProblem,
  ttlLabel,
  type ReadinessInput,
  type StepState,
} from './domainReadiness'
import { useRecheckCountdown } from './useRecheckCountdown'
import type { DnsSource } from '../../queries/domainCheck'
import type { ProxyDomain } from '../../queries/proxyIntegration'

const DomainDiagnoseDialog = lazy(() =>
  import('./DomainDiagnoseDialog').then((m) => ({
    default: m.DomainDiagnoseDialog,
  })),
)

function StepIcon({ state }: { state: StepState }) {
  if (state === 'done')
    return (
      <CircleCheckGlassIcon
        className="size-5 text-tone-success"
        aria-hidden="true"
      />
    )
  if (state === 'blocked')
    return (
      <CircleAlertGlassIcon
        className="size-5 text-destructive"
        aria-hidden="true"
      />
    )
  return (
    <ClockGlassIcon
      className={cn(
        'size-5 text-muted-foreground',
        state === 'progress' && 'text-tone-warning',
      )}
      aria-hidden="true"
    />
  )
}

// Three plain steps with one sentence each, one Check now and a visible auto recheck.
export function DomainReadinessSteps({
  domain,
  input,
  authoritative,
  localStale,
  checkedAt,
  isFetching,
  onRefetch,
  proxyRow,
  showControls = true,
}: {
  domain: string
  input: ReadinessInput
  authoritative?: DnsSource[]
  localStale?: boolean
  checkedAt?: string
  isFetching: boolean
  onRefetch: () => void
  proxyRow?: ProxyDomain
  // The domain page runs its own header countdown and Check now.
  showControls?: boolean
}) {
  const { t } = useTranslation('domains')
  const [diagnosing, setDiagnosing] = useState(false)
  const steps = readinessSteps(input)
  const done = allStepsDone(steps)
  const left = useRecheckCountdown({
    active: showControls && !done && !isFetching,
    resetKey: checkedAt,
    onFire: onRefetch,
  })
  const ttl = longestTtl(authoritative)
  const ttlText = ttl
    ? t('readiness.ttl', {
        time: t(`readiness.unit.${ttlLabel(ttl).unit}`, {
          count: ttlLabel(ttl).count,
        }),
      })
    : ''
  const problem = routeProblem(proxyRow, input.route)
  return (
    <div className="mt-2 space-y-2 text-xs" data-testid="readiness-steps">
      <ol
        className="grid gap-1.5 sm:grid-cols-3"
        aria-label={t('readiness.title')}
      >
        {steps.map((s, i) => (
          <li
            key={s.id}
            className="flex items-start gap-2 rounded-md border border-border bg-background p-2"
          >
            <StepIcon state={s.state} />
            <span className="min-w-0">
              <span className="block font-medium text-foreground">
                {i + 1}. {t(`readiness.step.${s.id}`)}
              </span>
              <span className="block text-muted-foreground">
                {t(`readiness.say.${s.id}.${s.state}`)}
              </span>
            </span>
          </li>
        ))}
      </ol>
      <div aria-live="polite" className="space-y-1">
        {localStale && steps[0]?.state === 'done' ? (
          <p className="text-muted-foreground">{t('readiness.localLag')}</p>
        ) : null}
        {steps[0]?.state !== 'done' && ttlText ? (
          <p className="text-muted-foreground">{ttlText}</p>
        ) : null}
        {steps[1]?.state === 'progress' ? (
          <p className="text-foreground">{t('readiness.issuing')}</p>
        ) : null}
        {steps[1]?.state === 'blocked' ? (
          <p className="text-destructive">{t('readiness.issueFailed')}</p>
        ) : null}
        {problem ? (
          <p className="text-foreground">{t(`readiness.route.${problem}`)}</p>
        ) : null}
      </div>
      <div className="flex flex-wrap items-center gap-2">
        {showControls ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={onRefetch}
            disabled={isFetching}
          >
            <RefreshCwGlassIcon
              className={isFetching ? 'size-5 animate-spin' : 'size-5'}
              aria-hidden="true"
            />
            {isFetching ? t('dnsCheck.checking') : t('readiness.checkNow')}
          </Button>
        ) : null}
        {problem ? (
          <Button
            type="button"
            variant="ghost"
            size="sm"
            onClick={() => {
              setDiagnosing(true)
            }}
          >
            {t('readiness.diagnose')}
          </Button>
        ) : null}
        {showControls ? (
          <span className="text-muted-foreground" aria-live="polite">
            {done
              ? t('readiness.allDone')
              : isFetching
                ? ''
                : t('readiness.countdown', { seconds: left })}
          </span>
        ) : null}
      </div>
      {diagnosing ? (
        <Suspense fallback={null}>
          <DomainDiagnoseDialog
            domain={domain}
            onClose={() => {
              setDiagnosing(false)
            }}
          />
        </Suspense>
      ) : null}
    </div>
  )
}
