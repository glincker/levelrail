import type * as React from 'react'
import { useTranslation } from 'react-i18next'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { Kbd } from '../commandPaletteEntries'

export interface Bubble {
  key: string
  label: string
  icon: React.ReactNode
  onPick: () => void
}

// Static class names per slot so Tailwind sees them; the stagger is short on purpose.
const STAGGER = [
  'delay-0',
  'delay-50',
  'delay-100',
  'delay-150',
  'delay-200',
  'delay-250',
  'delay-300',
  'delay-300',
  'delay-300',
]

// Round quick links next to the search pill. Alt+number picks one while the results panel is closed.
export function RecentBubbles({ bubbles }: { bubbles: Bubble[] }) {
  const { t } = useTranslation('palette')
  if (bubbles.length === 0) return null
  return (
    <div
      role="group"
      aria-label={t('bubbles.label')}
      className="flex shrink-0 items-center gap-2"
    >
      {bubbles.map((b, i) => (
        <Tooltip key={b.key}>
          <TooltipTrigger
            render={
              <button
                type="button"
                aria-label={b.label}
                aria-keyshortcuts={`Alt+${i + 1}`}
                onClick={b.onPick}
                className={cn(
                  'palette-pill flex size-11 items-center justify-center rounded-full text-foreground outline-none transition-transform hover:scale-105 focus-visible:ring-2 focus-visible:ring-ring [&_svg]:size-6',
                  'motion-safe:animate-in motion-safe:fade-in-0 motion-safe:zoom-in-50 motion-safe:duration-200 motion-safe:ease-[cubic-bezier(0.34,1.56,0.64,1)] motion-safe:fill-mode-backwards',
                  STAGGER[i] ?? 'delay-300',
                )}
              />
            }
          >
            {b.icon}
          </TooltipTrigger>
          <TooltipContent side="bottom">
            {b.label}
            <Kbd>{`Alt+${i + 1}`}</Kbd>
          </TooltipContent>
        </Tooltip>
      ))}
    </div>
  )
}
