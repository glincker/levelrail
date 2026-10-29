import { useTranslation } from 'react-i18next'
import {
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  CircleXGlassIcon,
  ClockGlassIcon,
  CountdownGlassIcon,
  LockGlassIcon,
  PinGlassIcon,
  ScissorsGlassIcon,
  ShieldCheckGlassIcon,
  TriangleAlertGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
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

const STATUS_VIEW: Record<StackRevisionStatus, { tone: Tone; icon: UiIcon }> = {
  pending_approval: { tone: 'warning', icon: CountdownGlassIcon },
  applying: { tone: 'info', icon: ClockGlassIcon },
  applied: { tone: 'success', icon: CircleCheckGlassIcon },
  partial: { tone: 'warning', icon: CircleAlertGlassIcon },
  failed: { tone: 'danger', icon: CircleXGlassIcon },
  superseded: { tone: 'neutral', icon: CircleDashedGlassIcon },
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
          icon={CircleXGlassIcon}
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
          <LockGlassIcon size="1.4em" aria-hidden="true" />
          {t('flags.locked')}
        </Badge>
      ) : null}
      {stack.pinned_revision ? (
        <Badge variant="info">
          <PinGlassIcon size="1.4em" aria-hidden="true" />
          {t('flags.pinned', { revision: stack.pinned_revision })}
        </Badge>
      ) : null}
      <DriftBadge drift={stack.drift} />
      {stack.drift_policy === 'lock' ? (
        <Badge variant="outline">
          <TriangleAlertGlassIcon
            className="text-tone-warning"
            size="1.4em"
            aria-hidden="true"
          />
          {t('flags.driftLock')}
        </Badge>
      ) : null}
      {stack.require_approval ? (
        <Badge variant="outline">
          <ShieldCheckGlassIcon
            className="text-tone-success"
            size="1.4em"
            aria-hidden="true"
          />
          {t('flags.approval')}
        </Badge>
      ) : null}
      {stack.allow_prune ? (
        <Badge variant="outline">
          <ScissorsGlassIcon size="1.4em" aria-hidden="true" />
          {t('flags.prune')}
        </Badge>
      ) : null}
    </span>
  )
}
