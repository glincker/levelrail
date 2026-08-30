import {
  CheckCircleIcon,
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
}

export const DEPLOY_ATTEMPT_STATUS_ICON: Record<DeployAttemptStatus, Icon> = {
  succeeded: CheckCircleIcon,
  failed: WarningCircleIcon,
  running: SpinnerGapIcon,
}

export const DEPLOY_ATTEMPT_STATUS_LABEL: Record<DeployAttemptStatus, string> =
  {
    succeeded: 'Succeeded',
    failed: 'Failed',
    running: 'Running',
  }

export const DEPLOY_ATTEMPT_SOURCE_LABEL: Record<DeployAttemptSource, string> =
  {
    webhook: 'Webhook',
    manual: 'Manual build',
    image: 'Image',
  }
