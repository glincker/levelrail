import {
  CheckCircleIcon,
  CircleDashedIcon,
  CircleNotchIcon,
  HourglassIcon,
  MinusCircleIcon,
  ProhibitIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { PipelineStatus } from '../types/pipelines'
import { STATUS_LABEL, STATUS_VARIANT } from '../lib/pipelineStatus'
import { Badge } from '@/components/ui/badge'

export function PipelineStatusIcon({
  status,
  className,
  spin = true,
}: {
  status: PipelineStatus
  className?: string
  spin?: boolean
}) {
  switch (status) {
    case 'succeeded':
      return <CheckCircleIcon className={className} aria-hidden="true" />
    case 'failed':
      return <XCircleIcon className={className} aria-hidden="true" />
    case 'running':
      return (
        <CircleNotchIcon
          className={
            spin
              ? `${className ?? ''} animate-spin motion-reduce:animate-none`
              : className
          }
          aria-hidden="true"
        />
      )
    case 'waiting_approval':
      return <HourglassIcon className={className} aria-hidden="true" />
    case 'cancelled':
      return <ProhibitIcon className={className} aria-hidden="true" />
    case 'skipped':
      return <MinusCircleIcon className={className} aria-hidden="true" />
    default:
      return <CircleDashedIcon className={className} aria-hidden="true" />
  }
}

export function PipelineStatusBadge({ status }: { status: PipelineStatus }) {
  return (
    <Badge variant={STATUS_VARIANT[status]}>
      <PipelineStatusIcon status={status} />
      {STATUS_LABEL[status]}
    </Badge>
  )
}
