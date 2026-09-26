import {
  CheckCircleIcon,
  HourglassIcon,
  PauseCircleIcon,
  ProhibitIcon,
  SkipForwardIcon,
  SpinnerGapIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import type {
  DeployAttemptSource,
  DeployAttemptStatus,
} from '../types/deployAttempt'
import type { badgeVariants } from '@/components/ui/badge'
import type { Tone } from '../components/kit'

// Shared status/source vocabulary for a deploy attempt as a whole
// (distinct from a single stage, see lib/deployStages.ts for that),
// used by DeployAttemptsList and the deploy detail page so the same
// attempt reads with the same icon/label/color in both places.
export const DEPLOY_ATTEMPT_STATUS_BADGE_VARIANT: Record<
  DeployAttemptStatus,
  VariantProps<typeof badgeVariants>['variant']
> = {
  succeeded: 'success',
  failed: 'destructive',
  running: 'muted',
  held: 'warning',
  superseded: 'muted',
  queued: 'muted',
  canceled: 'muted',
}

export const DEPLOY_ATTEMPT_STATUS_TONE: Record<DeployAttemptStatus, Tone> = {
  succeeded: 'success',
  failed: 'danger',
  running: 'info',
  held: 'warning',
  superseded: 'neutral',
  queued: 'neutral',
  canceled: 'neutral',
}

export const DEPLOY_ATTEMPT_STATUS_ICON: Record<DeployAttemptStatus, Icon> = {
  succeeded: CheckCircleIcon,
  failed: WarningCircleIcon,
  running: SpinnerGapIcon,
  held: PauseCircleIcon,
  superseded: SkipForwardIcon,
  queued: HourglassIcon,
  canceled: ProhibitIcon,
}

export const DEPLOY_ATTEMPT_STATUS_LABEL: Record<DeployAttemptStatus, string> =
  {
    succeeded: 'Succeeded',
    failed: 'Failed',
    running: 'Running',
    held: 'Held (frozen)',
    superseded: 'Superseded',
    queued: 'Queued',
    canceled: 'Canceled',
  }

export const DEPLOY_ATTEMPT_SOURCE_LABEL: Record<DeployAttemptSource, string> =
  {
    webhook: 'Webhook',
    manual: 'Manual build',
    image: 'Image',
    auto_rollback: 'Auto-rollback',
  }
