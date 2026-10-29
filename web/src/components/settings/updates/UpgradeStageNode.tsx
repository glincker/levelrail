import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, MinusGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'
import type { PhaseView } from '../../../lib/upgradePhases'
import { formatElapsed } from '../../../lib/upgradeTime'
import { ENTER_DELAYS } from './progressScale'

interface Props {
  phase: PhaseView
  index: number
  selected: boolean
  motion: boolean
  panelId: string
  onSelect: () => void
}

const TONE: Record<PhaseView['state'], string> = {
  ok: 'border-success bg-success text-white',
  running: 'border-primary bg-card text-primary',
  failed: 'border-destructive bg-destructive text-white',
  skipped: 'border-muted-foreground bg-card text-muted-foreground',
  pending: 'border-border bg-card text-muted-foreground',
}

// One-shot effect classes fire on a state transition only, never on first
// render, so a page opened mid-run does not replay every finished stage.
function useTransitionEffect(
  state: PhaseView['state'],
): 'done' | 'fail' | null {
  const [prev, setPrev] = useState(state)
  const [fx, setFx] = useState<'done' | 'fail' | null>(null)
  if (prev !== state) {
    setPrev(state)
    setFx(state === 'ok' ? 'done' : state === 'failed' ? 'fail' : null)
  }
  return fx
}

export function UpgradeStageNode({
  phase,
  index,
  selected,
  motion,
  panelId,
  onSelect,
}: Props) {
  const { t } = useTranslation('updates')
  const fx = useTransitionEffect(phase.state)
  const running = phase.state === 'running'
  const label = t(`pipeline.phase.${phase.id}`)
  const status = t(`pipeline.state.${phase.state}`)
  return (
    <button
      type="button"
      aria-expanded={selected}
      aria-controls={panelId}
      onClick={onSelect}
      className={cn(
        'flex w-full items-center gap-3 rounded-md p-1.5 text-left outline-none focus-visible:ring-2 focus-visible:ring-ring sm:w-28 sm:flex-col sm:gap-1.5 sm:text-center',
        selected && 'bg-muted/60',
        motion && 'upg-enter',
        motion && ENTER_DELAYS[Math.min(index, ENTER_DELAYS.length - 1)],
        motion && fx === 'fail' && 'upg-shake',
      )}
    >
      <span className="relative grid size-9 shrink-0 place-items-center">
        {running && motion ? (
          <span
            className="upg-ring absolute inset-0 rounded-full bg-primary/30"
            aria-hidden
          />
        ) : null}
        {running ? (
          <span
            className={cn(
              'absolute inset-0 rounded-full border-2 border-transparent border-t-primary',
              motion && 'animate-spin',
            )}
            aria-hidden
          />
        ) : null}
        {fx === 'done' && motion ? (
          <span
            className="upg-ripple absolute inset-0 rounded-full bg-success/40"
            aria-hidden
          />
        ) : null}
        {fx === 'fail' && motion ? (
          <span
            className="upg-flash absolute inset-0 rounded-full bg-destructive/50"
            aria-hidden
          />
        ) : null}
        <span
          className={cn(
            'relative grid size-7 place-items-center rounded-full border-2',
            TONE[phase.state],
            motion && fx === 'done' && 'upg-pop',
          )}
        >
          {phase.state === 'ok' ? (
            <CheckGlassIcon className="size-5" aria-hidden />
          ) : null}
          {phase.state === 'failed' ? (
            <XGlassIcon className="size-5" aria-hidden />
          ) : null}
          {phase.state === 'skipped' ? (
            <MinusGlassIcon className="size-5" aria-hidden />
          ) : null}
        </span>
      </span>
      <span className="min-w-0">
        <span
          className={cn(
            'block text-sm font-medium',
            phase.state === 'pending' && 'text-muted-foreground',
          )}
        >
          {label}
        </span>
        <span className="block text-xs text-muted-foreground">
          <span className="sr-only">{status}. </span>
          {phase.durationMs > 0 ? formatElapsed(phase.durationMs) : status}
        </span>
      </span>
    </button>
  )
}
