import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { PinGlassIcon } from '@thesvg/ui-react'
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from '@/components/ui/tooltip'
import { cn } from '@/lib/utils'
import { shortcutLabel } from '@/lib/platform'
import { fuzzyMatchIndices } from '@/lib/fuzzy'
import { isRtl } from '@/lib/bubbleMotion'
import type { PinnedBubble } from '@/lib/paletteBubbleLayout'
import { useBubbleReorder } from '@/hooks/useBubbleReorder'
import { Kbd } from '../commandPaletteEntries'
import { BubbleMenu } from './BubbleMenu'

export interface Bubble {
  key: string
  label: string
  icon: React.ReactNode
  onPick: () => void
  /** Hand-placed; shows the pin mark. */
  pinned?: boolean
  /** Destination, enables "Open in new tab". */
  href?: string
  /** What gets stored when this bubble is pinned or moved. */
  entry?: PinnedBubble
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
export function RecentBubbles({
  bubbles,
  query = '',
  onReorder,
  onUnpin,
  rowRef: outerRef,
}: {
  bubbles: Bubble[]
  query?: string
  onReorder?: (next: Bubble[], movedKey: string) => void
  onUnpin?: (key: string) => void
  /** Lets the palette hit-test the row when a result is dragged onto it. */
  rowRef?: React.RefObject<HTMLDivElement | null>
}) {
  const { t } = useTranslation('palette')
  const innerRef = React.useRef<HTMLDivElement>(null)
  const rowRef = outerRef ?? innerRef
  const buttons = React.useRef(new Map<string, HTMLButtonElement>())
  const [menuKey, setMenuKey] = React.useState<string | null>(null)
  const [rtl, setRtl] = React.useState(false)
  const r = useBubbleReorder({
    bubbles,
    rowRef,
    onReorder,
    onLongPress: setMenuKey,
  })
  const order = r.shown.map((b) => b.key).join('|')

  React.useLayoutEffect(() => {
    if (rowRef.current) setRtl(isRtl(rowRef.current))
  }, [rowRef, order])

  // Keep keyboard focus on the lifted bubble after React moves its node.
  React.useLayoutEffect(() => {
    if (r.liftedKey) buttons.current.get(r.liftedKey)?.focus()
  }, [order, r.liftedKey])

  if (bubbles.length === 0) return null
  return (
    <div
      ref={rowRef}
      role="group"
      aria-label={t('bubbles.label')}
      className="group/bubbles flex shrink-0 items-center gap-2 rounded-full transition-shadow data-[drop=true]:ring-2 data-[drop=true]:ring-ring motion-reduce:transition-none"
    >
      <p id="palette-bubbles-hint" className="sr-only">
        {t('bubbles.hint')}
      </p>
      <p aria-live="polite" className="sr-only">
        {r.announcement}
      </p>
      {r.shown.map((b, i) => {
        const lifted = r.liftedKey === b.key

        const last = r.shown.length - 1
        return (
          <span
            key={b.key}
            data-bubble=""
            className="relative translate-x-(--bubble-x,0px) data-[dragging=true]:z-10 data-[dragging=true]:transition-none motion-safe:transition-[translate] motion-safe:duration-200 motion-safe:ease-out group-data-[settling=true]/bubbles:transition-none"
          >
            <Tooltip>
              <TooltipTrigger
                render={
                  <button
                    type="button"
                    ref={(el) => {
                      if (el) buttons.current.set(b.key, el)
                      else buttons.current.delete(b.key)
                    }}
                    aria-label={b.label}
                    aria-keyshortcuts={`Alt+${i + 1}`}
                    aria-describedby="palette-bubbles-hint"
                    aria-pressed={lifted ? true : undefined}
                    onClick={(e) => {
                      if (r.takeClick()) return e.preventDefault()
                      b.onPick()
                    }}
                    onContextMenu={(e) => {
                      e.preventDefault()
                      setMenuKey(b.key)
                    }}
                    onKeyDown={(e) => {
                      if (
                        e.key === 'ContextMenu' ||
                        (e.shiftKey && e.key === 'F10')
                      ) {
                        e.preventDefault()
                        return setMenuKey(b.key)
                      }
                      r.onKeyDown(e, b.key)
                    }}
                    onBlur={r.cancelLift}
                    onPointerDown={(e) => r.onPointerDown(e, i)}
                    onPointerMove={r.onPointerMove}
                    onPointerUp={r.onPointerUp}
                    onPointerCancel={r.onPointerCancel}
                    className={cn(
                      'palette-pill flex size-11 touch-none items-center justify-center rounded-full text-foreground outline-none select-none motion-safe:transition-[scale,opacity] hover:scale-105 focus-visible:ring-2 focus-visible:ring-ring active:scale-95 group-data-[reordering=true]/bubbles:hover:scale-100 [&_svg]:size-6',
                      'motion-safe:animate-in motion-safe:fade-in-0 motion-safe:zoom-in-50 motion-safe:duration-200 motion-safe:ease-[cubic-bezier(0.34,1.56,0.64,1)] motion-safe:fill-mode-backwards',
                      lifted && 'scale-110 ring-2 ring-ring',
                      STAGGER[i] ?? 'delay-300',
                      query.trim() !== '' &&
                        fuzzyMatchIndices(query, b.label).length === 0 &&
                        'opacity-60',
                    )}
                  />
                }
              >
                {b.icon}
              </TooltipTrigger>
              <TooltipContent side="bottom">
                {b.label}
                <Kbd>{shortcutLabel('alt', String(i + 1))}</Kbd>
              </TooltipContent>
            </Tooltip>
            {b.pinned ? (
              <PinGlassIcon
                size={14}
                aria-hidden="true"
                data-testid="bubble-pin-mark"
                className="pointer-events-none absolute -end-0.5 -top-0.5 text-foreground"
              />
            ) : null}
            <BubbleMenu
              open={menuKey === b.key}
              onOpenChange={(o) => setMenuKey(o ? b.key : null)}
              returnFocus={() => buttons.current.get(b.key) ?? null}
              pinned={b.pinned === true}
              href={b.href}
              canMoveLeft={rtl ? i < last : i > 0}
              canMoveRight={rtl ? i > 0 : i < last}
              onUnpin={() => onUnpin?.(b.key)}
              onMoveLeft={() => r.moveBy(b.key, rtl ? 1 : -1)}
              onMoveRight={() => r.moveBy(b.key, rtl ? -1 : 1)}
              onOpenNewTab={() =>
                b.href && window.open(b.href, '_blank', 'noopener,noreferrer')
              }
            />
          </span>
        )
      })}
    </div>
  )
}
