import {
  CheckGlassIcon,
  CircleDashedGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  ClockGlassIcon,
} from '@thesvg/ui-react'
import { useDeployDetailT } from '../deployDetail/useDeployDetailT'
import type { DeployStageStatus } from '../../lib/deployStages'

export interface NoStageStep {
  key: 'pull' | 'create' | 'readiness' | 'rollout'
  status: DeployStageStatus
}

const TONE: Record<DeployStageStatus, string> = {
  pending: 'text-muted-foreground',
  running: 'text-tone-info',
  done: 'text-tone-success',
  failed: 'text-tone-danger',
  skipped: 'text-muted-foreground',
  unknown: 'text-muted-foreground',
}

function StepIcon({ status }: { status: DeployStageStatus }) {
  const cls = `size-4 shrink-0 ${TONE[status]}`
  if (status === 'done') return <CheckGlassIcon className={cls} aria-hidden />
  if (status === 'failed')
    return <CircleXGlassIcon className={cls} aria-hidden />
  if (status === 'running') {
    return (
      <ClockGlassIcon
        className={`${cls} motion-safe:animate-pulse`}
        aria-hidden
      />
    )
  }
  if (status === 'skipped') {
    return <CircleMinusGlassIcon className={cls} aria-hidden />
  }
  return <CircleDashedGlassIcon className={cls} aria-hidden />
}

/** Honest empty state for deploys with no recorded build stages, plus the phases we do know. */
export function NoBuildStages({
  prebuilt,
  steps,
}: {
  prebuilt: boolean
  steps: NoStageStep[]
}) {
  const { t } = useDeployDetailT()
  return (
    <div className="space-y-3">
      <p className="text-sm text-muted-foreground">
        {prebuilt ? t('noStages.prebuilt') : t('buildStages.empty')}
      </p>
      <ol
        aria-label={t('noStages.label')}
        className="flex flex-wrap items-center gap-x-4 gap-y-2"
      >
        {steps.map((s) => (
          <li key={s.key} className="flex items-center gap-1.5 text-xs">
            <StepIcon status={s.status} />
            <span className="font-medium text-foreground">
              {t(`noStages.step.${s.key}`)}
            </span>
            <span className="sr-only">{t(`noStages.status.${s.status}`)}</span>
          </li>
        ))}
      </ol>
    </div>
  )
}
