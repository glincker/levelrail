import {
  BanGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  CircleMinusGlassIcon,
  CircleXGlassIcon,
  CountdownGlassIcon,
  LoaderCircleGlassIcon,
} from '@thesvg/ui-react'
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
      return (
        <CircleCheckGlassIcon
          size="1.4em"
          className={className}
          aria-hidden="true"
        />
      )
    case 'failed':
      return (
        <CircleXGlassIcon
          size="1.4em"
          className={className}
          aria-hidden="true"
        />
      )
    case 'running':
      return (
        <LoaderCircleGlassIcon
          size="1.4em"
          className={
            spin
              ? `${className ?? ''} animate-spin motion-reduce:animate-none`
              : className
          }
          aria-hidden="true"
        />
      )
    case 'waiting_approval':
      return (
        <CountdownGlassIcon
          size="1.4em"
          className={className}
          aria-hidden="true"
        />
      )
    case 'cancelled':
      return (
        <BanGlassIcon size="1.4em" className={className} aria-hidden="true" />
      )
    case 'skipped':
      return (
        <CircleMinusGlassIcon
          size="1.4em"
          className={className}
          aria-hidden="true"
        />
      )
    default:
      return (
        <CircleDashedGlassIcon
          size="1.4em"
          className={className}
          aria-hidden="true"
        />
      )
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
