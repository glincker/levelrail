import type * as React from 'react'
import { useTranslation } from 'react-i18next'
import { XGlassIcon } from '@thesvg/ui-react'
import { cn } from '@/lib/utils'

export interface Bubble {
  key: string
  label: string
  icon: React.ReactNode
  onPick: () => void
  onRemove?: () => void
  removeLabel?: string
}

export interface BubbleGroup {
  id: string
  label: string
  bubbles: Bubble[]
}

// Recent, most used and next-up destinations as one scrolling row of bubbles instead of tall lists.
export function RecentBubbles({
  groups,
  onClear,
}: {
  groups: BubbleGroup[]
  onClear: () => void
}) {
  const { t } = useTranslation('palette')
  const shown = groups.filter((g) => g.bubbles.length > 0)
  if (shown.length === 0) return null
  return (
    <div
      role="group"
      aria-label={t('bubbles.label')}
      className="flex items-center gap-4 overflow-x-auto px-3 pt-2 pb-1 [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
    >
      {shown.map((group) => (
        <div key={group.id} className="flex shrink-0 items-center gap-1.5">
          <span className="pr-1 text-[11px] font-medium tracking-wide text-muted-foreground uppercase">
            {group.label}
          </span>
          {group.bubbles.map((b) => (
            <span
              key={b.key}
              className={cn(
                'group/bubble flex items-center rounded-full border border-border bg-foreground/5 text-xs text-foreground/90 transition-colors hover:bg-foreground/10',
              )}
            >
              <button
                type="button"
                tabIndex={-1}
                onClick={b.onPick}
                className="flex max-w-48 items-center gap-1.5 rounded-full py-1 pr-2 pl-3 outline-none focus-visible:ring-2 focus-visible:ring-ring/60 [&_svg]:size-4"
              >
                {b.icon}
                <span className="truncate">{b.label}</span>
              </button>
              {b.onRemove ? (
                <button
                  type="button"
                  tabIndex={-1}
                  aria-label={b.removeLabel}
                  onClick={b.onRemove}
                  className="mr-1 flex size-5 items-center justify-center rounded-full text-muted-foreground opacity-60 outline-none hover:bg-foreground/10 hover:text-foreground hover:opacity-100 focus-visible:opacity-100 focus-visible:ring-2 focus-visible:ring-ring/60 group-hover/bubble:opacity-100"
                >
                  <XGlassIcon size={12} />
                </button>
              ) : (
                <span className="w-1" />
              )}
            </span>
          ))}
        </div>
      ))}
      <button
        type="button"
        tabIndex={-1}
        onClick={onClear}
        className="shrink-0 rounded-full px-2 py-1 text-[11px] text-muted-foreground underline-offset-2 outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring/60"
      >
        {t('bubbles.clear')}
      </button>
    </div>
  )
}
