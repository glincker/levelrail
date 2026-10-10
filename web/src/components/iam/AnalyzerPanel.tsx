import { useMemo, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightIcon,
  CheckCircleIcon,
  InfoIcon,
  WarningCircleIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { TONE } from '../kit/tone'
import type { Tone } from '../kit/tone'
import { ReadinessRing } from '../setup/ReadinessRing'
import { iamAnalysisQueryOptions } from '../../queries/iamBuilder'
import type { FindingSeverity, IamFinding } from '../../queries/iamBuilder'

const SEVERITIES: FindingSeverity[] = [
  'critical',
  'high',
  'medium',
  'low',
  'info',
]

const SEVERITY_TONE: Record<FindingSeverity, Tone> = {
  critical: 'danger',
  high: 'danger',
  medium: 'warning',
  low: 'neutral',
  info: 'info',
}

const SEVERITY_ICON: Record<FindingSeverity, Icon> = {
  critical: XCircleIcon,
  high: XCircleIcon,
  medium: WarningCircleIcon,
  low: WarningCircleIcon,
  info: InfoIcon,
}

function verdictFor(score: number): 'ready' | 'attention' | 'blocked' {
  if (score >= 90) return 'ready'
  if (score >= 60) return 'attention'
  return 'blocked'
}

function FindingCard({
  f,
  onOpenPolicy,
  onOpenPrincipal,
}: {
  f: IamFinding
  onOpenPolicy: (policyId: string) => void
  onOpenPrincipal: (key: string) => void
}) {
  const { t } = useTranslation('iam')
  const tone = TONE[SEVERITY_TONE[f.severity]]
  const Icon = SEVERITY_ICON[f.severity]
  return (
    <article
      className={cn('rounded-xl border bg-card p-4', tone.border)}
      aria-label={f.message}
    >
      <header className="flex items-start gap-3">
        <span
          className={cn(
            'flex size-8 shrink-0 items-center justify-center rounded-lg',
            tone.soft,
            tone.text,
          )}
        >
          <Icon className="size-5" aria-hidden="true" />
        </span>
        <div className="min-w-0 flex-1">
          <p className={cn('text-xs font-medium uppercase', tone.text)}>
            {t(`analyzer.severity.${f.severity}`)}
          </p>
          <p className="mt-0.5 text-sm break-words text-foreground">
            {f.message}
          </p>
          <p className="mt-1 text-sm break-words text-muted-foreground">
            <span className="font-medium text-foreground">
              {t('analyzer.fix')}:{' '}
            </span>
            {f.fix}
          </p>
        </div>
      </header>
      <div className="mt-3 flex flex-wrap gap-2 pl-11">
        {f.policy_id ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => onOpenPolicy(f.policy_id ?? '')}
          >
            {t('analyzer.openPolicy')}
            <ArrowRightIcon />
          </Button>
        ) : null}
        {f.principal_id ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() =>
              onOpenPrincipal(`${f.principal_type}:${f.principal_id}`)
            }
          >
            {t('analyzer.openPrincipal')}
            <ArrowRightIcon />
          </Button>
        ) : null}
      </div>
    </article>
  )
}

/** AnalyzerPanel lists static findings with severity and a one line fix, under an overall score drawn like the setup readiness ring. */
export function AnalyzerPanel({
  onOpenPolicy,
  onOpenPrincipal,
}: {
  onOpenPolicy: (policyId: string) => void
  onOpenPrincipal: (key: string) => void
}) {
  const { t } = useTranslation('iam')
  const q = useQuery(iamAnalysisQueryOptions())
  const [filter, setFilter] = useState<FindingSeverity | 'all'>('all')
  const shown = useMemo(
    () =>
      (q.data?.findings ?? []).filter(
        (f) => filter === 'all' || f.severity === filter,
      ),
    [q.data, filter],
  )

  if (q.isPending) return <Skeleton className="h-48 w-full" />
  if (q.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{q.error.message}</AlertDescription>
      </Alert>
    )
  }
  const { score, counts, findings } = q.data

  return (
    <div className="space-y-4">
      <header className="flex flex-wrap items-center gap-5 rounded-xl border border-border bg-card p-4">
        <ReadinessRing
          score={score}
          verdict={verdictFor(score)}
          label={t('analyzer.score', { score })}
        />
        <div className="min-w-0 flex-1 space-y-1">
          <h2 className="text-sm font-medium text-foreground">
            {t('analyzer.title')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('analyzer.description')}
          </p>
          <div className="flex flex-wrap gap-2 pt-1">
            <FilterChip
              active={filter === 'all'}
              onClick={() => setFilter('all')}
              label={`${t('analyzer.all')} ${findings.length}`}
            />
            {SEVERITIES.filter((s) => (counts[s] ?? 0) > 0).map((s) => (
              <FilterChip
                key={s}
                active={filter === s}
                onClick={() => setFilter(s)}
                label={`${t(`analyzer.severity.${s}`)} ${counts[s] ?? 0}`}
              />
            ))}
          </div>
        </div>
        <Button
          type="button"
          variant="outline"
          onClick={() => void q.refetch()}
        >
          {t('analyzer.rerun')}
        </Button>
      </header>
      {findings.length === 0 ? (
        <EmptyState
          icon={<CheckCircleIcon className="size-5" />}
          title={t('analyzer.clean.title')}
          description={t('analyzer.clean.description')}
        />
      ) : (
        <ul className="space-y-3">
          {shown.map((f, i) => (
            <li
              key={`${f.kind}:${f.policy_id ?? ''}:${f.statement_index ?? ''}:${i}`}
            >
              <FindingCard
                f={f}
                onOpenPolicy={onOpenPolicy}
                onOpenPrincipal={onOpenPrincipal}
              />
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}

function FilterChip({
  active,
  onClick,
  label,
}: {
  active: boolean
  onClick: () => void
  label: string
}) {
  return (
    <button
      type="button"
      aria-pressed={active}
      onClick={onClick}
      className={cn(
        'rounded-full border px-2.5 py-0.5 text-xs outline-none focus-visible:ring-3 focus-visible:ring-ring/50',
        active
          ? 'border-primary bg-primary text-primary-foreground'
          : 'border-border text-muted-foreground hover:text-foreground',
      )}
    >
      {label}
    </button>
  )
}
