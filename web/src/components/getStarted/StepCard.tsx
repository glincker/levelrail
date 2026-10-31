import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  CircleMinusGlassIcon,
  ClockGlassIcon,
  LoaderCircleGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type { SetupGuideStep, SetupGuideState } from '../../lib/setupGuide'
import { isOpen } from '../../lib/setupGuide'

const STATE_ICON: Record<SetupGuideState, ReactNode> = {
  todo: <CircleDashedGlassIcon className="size-6 text-muted-foreground" />,
  in_progress: <LoaderCircleGlassIcon className="size-6 text-tone-info" />,
  done: <CircleCheckGlassIcon className="size-6 text-tone-success" />,
  skipped: <CircleMinusGlassIcon className="size-6 text-muted-foreground" />,
  not_applicable: (
    <CircleMinusGlassIcon className="size-6 text-muted-foreground" />
  ),
}

export interface StepCardProps {
  step: SetupGuideStep
  highlighted?: boolean
  canEdit: boolean
  busy?: boolean
  /** Replaces the default link action, e.g. with a dialog trigger. */
  primaryAction?: ReactNode
  onSkip: () => void
  onUndo: () => void
  onMarkDone: () => void
}

export function StepCard({
  step,
  highlighted = false,
  canEdit,
  busy = false,
  primaryAction,
  onSkip,
  onUndo,
  onMarkDone,
}: StepCardProps) {
  const { t } = useTranslation('getStarted')
  const open = isOpen(step)
  const blockedBy = step.blocked_by?.[0]
  const settled = !open
  if (settled) {
    // Finished steps collapse to one row so the open work stays above the fold.
    return (
      <li
        data-step={step.id}
        data-state={step.state}
        className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-border bg-muted/30 px-4 py-2"
      >
        <span aria-hidden="true" className="shrink-0">
          {STATE_ICON[step.state]}
        </span>
        <h3 className="min-w-0 flex-1 truncate text-sm font-medium text-foreground">
          {t(`steps.${step.id}.title`)}
        </h3>
        {step.reason ? (
          <span className="text-xs text-muted-foreground">
            {t(`reason.${step.reason}`)}
          </span>
        ) : null}
        <Badge variant={step.state === 'done' ? 'success' : 'muted'}>
          {t(`state.${step.state}`)}
        </Badge>
        {step.state !== 'not_applicable' ? (
          <Button
            size="sm"
            variant="ghost"
            render={<Link to={step.href} />}
            nativeButton={false}
          >
            {t(`steps.${step.id}.cta`)}
          </Button>
        ) : null}
        {canEdit ? (
          <StepOverrideActions
            step={step}
            busy={busy}
            onSkip={onSkip}
            onUndo={onUndo}
            onMarkDone={onMarkDone}
          />
        ) : null}
      </li>
    )
  }
  return (
    <li
      data-step={step.id}
      data-state={step.state}
      className={cn(
        'rounded-lg border bg-card p-4',
        highlighted ? 'border-brand/50 ring-1 ring-brand/30' : 'border-border',
      )}
    >
      <div className="flex gap-3">
        <span className="mt-0.5 shrink-0" aria-hidden="true">
          {STATE_ICON[step.state]}
        </span>
        <div className="min-w-0 flex-1 space-y-2">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <h3 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
              {t(`steps.${step.id}.title`)}
            </h3>
            {step.optional ? (
              <Badge variant="muted">{t('meta.optional')}</Badge>
            ) : null}
            <Badge
              variant={
                step.state === 'done'
                  ? 'success'
                  : step.state === 'in_progress'
                    ? 'info'
                    : 'muted'
              }
            >
              {t(`state.${step.state}`)}
            </Badge>
          </div>
          <p className="text-sm text-foreground">
            {t(`steps.${step.id}.what`)}
          </p>
          <p className="text-sm text-muted-foreground">
            {t(`steps.${step.id}.why`)}
          </p>
          <StepMeta step={step} blockedBy={blockedBy} />
          <div className="flex flex-wrap items-center gap-2 pt-1">
            {primaryAction ?? (
              <Button
                size="sm"
                variant={highlighted ? 'default' : 'outline'}
                render={<Link to={step.href} />}
                nativeButton={false}
              >
                {t(`steps.${step.id}.cta`)}
                <ArrowRightGlassIcon size="1.4em" aria-hidden="true" />
              </Button>
            )}
            {canEdit ? (
              <StepOverrideActions
                step={step}
                busy={busy}
                onSkip={onSkip}
                onUndo={onUndo}
                onMarkDone={onMarkDone}
              />
            ) : null}
          </div>
        </div>
      </div>
    </li>
  )
}

function StepMeta({
  step,
  blockedBy,
}: {
  step: SetupGuideStep
  blockedBy: SetupGuideStep['id'] | undefined
}) {
  const { t } = useTranslation('getStarted')
  const parts: string[] = []
  if (isOpen(step)) {
    parts.push(t('meta.minutes', { count: step.estimate_minutes }))
  }
  if (step.progress) {
    parts.push(
      t('meta.progress', {
        done: step.progress.done,
        total: step.progress.total,
      }),
    )
  }
  if (blockedBy && isOpen(step)) {
    parts.push(t('meta.blocked', { step: t(`steps.${blockedBy}.title`) }))
  }
  const reason = step.reason ? t(`reason.${step.reason}`) : null
  if (parts.length === 0 && !reason) return null
  return (
    <p className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
      {parts.length > 0 ? (
        <span className="inline-flex items-center gap-1">
          <ClockGlassIcon aria-hidden="true" className="size-5" />
          {parts.join(' · ')}
        </span>
      ) : null}
      {reason ? <span>{reason}</span> : null}
    </p>
  )
}

function StepOverrideActions({
  step,
  busy,
  onSkip,
  onUndo,
  onMarkDone,
}: {
  step: SetupGuideStep
  busy: boolean
  onSkip: () => void
  onUndo: () => void
  onMarkDone: () => void
}) {
  const { t } = useTranslation('getStarted')
  if (step.state === 'not_applicable') return null
  if (step.state === 'skipped' || step.manual) {
    return (
      <Button size="sm" variant="ghost" disabled={busy} onClick={onUndo}>
        {t('actions.undo')}
      </Button>
    )
  }
  if (step.state === 'done') return null
  return (
    <>
      <Button size="sm" variant="ghost" disabled={busy} onClick={onSkip}>
        {t('actions.skip')}
      </Button>
      <Button size="sm" variant="ghost" disabled={busy} onClick={onMarkDone}>
        {t('actions.markDone')}
      </Button>
    </>
  )
}
