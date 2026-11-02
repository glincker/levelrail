import { useEffect, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CircleCheckGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'

export type SaveState = 'idle' | 'saving' | 'saved' | 'error'

// Env-tunable so slow pages can keep the confirmation visible longer.
const ENV_CLEAR_MS = Number(import.meta.env.VITE_SAVE_STATUS_CLEAR_MS)
export const SAVE_STATUS_CLEAR_MS =
  Number.isFinite(ENV_CLEAR_MS) && ENV_CLEAR_MS > 0 ? ENV_CLEAR_MS : 2500

export interface SaveStatusProps {
  state: SaveState
  /** How long "Saved" stays before it clears. */
  clearAfterMs?: number
  className?: string
}

function Chip({
  state,
  clearAfterMs,
}: {
  state: Exclude<SaveState, 'idle'>
  clearAfterMs: number
}) {
  const { t } = useTranslation('uiShared')
  const [expired, setExpired] = useState(false)
  useEffect(() => {
    if (state !== 'saved') return
    const id = window.setTimeout(() => setExpired(true), clearAfterMs)
    return () => window.clearTimeout(id)
  }, [state, clearAfterMs])
  if (expired) return null
  if (state === 'error') {
    return (
      <span className="inline-flex items-center gap-1 text-tone-danger">
        <TriangleAlertGlassIcon size="1.2em" aria-hidden="true" />
        {t('saveStatus.error')}
      </span>
    )
  }
  if (state === 'saved') {
    return (
      <span className="inline-flex items-center gap-1">
        <CircleCheckGlassIcon
          size="1.2em"
          aria-hidden="true"
          className="text-tone-success"
        />
        {t('saveStatus.saved')}
      </span>
    )
  }
  return <span>{t('saveStatus.saving')}</span>
}

/** SaveStatus is an inline Saving, Saved or Could not save chip; the live region stays mounted so changes are announced. */
export function SaveStatus({
  state,
  clearAfterMs = SAVE_STATUS_CLEAR_MS,
  className,
}: SaveStatusProps) {
  return (
    <span
      role="status"
      aria-live="polite"
      className={cn('text-xs text-muted-foreground', className)}
    >
      {state === 'idle' ? null : (
        <Chip key={state} state={state} clearAfterMs={clearAfterMs} />
      )}
    </span>
  )
}
