import { useMemo, useRef, useState } from 'react'
import type { PointerEvent } from 'react'
import { useTranslation } from 'react-i18next'
import {
  MinimizeGlassIcon,
  RotateCcwGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import type { ProjectTopologyGraph } from '../../types/projectTopology'
import { useInfraCanvas } from '../../hooks/useInfraCanvas'
import { healthOf, nodeIntersects } from '../../lib/infraCanvasLayout'
import { InfraCanvasEdges } from './InfraCanvasEdges'
import { InfraCanvasNode } from './InfraCanvasNode'
import { InfraCanvasPanel } from './InfraCanvasPanel'

const PAN_STEP = 80

export function InfraCanvas({
  projectId,
  graph,
}: {
  projectId: string
  graph: ProjectTopologyGraph
}) {
  const { t } = useTranslation('infraCanvas')
  const containerRef = useRef<HTMLDivElement>(null)
  const c = useInfraCanvas(projectId, graph, containerRef)
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const pan = useRef<{ x: number; y: number } | null>(null)

  const nodesById = useMemo(
    () => new Map(graph.nodes.map((n) => [n.id, n])),
    [graph.nodes],
  )
  const visibleNodes = useMemo(
    () =>
      graph.nodes.filter((n) => {
        const p = c.positions[n.id]
        return n.id === selectedId || (p && nodeIntersects(p, c.worldRect))
      }),
    [graph.nodes, c.positions, c.worldRect, selectedId],
  )
  const visibleIds = useMemo(
    () => new Set(visibleNodes.map((n) => n.id)),
    [visibleNodes],
  )
  const hidden = graph.nodes.length - visibleNodes.length
  const selected = selectedId ? nodesById.get(selectedId) : undefined

  const onBgDown = (e: PointerEvent<HTMLDivElement>) => {
    if (e.button !== 0) return
    e.currentTarget.setPointerCapture(e.pointerId)
    pan.current = { x: e.clientX, y: e.clientY }
    setSelectedId(null)
  }
  const onBgMove = (e: PointerEvent<HTMLDivElement>) => {
    const p = pan.current
    if (!p) return
    c.panBy(e.clientX - p.x, e.clientY - p.y)
    pan.current = { x: e.clientX, y: e.clientY }
  }
  const onBgUp = () => {
    pan.current = null
  }

  return (
    <div className="relative h-[70vh] min-h-[28rem] overflow-hidden rounded-xl border border-border bg-muted/20">
      <div
        ref={containerRef}
        role="group"
        aria-label={t('canvasLabel', { count: graph.nodes.length })}
        onPointerDown={onBgDown}
        onPointerMove={onBgMove}
        onPointerUp={onBgUp}
        onKeyDown={(e) => {
          if (e.target !== e.currentTarget) return
          if (e.key === 'Escape') setSelectedId(null)
          else if (e.key === 'ArrowLeft') c.panBy(PAN_STEP, 0)
          else if (e.key === 'ArrowRight') c.panBy(-PAN_STEP, 0)
          else if (e.key === 'ArrowUp') c.panBy(0, PAN_STEP)
          else if (e.key === 'ArrowDown') c.panBy(0, -PAN_STEP)
          else if (e.key === '+' || e.key === '=') c.zoomBy(1.2)
          else if (e.key === '-') c.zoomBy(1 / 1.2)
        }}
        tabIndex={0}
        className="absolute inset-0 touch-none cursor-grab focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing"
      >
        <div
          // Compositor-only: one transform on the layer, never per node.
          style={{
            transform: `translate3d(${c.view.x}px, ${c.view.y}px, 0) scale(${c.view.scale})`,
            transformOrigin: '0 0',
          }}
          className="absolute left-0 top-0 will-change-transform"
        >
          <InfraCanvasEdges
            edges={graph.edges}
            positions={c.positions}
            visible={visibleIds}
            highlighted={selectedId}
          />
          {visibleNodes.map((n) => {
            const pos = c.positions[n.id]
            if (!pos) return null
            return (
              <InfraCanvasNode
                key={n.id}
                node={n}
                position={pos}
                health={healthOf(n)}
                scale={c.view.scale}
                selected={n.id === selectedId}
                onSelect={setSelectedId}
                onMove={c.moveNode}
                onMoveEnd={c.persist}
              />
            )
          })}
        </div>
      </div>

      <div
        role="toolbar"
        aria-label={t('toolbar.label')}
        className="absolute bottom-3 left-3 flex items-center gap-1 rounded-lg border border-border bg-card p-1"
      >
        <Button
          variant="ghost"
          size="icon"
          onClick={() => c.zoomBy(1.2)}
          aria-label={t('toolbar.zoomIn')}
        >
          <SearchGlassIcon className="size-5" aria-hidden="true" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={() => c.zoomBy(1 / 1.2)}
          aria-label={t('toolbar.zoomOut')}
        >
          <SearchGlassIcon className="size-5" aria-hidden="true" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={c.fit}
          aria-label={t('toolbar.fit')}
        >
          <MinimizeGlassIcon className="size-5" aria-hidden="true" />
        </Button>
        <Button
          variant="ghost"
          size="icon"
          onClick={c.resetLayout}
          aria-label={t('toolbar.resetLayout')}
        >
          <RotateCcwGlassIcon className="size-5" aria-hidden="true" />
        </Button>
        <span className="px-2 text-xs text-muted-foreground" aria-live="polite">
          {t('toolbar.zoomLevel', { percent: Math.round(c.view.scale * 100) })}
        </span>
      </div>

      {hidden > 0 ? (
        <p className="absolute left-3 top-3 rounded-md bg-card px-2 py-1 text-xs text-muted-foreground">
          {t('toolbar.hidden', { count: hidden })}
        </p>
      ) : null}

      {selected ? (
        <InfraCanvasPanel
          node={selected}
          health={healthOf(selected)}
          nodes={nodesById}
          edges={graph.edges}
          onClose={() => setSelectedId(null)}
          onSelect={setSelectedId}
        />
      ) : null}
    </div>
  )
}
