import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import {
  ArrowUUpLeftIcon,
  ArrowsClockwiseIcon,
  GitDiffIcon,
  RocketLaunchIcon,
  StopCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Kbd } from '@/components/kit'
import type { Deployment } from '../../types/deployment'
import {
  cancelReason,
  promoteReason,
  redeployReason,
  rollbackReason,
} from '../../lib/deploymentReasons'
import type { DeploymentActionKind } from '../../hooks/useDeploymentActions'

interface ActionButtonProps {
  label: string
  keyHint?: string
  icon: ReactNode
  reason: string
  onClick: () => void
}

function ActionButton({
  label,
  keyHint,
  icon,
  reason,
  onClick,
}: ActionButtonProps) {
  return (
    <Button
      variant="outline"
      size="sm"
      disabled={reason !== ''}
      title={reason || undefined}
      onClick={onClick}
    >
      {icon}
      {label}
      {keyHint && !reason && (
        <span className="hidden sm:inline">
          <Kbd keys={[keyHint]} />
        </span>
      )}
    </Button>
  )
}

export interface DrawerActionsProps {
  d: Deployment
  cancelSupported: boolean
  hasProject: boolean
  onAction: (kind: DeploymentActionKind, d: Deployment) => void
  onPromote: () => void
}

export function DrawerActions({
  d,
  cancelSupported,
  hasProject,
  onAction,
  onPromote,
}: DrawerActionsProps) {
  const rows: { label: string; reason: string }[] = [
    { label: 'Roll back to this', reason: rollbackReason(d) },
    { label: 'Cancel', reason: cancelReason(d, cancelSupported) },
    { label: 'Promote', reason: promoteReason(d, hasProject) },
  ].filter((r) => r.reason !== '')
  return (
    <div className="flex flex-col gap-2">
      <div className="flex flex-wrap gap-2">
        <ActionButton
          label="Redeploy"
          keyHint="r"
          icon={<ArrowsClockwiseIcon aria-hidden="true" />}
          reason={redeployReason(d)}
          onClick={() => {
            onAction('redeploy', d)
          }}
        />
        <ActionButton
          label="Roll back to this"
          keyHint="b"
          icon={<ArrowUUpLeftIcon aria-hidden="true" />}
          reason={rollbackReason(d)}
          onClick={() => {
            onAction('rollback', d)
          }}
        />
        <Button
          variant="outline"
          size="sm"
          nativeButton={false}
          render={
            <Link
              to="/apps/$name/deploys/compare"
              params={{ name: d.app }}
              search={{ from: d.id }}
            />
          }
        >
          <GitDiffIcon aria-hidden="true" />
          Compare
        </Button>
        <ActionButton
          label="Promote"
          icon={<RocketLaunchIcon aria-hidden="true" />}
          reason={promoteReason(d, hasProject)}
          onClick={onPromote}
        />
        <ActionButton
          label="Cancel"
          keyHint="c"
          icon={<StopCircleIcon aria-hidden="true" />}
          reason={cancelReason(d, cancelSupported)}
          onClick={() => {
            onAction('cancel', d)
          }}
        />
      </div>
      {rows.length > 0 && (
        <ul className="flex flex-col gap-0.5 text-xs text-muted-foreground">
          {rows.map((r) => (
            <li key={r.label}>
              {r.label}: {r.reason}.
            </li>
          ))}
        </ul>
      )}
    </div>
  )
}
