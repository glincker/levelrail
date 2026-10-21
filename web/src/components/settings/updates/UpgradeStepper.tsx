import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleIcon,
  LockSimpleIcon,
  SpinnerGapIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
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
      return <CheckCircleIcon className={cls} weight="fill" aria-hidden />
    case 'current':
      return (
        <SpinnerGapIcon
          className={cn(cls, motion && 'animate-spin')}
          aria-hidden
        />
      )
    case 'failed':
      return <WarningCircleIcon className={cls} weight="fill" aria-hidden />
    case 'blocked':
      return <LockSimpleIcon className={cls} weight="fill" aria-hidden />
    default:
      return <CircleIcon className={cls} aria-hidden />
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
