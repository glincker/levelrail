import * as React from 'react'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { useNavigate } from '@tanstack/react-router'
import {
  ArrowsSplitIcon,
  CloudArrowUpIcon,
  CpuIcon,
  GearIcon,
  GlobeIcon,
  HardDrivesIcon,
  HeartbeatIcon,
  StackIcon,
  TreeStructureIcon,
  XIcon,
} from '@phosphor-icons/react/dist/ssr'
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

// One icon per GO_TARGETS key, matching the same choices navModel.tsx and
// commandPaletteData.tsx already use for these routes.
const TILE_ICONS: Record<string, React.ReactNode> = {
  a: <StackIcon />,
  n: <HardDrivesIcon />,
  s: <HeartbeatIcon />,
  d: <GlobeIcon />,
  b: <CloudArrowUpIcon />,
  l: <ArrowsSplitIcon />,
  p: <TreeStructureIcon />,
  m: <CpuIcon />,
  t: <GearIcon />,
}

// Decorative cascade (Stage Manager's stacked-thumbnail look). Purely a
// visual offset: it never changes DOM order, so tab/arrow-key order stays
// the plain reading order.
const CASCADE_TRANSFORM = [
  'sm:-translate-y-1.5 sm:-rotate-1',
  'sm:translate-y-1 sm:rotate-1',
  'sm:-translate-y-0.5 sm:rotate-0.5',
  'sm:translate-y-1.5 sm:-rotate-0.5',
]

interface Tile {
  key: string
  to: string
  label: string
}

// Roving-tabindex grid nav: arrows move/wrap, Home/End jump to an edge.
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
            'fixed inset-0 z-50 bg-black/50 [backdrop-filter:var(--glinui-blur-modal)] [-webkit-backdrop-filter:var(--glinui-blur-modal)]',
            animate &&
              'duration-200 data-open:animate-in data-open:fade-in-0 data-closed:animate-out data-closed:fade-out-0',
          )}
        />
        <DialogPrimitive.Popup
          data-slot="stage-overlay-popup"
          aria-label="Quick navigation"
          className={cn(
            'glinui-glass-surface fixed inset-4 z-50 flex flex-col overflow-y-auto p-6 outline-none sm:inset-8 sm:p-10 md:inset-16',
            animate &&
              'duration-200 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95',
          )}
        >
          <DialogPrimitive.Close
            data-slot="stage-overlay-close"
            render={
              <Button
                variant="ghost"
                className="absolute top-4 right-4"
                size="icon-sm"
              />
            }
          >
            <XIcon />
            <span className="sr-only">Close</span>
          </DialogPrimitive.Close>

          <div className="mx-auto max-w-xl text-center">
            <DialogTitle className="text-2xl font-semibold tracking-tight text-foreground">
              Quick navigation
            </DialogTitle>
            <DialogDescription className="mt-2 text-sm">
              Hold <Kbd>L</Kbd> any time to summon this. Pick a tile, or move
              between them with the arrow keys.
            </DialogDescription>
          </div>

          <div
            role="group"
            aria-label="Go to"
            className="mx-auto mt-10 grid w-full max-w-4xl grid-cols-2 gap-3 sm:grid-cols-3 md:grid-cols-4"
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
                  'flex flex-col items-start gap-2 rounded-[var(--glinui-radius-md)] border p-4 text-left outline-none transition-colors',
                  'border-[var(--glinui-glass-border)] bg-white/5 hover:bg-white/10',
                  'focus-visible:ring-2 focus-visible:ring-[var(--glinui-accent)]',
                  CASCADE_TRANSFORM[index % CASCADE_TRANSFORM.length],
                )}
              >
                <span className="flex size-9 shrink-0 items-center justify-center rounded-[var(--glinui-radius-sm)] bg-[var(--glinui-accent-active-bg)] text-[var(--glinui-accent)] [&_svg]:size-5">
                  {TILE_ICONS[tile.key]}
                </span>
                <span className="text-sm font-medium text-foreground">
                  {tile.label}
                </span>
                <span className="flex items-center gap-1" aria-hidden="true">
                  <Kbd>g</Kbd>
                  <Kbd>{tile.key}</Kbd>
                </span>
              </button>
            ))}
          </div>

          <div className="mx-auto mt-auto flex flex-wrap items-center justify-center gap-4 pt-10 text-xs text-muted-foreground">
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
