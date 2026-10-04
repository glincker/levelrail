import * as React from 'react'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { useNavigate } from '@tanstack/react-router'
import { XIcon } from '@phosphor-icons/react/dist/ssr'
import {
  DialogPortal,
  DialogTitle,
  DialogDescription,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Kbd } from '@/components/commandPaletteEntries'
import { useReducedMotion } from '@/components/kit/useReducedMotion'
import { useExperimentalFeatures } from '@/hooks/useExperimental'
import { filterByFeature } from '@/lib/experimental'
import { BASE_DOCS, GO_TARGETS } from '@/lib/shortcuts'
import { cn } from '@/lib/utils'

interface Tile {
  key: string
  to: string
  label: string
}

// Roving-tabindex row nav: left/right move/wrap, Home/End jump to an edge.
// Up/down mirror left/right so an accidental vertical arrow still moves.
function nextTileIndex(
  key: string,
  index: number,
  count: number,
): number | undefined {
  if (key === 'ArrowRight' || key === 'ArrowDown') return (index + 1) % count
  if (key === 'ArrowLeft' || key === 'ArrowUp') {
    return (index - 1 + count) % count
  }
  if (key === 'Home') return 0
  if (key === 'End') return count - 1
  return undefined
}

export function StageOverlay({
  open: openProp,
  onOpenChange,
}: {
  open?: boolean
  onOpenChange?: (open: boolean) => void
} = {}) {
  const [internalOpen, setInternalOpen] = React.useState(false)
  const open = openProp ?? internalOpen
  const setOpen = React.useCallback(
    (next: boolean) => {
      setInternalOpen(next)
      onOpenChange?.(next)
    },
    [onOpenChange],
  )

  const navigate = useNavigate()
  const experimental = useExperimentalFeatures()
  const reducedMotion = useReducedMotion()
  const tileRefs = React.useRef<(HTMLButtonElement | null)[]>([])
  const [activeTile, setActiveTile] = React.useState(0)

  const tiles = React.useMemo<Tile[]>(
    () =>
      filterByFeature(
        Object.entries(GO_TARGETS).map(([key, target]) => ({
          key,
          to: target.to,
          label: target.label,
          feature: target.feature,
        })),
        experimental,
      ),
    [experimental],
  )

  // Reset roving focus to the first tile every time the overlay opens.
  const [prevOpen, setPrevOpen] = React.useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (open) setActiveTile(0)
  }

  const select = React.useCallback(
    (to: string) => {
      void navigate({ to })
      setOpen(false)
    },
    [navigate, setOpen],
  )

  function onTileKeyDown(e: React.KeyboardEvent, index: number) {
    const next = nextTileIndex(e.key, index, tiles.length)
    if (next === undefined) return
    e.preventDefault()
    setActiveTile(next)
    tileRefs.current[next]?.focus()
  }

  const animate = !reducedMotion

  return (
    <DialogPrimitive.Root
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
      }}
    >
      <DialogPortal>
        <DialogPrimitive.Backdrop
          data-slot="stage-overlay-backdrop"
          className={cn(
            'fixed inset-0 z-50 bg-black/30 [backdrop-filter:var(--glinui-blur-modal)] [-webkit-backdrop-filter:var(--glinui-blur-modal)]',
            animate &&
              'duration-200 data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0',
          )}
        />
        {/* Bottom-anchored strip, not a full-screen dialog: a quick "go to"
            glance, not a destination of its own. */}
        <DialogPrimitive.Popup
          data-slot="stage-overlay-popup"
          aria-label="Quick navigation"
          className={cn(
            'glinui-glass-surface fixed bottom-6 left-1/2 z-50 flex w-[calc(100vw-2rem)] max-w-3xl -translate-x-1/2 flex-col gap-2 p-3 outline-none',
            animate &&
              'duration-200 data-open:animate-in data-open:fade-in-0 data-open:slide-in-from-bottom-4 data-closed:animate-out data-closed:fade-out-0 data-closed:slide-out-to-bottom-4',
          )}
        >
          <DialogTitle className="sr-only">Quick navigation</DialogTitle>
          <DialogDescription className="sr-only">
            Hold L any time to summon this. Pick a destination, or move between
            them with the arrow keys. Press Escape to close.
          </DialogDescription>

          <div className="flex items-center gap-2">
            <span className="flex shrink-0 items-center gap-1.5 pl-1 text-xs font-medium text-muted-foreground">
              <Kbd>L</Kbd>
              <span>Go to</span>
            </span>

            <div
              role="group"
              aria-label="Go to"
              className="flex min-w-0 flex-1 items-center gap-1.5 overflow-x-auto py-0.5"
            >
              {tiles.map((tile, index) => (
                <button
                  key={tile.key}
                  ref={(el) => {
                    tileRefs.current[index] = el
                  }}
                  type="button"
                  tabIndex={index === activeTile ? 0 : -1}
                  aria-label={`Go to ${tile.label}`}
                  onClick={() => select(tile.to)}
                  onFocus={() => setActiveTile(index)}
                  onKeyDown={(e) => onTileKeyDown(e, index)}
                  className={cn(
                    'flex shrink-0 items-center gap-1.5 rounded-[var(--glinui-radius-sm)] border px-2.5 py-1.5 outline-none transition-colors',
                    'border-[var(--glinui-glass-border)] bg-white/5 hover:bg-white/10',
                    'focus-visible:ring-2 focus-visible:ring-[var(--glinui-accent)]',
                  )}
                >
                  <span
                    className="flex items-center gap-0.5"
                    aria-hidden="true"
                  >
                    <Kbd>g</Kbd>
                    <Kbd>{tile.key}</Kbd>
                  </span>
                  <span className="text-xs font-medium whitespace-nowrap text-foreground">
                    {tile.label}
                  </span>
                </button>
              ))}
            </div>

            <DialogPrimitive.Close
              data-slot="stage-overlay-close"
              render={
                <Button variant="ghost" className="shrink-0" size="icon-sm" />
              }
            >
              <XIcon />
              <span className="sr-only">Close</span>
            </DialogPrimitive.Close>
          </div>

          <div className="flex flex-wrap items-center gap-3 border-t border-[var(--glinui-glass-border)] pt-2 pl-1 text-[10px] text-muted-foreground">
            {BASE_DOCS.map((doc) => (
              <span key={doc.description} className="flex items-center gap-1.5">
                <span className="flex items-center gap-0.5">
                  {doc.keys.map((k) => (
                    <Kbd key={k}>{k}</Kbd>
                  ))}
                </span>
                {doc.description}
              </span>
            ))}
          </div>
        </DialogPrimitive.Popup>
      </DialogPortal>
    </DialogPrimitive.Root>
  )
}
