import { useMemo, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CaretDownIcon,
  CheckCircleIcon,
  CircleDashedIcon,
  WarningCircleIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { SkeletonList } from '@/components/kit'
import { TONE } from '@/components/kit/tone'
import { Button } from '@/components/ui/button'
import { Progress } from '@/components/ui/progress'
import { cn } from '@/lib/utils'
import type { AppListEntry } from '../../types/appDetail'
import type { AppsSummary } from '../../queries/appPages'
import {
  buildNextSteps,
  type AttentionStep,
  type NextStep,
  type StepGroup,
} from '../../lib/dashboardHome'
import { attentionToSuggestions } from '../../lib/fleetSuggestions'
import {
  deploysPerHour,
  flattenAttempts,
  pickSampleApps,
} from '../../lib/fleetStats'
import { showGuideEntryPoints } from '../../lib/setupGuide'
import { useMinuteNow } from '../../hooks/useMinuteNow'
import { useFleetDeploys } from '../../queries/fleetTraffic'
import { useSetupGuide } from '../../queries/setupGuide'
import { useAttentionSummary } from '../shell/useAttentionSummary'
import { SAMPLE_APP_CAP } from './FleetTiles'
import { useSuggestionRunner } from './useSuggestionRunner'

const MAX_SHOWN = 4

type SingleStep = Exclude<NextStep, StepGroup>

function StepRow({
  step,
  onRun,
  nested = false,
}: {
  step: SingleStep
  onRun: () => void
  nested?: boolean
}) {
  const { t } = useTranslation(['dashboard', 'getStarted'])
  const attention = step.source === 'attention'
  const tone = attention ? step.tone : 'neutral'
  const Icon = !attention
    ? CircleDashedIcon
    : tone === 'danger'
      ? WarningCircleIcon
      : WarningIcon
  const title = attention
    ? step.title
    : t(`getStarted:steps.${step.stepId}.title`)
  const detail = attention
    ? step.detail
    : t(`getStarted:steps.${step.stepId}.what`)
  const label = attention
    ? step.actionLabel
    : step.inProgress
      ? t('dashboard:nextSteps.continue')
      : t(`getStarted:steps.${step.stepId}.cta`)
  return (
    <li
      className={cn(
        '@container flex flex-col gap-3 py-3 @md:flex-row @md:items-center',
        nested ? 'pl-8' : 'first:pt-0 last:pb-0',
      )}
    >
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <span
          aria-hidden="true"
          className={cn('mt-0.5 shrink-0 [&_svg]:size-5', TONE[tone].text)}
        >
          <Icon weight={attention ? 'fill' : 'regular'} />
        </span>
        <span className="min-w-0">
          <span className="block text-sm font-medium break-words text-foreground">
            {title}
          </span>
          {detail ? (
            <span className="mt-0.5 line-clamp-2 block text-sm text-muted-foreground">
              {detail}
            </span>
          ) : null}
        </span>
      </div>
      <Button
        size="sm"
        variant={attention ? 'default' : 'outline'}
        className="self-start @md:self-auto"
        onClick={onRun}
      >
        {label}
      </Button>
    </li>
  )
}

function GroupRow({
  group,
  onRun,
}: {
  group: StepGroup
  onRun: (step: AttentionStep) => void
}) {
  const { t } = useTranslation('dashboard')
  const [open, setOpen] = useState(false)
  const count = group.items.length
  return (
    <li className="py-3 first:pt-0 last:pb-0">
      <div className="@container flex flex-col gap-3 @md:flex-row @md:items-center">
        <div className="flex min-w-0 flex-1 items-start gap-3">
          <span
            aria-hidden="true"
            className={cn(
              'mt-0.5 shrink-0 [&_svg]:size-5',
              TONE[group.tone].text,
            )}
          >
            <WarningIcon weight="fill" />
          </span>
          <span className="min-w-0">
            <span className="block text-sm font-medium text-foreground">
              {t('nextSteps.group.copy', { count })}
            </span>
            <span className="mt-0.5 line-clamp-2 block text-sm text-muted-foreground">
              {group.detail}
            </span>
          </span>
        </div>
        <Button
          size="sm"
          variant="default"
          aria-expanded={open}
          className="self-start @md:self-auto"
          onClick={() => {
            setOpen((v) => !v)
          }}
        >
          {open
            ? t('nextSteps.group.hide')
            : t('nextSteps.group.show', { count })}
          <CaretDownIcon
            aria-hidden="true"
            className={cn('transition-transform', open && 'rotate-180')}
          />
        </Button>
      </div>
      {open ? (
        <ul className="mt-2 divide-y divide-border border-l border-border">
          {group.items.map((item) => (
            <StepRow
              key={item.id}
              step={item}
              nested
              onRun={() => {
                onRun(item)
              }}
            />
          ))}
        </ul>
      ) : null}
    </li>
  )
}

export function NextStepsView({
  steps,
  loading,
  percent,
  quiet,
  onRun,
}: {
  steps: NextStep[]
  loading: boolean
  percent: number | undefined
  quiet: { running: number; total: number; deploys: number }
  onRun: (step: SingleStep) => void
}) {
  const { t } = useTranslation('dashboard')
  return (
    <section
      aria-label={t('nextSteps.title')}
      className="space-y-4 rounded-lg border border-border bg-card p-4 sm:p-5"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-x-4 gap-y-1">
        <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
          {t('nextSteps.title')}
        </h2>
        <Link
          to="/status"
          className="text-xs text-muted-foreground hover:text-foreground"
        >
          {t('nextSteps.seeAll')}
        </Link>
      </div>
      {percent !== undefined ? (
        <Progress
          value={percent}
          aria-label={t('nextSteps.setupProgress', { percent })}
          className="gap-1.5"
        >
          <span className="text-xs text-muted-foreground tabular-nums">
            {t('nextSteps.setupProgress', { percent })}
          </span>
        </Progress>
      ) : null}
      {loading ? (
        <SkeletonList rows={2} />
      ) : steps.length === 0 ? (
        <p className="flex items-start gap-2 text-sm text-muted-foreground">
          <CheckCircleIcon
            aria-hidden="true"
            weight="fill"
            className="mt-0.5 size-4 shrink-0 text-tone-success"
          />
          <span>{t('nextSteps.quiet', quiet)}</span>
        </p>
      ) : (
        <ul className="divide-y divide-border">
          {steps.map((s) =>
            s.source === 'group' ? (
              <GroupRow key={s.id} group={s} onRun={onRun} />
            ) : (
              <StepRow
                key={s.id}
                step={s}
                onRun={() => {
                  onRun(s)
                }}
              />
            ),
          )}
        </ul>
      )}
    </section>
  )
}

export function NextSteps({
  summary,
  sample,
}: {
  summary: AppsSummary
  sample: AppListEntry[]
}) {
  const { t } = useTranslation('dashboard')
  const { visible, isLoading } = useAttentionSummary()
  const { data: guide } = useSetupGuide()
  const run = useSuggestionRunner()
  const now = useMinuteNow()
  const names = useMemo(() => pickSampleApps(sample, SAMPLE_APP_CAP), [sample])
  const deploys = useFleetDeploys(names)
  const steps = buildNextSteps({
    specs: attentionToSuggestions(visible, t),
    guide,
    limit: MAX_SHOWN,
  })
  const quiet = {
    running: summary.running,
    total: summary.total,
    deploys: deploysPerHour(flattenAttempts(deploys.byApp), now).total,
  }
  return (
    <NextStepsView
      steps={steps}
      loading={isLoading}
      percent={showGuideEntryPoints(guide) ? guide?.ready_percent : undefined}
      quiet={quiet}
      onRun={(s) => {
        run(s.source === 'attention' ? s.spec : { kind: 'path', to: s.href })
      }}
    />
  )
}
