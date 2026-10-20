import { useState, type ReactNode } from 'react'
import { SpinnerGapIcon, XIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { TONE, type Tone } from './tone'

export interface SuggestionAction {
  label: string
  onClick: () => void | Promise<void>
  kind?: 'primary' | 'secondary'
  pending?: boolean
}

export interface SuggestionProps {
  tone?: Tone
  icon?: ReactNode
  title: string
  detail?: string
  actions: SuggestionAction[]
  onDismiss?: () => void
}

export function Suggestion({
  tone = 'info',
  icon,
  title,
  detail,
  actions,
  onDismiss,
}: SuggestionProps) {
  const t = TONE[tone]
  const [busy, setBusy] = useState<number | null>(null)
  const anyPending = busy !== null || actions.some((a) => a.pending)

  const run = async (i: number, action: SuggestionAction) => {
    if (anyPending) return
    setBusy(i)
    try {
      await action.onClick()
    } finally {
      setBusy(null)
    }
  }

  return (
    <div
      data-testid="suggestion"
      data-tone={tone}
      className="relative flex items-start gap-3 overflow-hidden rounded-xl border border-border bg-card py-3 pr-3 pl-4 shadow-raised"
    >
      <span
        aria-hidden="true"
        className={cn('absolute inset-y-0 left-0 w-1', t.solid)}
      />
      {icon && (
        <span
          className={cn(
            'mt-0.5 flex size-8 shrink-0 items-center justify-center rounded-lg [&_svg]:size-4',
            t.soft,
            t.text,
          )}
        >
          {icon}
        </span>
      )}
      <div className="flex min-w-0 flex-1 flex-col gap-2">
        <div className="min-w-0">
          <p className="text-sm font-medium">{title}</p>
          {detail && <p className="text-sm text-muted-foreground">{detail}</p>}
        </div>
        {actions.length > 0 && (
          <div className="flex flex-wrap items-center gap-2">
            {actions.map((a, i) => {
              const pending = a.pending || busy === i
              return (
                <Button
                  key={`${a.label}-${i}`}
                  size="sm"
                  variant={a.kind === 'secondary' ? 'ghost' : 'default'}
                  disabled={anyPending}
                  aria-busy={pending || undefined}
                  onClick={() => void run(i, a)}
                >
                  {pending && (
                    <SpinnerGapIcon
                      className="animate-spin"
                      aria-hidden="true"
                    />
                  )}
                  {a.label}
                </Button>
              )
            })}
          </div>
        )}
      </div>
      {onDismiss && (
        <button
          type="button"
          aria-label="Dismiss"
          onClick={onDismiss}
          className="flex size-6 shrink-0 items-center justify-center rounded-md text-muted-foreground outline-none transition-colors hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring/60"
        >
          <XIcon className="size-3.5" aria-hidden="true" />
        </button>
      )}
    </div>
  )
}
