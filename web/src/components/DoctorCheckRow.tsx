import {
  CheckCircleIcon,
  WarningCircleIcon,
  XCircleIcon,
  MinusCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import type { VariantProps } from 'class-variance-authority'
import { Badge, type badgeVariants } from '@/components/ui/badge'
import type { DoctorCheck, DoctorCheckStatus } from '../queries/systemDoctor'
import { CheckDetail } from './CheckDetail'

const STATUS_META: Record<
  DoctorCheckStatus,
  {
    label: string
    variant: VariantProps<typeof badgeVariants>['variant']
    icon: Icon
  }
> = {
  ok: { label: 'OK', variant: 'success', icon: CheckCircleIcon },
  warn: { label: 'Warning', variant: 'warning', icon: WarningCircleIcon },
  fail: { label: 'Failed', variant: 'destructive', icon: XCircleIcon },
  unknown: { label: 'Not checked', variant: 'muted', icon: MinusCircleIcon },
}

/** DoctorCheckRow is the full-detail rendering of one check: used on the system status page, where showing everything at once is the point. */
export function DoctorCheckRow({ check }: { check: DoctorCheck }) {
  const meta = STATUS_META[check.status]
  const StatusIcon = meta.icon
  return (
    <div className="py-2.5 text-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-medium text-foreground">{check.name}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {check.message}
          </p>
        </div>
        <Badge variant={meta.variant} className="shrink-0">
          <StatusIcon />
          {meta.label}
        </Badge>
      </div>
      <CheckDetail check={check} />
    </div>
  )
}
