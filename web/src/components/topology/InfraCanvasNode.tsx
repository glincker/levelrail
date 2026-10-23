import { memo, useRef } from 'react'
import type { KeyboardEvent, PointerEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  ArrowsSplitIcon,
  CpuIcon,
  DatabaseIcon,
  GlobeIcon,
  HardDrivesIcon,
  StackIcon,
} from '@phosphor-icons/react/dist/ssr'
import { cn } from '@/lib/utils'
import type { ProjectTopologyNode } from '../../types/projectTopology'
import {
  type CanvasHealth,
  type Point,
  NODE_HEIGHT,
  NODE_WIDTH,
} from '../../lib/infraCanvasLayout'

const KIND_ICON = {
  app: StackIcon,
  database: DatabaseIcon,
  volume: HardDrivesIcon,
  domain: GlobeIcon,
  load_balancer: ArrowsSplitIcon,
  node: CpuIcon,
} as const

// Shape and icon carry the kind too, so it never relies on color alone.
const KIND_CLASS: Partial<Record<ProjectTopologyNode['kind'], string>> = {
  load_balancer: 'border-l-4 border-l-ring',
  node: 'border-dashed bg-muted/40',
}

const HEALTH_DOT: Record<CanvasHealth, string> = {
  healthy: 'bg-green-500 dark:bg-green-400',
  deploying: 'bg-amber-500 motion-safe:animate-pulse',
  failed: 'bg-destructive',
  idle: 'bg-muted-foreground/40',
}

const NUDGE = 16
const NUDGE_FAST = 64
const DRAG_THRESHOLD = 3

interface Props {
  node: ProjectTopologyNode
  position: Point
  health: CanvasHealth
  scale: number
  selected: boolean
  onSelect: (id: string) => void
  onMove: (id: string, to: Point) => void
  onMoveEnd: () => void
}

function InfraCanvasNodeBase({
  node,
  position,
  health,
  scale,
  selected,
  onSelect,
  onMove,
  onMoveEnd,
}: Props) {
  const { t } = useTranslation('infraCanvas')
  const drag = useRef<{
    px: number
    py: number
    ox: number
    oy: number
    moved: boolean
  } | null>(null)
  const Icon = KIND_ICON[node.kind]
  const healthLabel = node.draft ? t('status.draft') : t(`status.${health}`)

  const onPointerDown = (e: PointerEvent<HTMLButtonElement>) => {
    if (e.button !== 0) return
    e.stopPropagation()
    e.currentTarget.setPointerCapture(e.pointerId)
    drag.current = {
      px: e.clientX,
      py: e.clientY,
      ox: position.x,
      oy: position.y,
      moved: false,
    }
  }

  const onPointerMove = (e: PointerEvent<HTMLButtonElement>) => {
    const d = drag.current
    if (!d) return
    const dx = e.clientX - d.px
    const dy = e.clientY - d.py
    if (!d.moved && Math.hypot(dx, dy) < DRAG_THRESHOLD) return
    d.moved = true
    onMove(node.id, { x: d.ox + dx / scale, y: d.oy + dy / scale })
  }

  const onPointerUp = (e: PointerEvent<HTMLButtonElement>) => {
    const d = drag.current
    drag.current = null
    if (e.currentTarget.hasPointerCapture(e.pointerId)) {
      e.currentTarget.releasePointerCapture(e.pointerId)
    }
    if (d?.moved) onMoveEnd()
    else onSelect(node.id)
  }

  const onKeyDown = (e: KeyboardEvent<HTMLButtonElement>) => {
    const step = e.shiftKey ? NUDGE_FAST : NUDGE
    const delta: Record<string, Point> = {
      ArrowLeft: { x: -step, y: 0 },
      ArrowRight: { x: step, y: 0 },
      ArrowUp: { x: 0, y: -step },
      ArrowDown: { x: 0, y: step },
    }
    const d = delta[e.key]
    if (d) {
      e.preventDefault()
      onMove(node.id, { x: position.x + d.x, y: position.y + d.y })
      onMoveEnd()
    } else if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onSelect(node.id)
    }
  }

  return (
    <button
      type="button"
      data-node-id={node.id}
      aria-label={`${t(`kind.${node.kind}`)} ${node.label}, ${healthLabel}`}
      aria-pressed={selected}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onKeyDown={onKeyDown}
      // World coordinates are dynamic per node, so a transform is the one
      // thing Tailwind classes cannot express.
      style={{
        width: NODE_WIDTH,
        height: NODE_HEIGHT,
        transform: `translate3d(${position.x}px, ${position.y}px, 0)`,
      }}
      className={cn(
        'absolute left-0 top-0 flex touch-none cursor-grab select-none items-center gap-2 rounded-lg border bg-card px-3 text-left text-sm active:cursor-grabbing',
        'focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring',
        selected ? 'border-ring' : 'border-border hover:border-ring',
        node.draft && 'border-dashed',
        KIND_CLASS[node.kind],
      )}
    >
      <Icon
        className="size-4 shrink-0 text-muted-foreground"
        aria-hidden="true"
      />
      <span className="min-w-0 flex-1">
        <span className="block truncate font-medium text-foreground">
          {node.label}
        </span>
        <span className="block truncate text-xs text-muted-foreground">
          {t(`kind.${node.kind}`)}
        </span>
      </span>
      {node.kind === 'app' || node.kind === 'database' ? (
        <span
          className={cn('size-2.5 shrink-0 rounded-full', HEALTH_DOT[health])}
          role="img"
          aria-label={healthLabel}
        />
      ) : null}
    </button>
  )
}

export const InfraCanvasNode = memo(InfraCanvasNodeBase)
export { NODE_HEIGHT }
