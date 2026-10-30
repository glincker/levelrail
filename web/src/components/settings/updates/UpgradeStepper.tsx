import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleGlassIcon,
  LoaderCircleGlassIcon,
  LockGlassIcon,
} from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { StepperStatus, StepperStep } from '@/lib/upgradeStepper'
import { usePrefersReducedMotion } from '@/hooks/usePrefersReducedMotion'

const TONE: Record<StepperStatus, string> = {
  done: 'text-success',
  current: 'text-primary',
  failed: 'text-destructive',
  blocked: 'text-destructive',
  upcoming: 'text-muted-foreground',
}

function StatusIcon({
  status,
  motion,
}: {
  status: StepperStatus
  motion: boolean
}) {
  const cls = 'size-4 shrink-0'
  switch (status) {
    case 'done':
      return <CircleCheckGlassIcon size="1.4em" className={cls} aria-hidden />
    case 'current':
      return (
        <LoaderCircleGlassIcon
          size="1.4em"
          className={cn(cls, motion && 'animate-spin')}
          aria-hidden
        />
      )
    case 'failed':
      return <CircleAlertGlassIcon size="1.4em" className={cls} aria-hidden />
    case 'blocked':
      return <LockGlassIcon size="1.4em" className={cls} aria-hidden />
    default:
      return <CircleGlassIcon size="1.4em" className={cls} aria-hidden />
  }
}

export function UpgradeStepper({ steps }: { steps: StepperStep[] }) {
  const { t } = useTranslation('updates')
  const motion = !usePrefersReducedMotion()
  return (
    <ol
      aria-label={t('stepper.label')}
      className="flex flex-wrap gap-x-4 gap-y-2 text-xs"
    >
      {steps.map((s, i) => (
        <li
          key={s.id}
          className={cn('flex items-center gap-1.5', TONE[s.status])}
          aria-current={s.status === 'current' ? 'step' : undefined}
        >
          <StatusIcon status={s.status} motion={motion} />
          <span className="font-medium">
            {i + 1}. {t(`stepper.step.${s.id}`)}
          </span>
          <span className="sr-only">{t(`stepper.status.${s.status}`)}</span>
        </li>
      ))}
    </ol>
  )
}
