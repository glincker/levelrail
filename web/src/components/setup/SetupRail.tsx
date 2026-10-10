import { useRef } from 'react'
import type { KeyboardEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckIcon,
  MinusIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import { SETUP_STEPS } from '../../lib/setupWizard'
import { railState } from '../../lib/setupRail'
import type { RailState } from '../../lib/setupRail'
import type { SetupStepId, SetupStepMap } from '../../lib/setupWizard'

const MARKER: Record<RailState, string> = {
  done: 'border-tone-success-border bg-tone-success-soft text-tone-success',
  current: 'border-primary bg-primary text-primary-foreground',
  attention:
    'border-tone-warning-border bg-tone-warning-soft text-tone-warning',
  skipped: 'border-border bg-muted text-muted-foreground',
  upcoming: 'border-border bg-card text-muted-foreground',
}

/** SetupRail lists every step with its state; arrow keys move focus between steps. */
export function SetupRail({
  current,
  steps,
  attention,
  onGoTo,
}: {
  current: SetupStepId
  steps: SetupStepMap
  attention: ReadonlySet<SetupStepId>
  onGoTo: (id: SetupStepId) => void
}) {
  const { t } = useTranslation('setup')
  const listRef = useRef<HTMLOListElement>(null)

  function onKeyDown(e: KeyboardEvent<HTMLOListElement>) {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return
    const buttons = Array.from(
      listRef.current?.querySelectorAll<HTMLButtonElement>('button') ?? [],
    )
    const at = buttons.findIndex((b) => b === document.activeElement)
    if (at < 0) return
    e.preventDefault()
    const delta = e.key === 'ArrowDown' ? 1 : -1
    buttons[(at + delta + buttons.length) % buttons.length]?.focus()
  }

  return (
    <nav aria-label={t('shell.railLabel')}>
      <ol ref={listRef} onKeyDown={onKeyDown} className="relative space-y-1">
        {SETUP_STEPS.map((id, i) => {
          const state = railState(id, current, steps, attention)
          const optional = id !== 'server' && id !== 'done'
          return (
            <li key={id}>
              <button
                type="button"
                onClick={() => onGoTo(id)}
                aria-current={state === 'current' ? 'step' : undefined}
                className={cn(
                  'group flex w-full items-center gap-3 rounded-lg px-2 py-2 text-left outline-none transition-colors focus-visible:ring-3 focus-visible:ring-ring/50 motion-reduce:transition-none',
                  state === 'current' ? 'bg-muted' : 'hover:bg-muted/60',
                )}
              >
                <span
                  className={cn(
                    'flex size-7 shrink-0 items-center justify-center rounded-full border text-xs font-medium tabular-nums',
                    MARKER[state],
                  )}
                >
                  {state === 'done' ? (
                    <CheckIcon className="size-4" aria-hidden="true" />
                  ) : state === 'attention' ? (
                    <WarningIcon className="size-4" aria-hidden="true" />
                  ) : state === 'skipped' ? (
                    <MinusIcon className="size-4" aria-hidden="true" />
                  ) : (
                    i + 1
                  )}
                </span>
                <span className="min-w-0 flex-1">
                  <span
                    className={cn(
                      'block truncate text-sm',
                      state === 'current'
                        ? 'font-medium text-foreground'
                        : 'text-muted-foreground group-hover:text-foreground',
                    )}
                  >
                    {t(`steps.${id}.title`)}
                  </span>
                  <span className="block truncate text-xs text-muted-foreground">
                    {optional && state === 'upcoming'
                      ? t('shell.optional')
                      : t(`shell.state.${state}`)}
                  </span>
                </span>
              </button>
            </li>
          )
        })}
      </ol>
    </nav>
  )
}
