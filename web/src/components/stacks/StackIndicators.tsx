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
import { StatusBadge } from '@/components/ui/status-badge'
import type { Tone } from '@/components/kit'
import type { Stack, StackRevisionStatus } from '../../types/stacks'

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
