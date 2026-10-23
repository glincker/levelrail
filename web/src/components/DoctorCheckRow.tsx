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
import { SilenceMenu } from './attention/SilenceMenu'
import { useSnoozed } from './shell/snoozeStore'

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
export function DoctorCheckRow({
  check,
  onRecheck,
}: {
  check: DoctorCheck
  onRecheck?: () => Promise<void> | void
}) {
  const meta = STATUS_META[check.status]
  const silenceKey = `doctor:${check.name}`
  const { isSnoozed } = useSnoozed()
  const silenced = isSnoozed(silenceKey)
  const StatusIcon = meta.icon
  return (
    <div className="py-2.5 text-sm">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="font-medium text-foreground">{check.name}</p>
          <p className="mt-0.5 text-xs text-muted-foreground">
            {check.message}
          </p>
          {check.evidence ? (
            <p className="mt-0.5 break-words font-mono text-xs text-muted-foreground">
              {check.evidence}
            </p>
          ) : null}
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {check.status === 'warn' || check.status === 'fail' ? (
            <SilenceMenu
              issueKey={silenceKey}
              severity={check.status === 'fail' ? 'critical' : 'warning'}
              title={check.name}
            />
          ) : null}
          <Badge variant={silenced ? 'muted' : meta.variant}>
            <StatusIcon />
            {meta.label}
          </Badge>
        </div>
      </div>
      <CheckDetail check={check} onRecheck={onRecheck} />
    </div>
  )
}
