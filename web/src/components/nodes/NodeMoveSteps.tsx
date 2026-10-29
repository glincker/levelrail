import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  ClockGlassIcon,
  LoaderCircleGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import type {
  NodeMove,
  NodeMoveOutcome,
  NodeMoveStatus,
} from '../../types/nodeMoves'

const MOVE_STATUS_VARIANT: Record<
  NodeMoveStatus,
  'muted' | 'success' | 'destructive' | 'warning'
> = {
  running: 'muted',
  succeeded: 'success',
  failed: 'destructive',
  rolled_back: 'warning',
}

function OutcomeIcon({ outcome }: { outcome: NodeMoveOutcome }) {
  switch (outcome) {
    case 'ok':
      return (
        <CircleCheckGlassIcon
          className="size-5 shrink-0 text-primary"
          aria-hidden="true"
        />
      )
    case 'failed':
      return (
        <CircleXGlassIcon
          className="size-5 shrink-0 text-destructive"
          aria-hidden="true"
        />
      )
    case 'retry':
      return (
        <TriangleAlertGlassIcon
          className="size-6 shrink-0 text-amber-500"
          aria-hidden="true"
        />
      )
    default:
      return (
        <ClockGlassIcon
          className="size-5 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
      )
  }
}

export function MoveStatusBadge({ status }: { status: NodeMoveStatus }) {
  const { t } = useTranslation('nodes')
  return (
    <Badge variant={MOVE_STATUS_VARIANT[status]}>
      {t(`move.status.${status}`)}
    </Badge>
  )
}

// Step log of one move, plus a live row for the phase in flight.
export function NodeMoveSteps({ move }: { move: NodeMove }) {
  const { t } = useTranslation('nodes')
  return (
    <ul className="space-y-1" aria-live="polite">
      {move.steps.map((step, i) => (
        <li
          key={`${step.phase}-${step.at}-${i}`}
          className="flex items-start gap-1.5 text-xs text-foreground"
        >
          <span className="mt-0.5">
            <OutcomeIcon outcome={step.outcome} />
          </span>
          <span className="min-w-0">
            <span className="font-medium">{t(`phase.${step.phase}`)}</span>
            {step.outcome === 'ok' ? null : (
              <span className="text-muted-foreground">
                {' '}
                ({t(`outcome.${step.outcome}`)})
              </span>
            )}
            {step.message ? (
              <span className="block break-words text-muted-foreground">
                {step.message}
              </span>
            ) : null}
          </span>
        </li>
      ))}
      {move.status === 'running' ? (
        <li className="flex items-center gap-1.5 text-xs text-muted-foreground">
          <LoaderCircleGlassIcon
            className="size-5 shrink-0 animate-spin"
            aria-hidden="true"
          />
          {t(`phase.${move.phase}`)}
        </li>
      ) : null}
    </ul>
  )
}
