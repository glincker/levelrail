import {
  CircleCheckGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  CountdownGlassIcon,
  LoaderCircleGlassIcon,
} from '@thesvg/ui-react'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/badge'
import { isKnownDeployApprovalStatus } from '../../lib/deployApprovalStatus'

// An unknown status from a newer server renders as a neutral badge with
// its raw value, so the list never crashes on a status this build predates.
export function DeployApprovalStatusBadge({ status }: { status: string }) {
  const { t } = useTranslation('common')
  if (!isKnownDeployApprovalStatus(status)) {
    return (
      <Badge variant="muted" data-status={status}>
        <CircleMinusGlassIcon className="size-3.5" aria-hidden="true" />
        {status}
      </Badge>
    )
  }
  const label = t(`deployApprovalStatus.${status}`)
  switch (status) {
    case 'pending':
      return (
        <Badge variant="warning" data-status={status}>
          <CountdownGlassIcon className="size-3.5" aria-hidden="true" />
          {label}
        </Badge>
      )
    case 'approving':
      return (
        <Badge variant="muted" data-status={status}>
          <LoaderCircleGlassIcon
            className="size-3.5 animate-spin motion-reduce:animate-none"
            aria-hidden="true"
          />
          {label}
        </Badge>
      )
    case 'approved':
      return (
        <Badge variant="success" data-status={status}>
          <CircleCheckGlassIcon
            className="size-3.5 text-tone-success"
            aria-hidden="true"
          />
          {label}
        </Badge>
      )
    case 'rejected':
      return (
        <Badge variant="destructive" data-status={status}>
          <CircleXGlassIcon
            className="size-3.5 text-tone-danger"
            aria-hidden="true"
          />
          {label}
        </Badge>
      )
    case 'expired':
      return (
        <Badge variant="muted" data-status={status}>
          <CircleMinusGlassIcon className="size-3.5" aria-hidden="true" />
          {label}
        </Badge>
      )
  }
}
