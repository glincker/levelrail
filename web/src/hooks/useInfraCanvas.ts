import type { RefObject } from 'react'
import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { ProjectTopologyGraph } from '../types/projectTopology'
import {
  type CanvasPositions,
  type Point,
  type Viewport,
  clampScale,
  defaultPositions,
  fitViewport,
  mergePositions,
  parseSavedPositions,
  positionsStorageKey,
  visibleWorldRect,
  zoomAt,
} from '../lib/infraCanvasLayout'

function readSaved(projectId: string): CanvasPositions {
  try {
    return parseSavedPositions(
      window.localStorage.getItem(positionsStorageKey(projectId)),
    )
  } catch {
    return {}
  }
}

function writeSaved(projectId: string, positions: CanvasPositions) {
  try {
    window.localStorage.setItem(
      positionsStorageKey(projectId),
      JSON.stringify(positions),
    )
  } catch {
    // Private mode or quota: the layout just will not persist.
  }
}

// Owns pan/zoom, node positions and their per-project persistence. View
// changes are plain state applied as one CSS transform on the canvas
// layer, so panning never re-lays-out any node.
export function useInfraCanvas(
  projectId: string,
  graph: ProjectTopologyGraph,
  containerRef: RefObject<HTMLDivElement | null>,
) {
  const [size, setSize] = useState({ width: 0, height: 0 })
  const [view, setView] = useState<Viewport>({ x: 0, y: 0, scale: 1 })
  const [saved, setSaved] = useState<CanvasPositions>(() =>
    readSaved(projectId),
  )
  const fitted = useRef(false)

  const positions = useMemo(() => mergePositions(graph, saved), [graph, saved])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const measure = () =>
      setSize({ width: el.clientWidth, height: el.clientHeight })
    measure()
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [containerRef])

  useEffect(() => {
    if (fitted.current || size.width === 0) return
    fitted.current = true
    setView(fitViewport(positions, size))
  }, [positions, size])

  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    const onWheel = (e: WheelEvent) => {
      e.preventDefault()
      const rect = el.getBoundingClientRect()
      if (e.ctrlKey || e.metaKey) {
        const anchor = { x: e.clientX - rect.left, y: e.clientY - rect.top }
        setView((v) => ({
          ...v,
          ...zoomAt(v, v.scale * Math.exp(-e.deltaY * 0.01), anchor),
        }))
      } else {
        setView((v) => ({ ...v, x: v.x - e.deltaX, y: v.y - e.deltaY }))
      }
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [containerRef])

  const panBy = useCallback((dx: number, dy: number) => {
    setView((v) => ({ ...v, x: v.x + dx, y: v.y + dy }))
  }, [])

  const zoomBy = useCallback(
    (factor: number) => {
      setView((v) => ({
        ...v,
        ...zoomAt(v, clampScale(v.scale * factor), {
          x: size.width / 2,
          y: size.height / 2,
        }),
      }))
    },
    [size],
  )

  const fit = useCallback(() => {
    setView(fitViewport(positions, size))
  }, [positions, size])

  const commit = useCallback(
    (next: CanvasPositions) => {
      setSaved(next)
      writeSaved(projectId, next)
    },
    [projectId],
  )

  const moveNode = useCallback((id: string, to: Point) => {
    setSaved((prev) => ({ ...prev, [id]: to }))
  }, [])

  const persist = useCallback(() => {
    setSaved((prev) => {
      writeSaved(projectId, prev)
      return prev
    })
  }, [projectId])

  const resetLayout = useCallback(() => {
    const base = defaultPositions(graph)
    commit({})
    setView(fitViewport(base, size))
  }, [graph, size, commit])

  const worldRect = useMemo(() => visibleWorldRect(view, size), [view, size])

  return {
    size,
    view,
    positions,
    worldRect,
    panBy,
    zoomBy,
    fit,
    moveNode,
    persist,
    resetLayout,
  }
}
