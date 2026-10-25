import { useTranslation } from 'react-i18next'
import {
  CheckCircleIcon,
  CircleDashedIcon,
  ClockIcon,
  HourglassIcon,
  LockKeyIcon,
  PushPinIcon,
  ScissorsIcon,
  ShieldCheckIcon,
  WarningCircleIcon,
  WarningIcon,
  XCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Badge } from '@/components/ui/badge'
import { formatAbsolute } from '../../lib/dateFormat'
import { StatusBadge } from '@/components/ui/status-badge'
import type { Tone } from '@/components/kit'
import type {
  Stack,
  StackRevisionStatus,
  StackRevisionSummary,
} from '../../types/stacks'
import { approvalExpiry } from './approval'
import { DriftBadge } from './StackDrift'

const STATUS_VIEW: Record<StackRevisionStatus, { tone: Tone; icon: Icon }> = {
  pending_approval: { tone: 'warning', icon: HourglassIcon },
  applying: { tone: 'info', icon: ClockIcon },
  applied: { tone: 'success', icon: CheckCircleIcon },
  partial: { tone: 'warning', icon: WarningCircleIcon },
  failed: { tone: 'danger', icon: XCircleIcon },
  superseded: { tone: 'neutral', icon: CircleDashedIcon },
}

export function RevisionStatusBadge({
  status,
}: Readonly<{ status: StackRevisionStatus | undefined }>) {
  const { t } = useTranslation('stacks')
  if (!status) {
    return <Badge variant="muted">{t('status.none')}</Badge>
  }
  const view = STATUS_VIEW[status]
  return (
    <StatusBadge
      tone={view.tone}
      icon={view.icon}
      label={t(`status.${status}`)}
    />
  )
}

// Status badge plus, for a revision waiting on a person, when the request lapses.
export function RevisionStatus({
  revision,
}: Readonly<{ revision: StackRevisionSummary }>) {
  const { t } = useTranslation('stacks')
  return (
    <span className="flex flex-wrap items-center gap-1.5">
      {revision.rejected ? (
        <StatusBadge
          tone="danger"
          icon={XCircleIcon}
          label={t('status.rejected')}
        />
      ) : (
        <RevisionStatusBadge status={revision.status} />
      )}
      {revision.status === 'pending_approval' ? (
        <span className="text-xs text-muted-foreground">
          {t('approval.expires', {
            when: formatAbsolute(approvalExpiry(revision)),
          })}
        </span>
      ) : null}
    </span>
  )
}

// Lock, pin, drift policy and approval at a glance; each is a small badge.
export function StackFlags({ stack }: Readonly<{ stack: Stack }>) {
  const { t } = useTranslation('stacks')
  return (
    <span className="flex flex-wrap items-center gap-1">
      {stack.lock ? (
        <Badge
          variant="warning"
          title={t('flags.lockedTip', {
            holder: stack.lock.holder,
            revision: stack.lock.revision,
          })}
        >
          <LockKeyIcon aria-hidden="true" />
          {t('flags.locked')}
        </Badge>
      ) : null}
      {stack.pinned_revision ? (
        <Badge variant="info">
          <PushPinIcon aria-hidden="true" />
          {t('flags.pinned', { revision: stack.pinned_revision })}
        </Badge>
      ) : null}
      <DriftBadge drift={stack.drift} />
      {stack.drift_policy === 'lock' ? (
        <Badge variant="outline">
          <WarningIcon aria-hidden="true" />
          {t('flags.driftLock')}
        </Badge>
      ) : null}
      {stack.require_approval ? (
        <Badge variant="outline">
          <ShieldCheckIcon aria-hidden="true" />
          {t('flags.approval')}
        </Badge>
      ) : null}
      {stack.allow_prune ? (
        <Badge variant="outline">
          <ScissorsIcon aria-hidden="true" />
          {t('flags.prune')}
        </Badge>
      ) : null}
    </span>
  )
}
