import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import type { KeyboardEvent, PointerEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useQueries } from '@tanstack/react-query'
import {
  DownloadGlassIcon,
  MaximizeGlassIcon,
  MinusGlassIcon,
  PlusGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import {
  tableStructureQuery,
  useDatabaseFullSchema,
} from '../../../queries/databaseViewer'
import { tableKey } from '../../../lib/explorerTree'
import { buildSchemaSvg } from '../../../lib/schemaMapExport'
import {
  HEADER_HEIGHT,
  ROW_HEIGHT,
  layoutSchemaMap,
  routeEdge,
  type MapEdge,
  type MapTable,
} from '../../../lib/schemaMapLayout'
import { classifyExplorerError } from '../../../lib/explorerError'
import { ExplorerLoading, ExplorerReadError } from './ExplorerStates'

const MAX_TABLES = Number(import.meta.env.VITE_SCHEMA_MAP_MAX_TABLES) || 40
const MIN_ZOOM = 0.2
const MAX_ZOOM = 2.5
const ZOOM_STEP = 1.2
const FIT_PAD = 24

interface View {
  x: number
  y: number
  k: number
}

function clampZoom(k: number): number {
  return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, k))
}

// ER view of the first tables in the schema. Columns come from the full
// schema; foreign keys only exist per table, so they load for the visible
// tables only.
export default function SchemaMap({
  databaseName,
  onOpen,
}: {
  databaseName: string
  onOpen: (schema: string, table: string) => void
}) {
  const { t } = useTranslation('databases')
  const full = useDatabaseFullSchema(databaseName, true)
  const [filter, setFilter] = useState('')
  const [hover, setHover] = useState<string | null>(null)
  const [view, setView] = useState<View>({ x: 0, y: 0, k: 1 })
  const frame = useRef<HTMLDivElement>(null)
  const drag = useRef<{ x: number; y: number } | null>(null)

  const all = useMemo(() => {
    const out: MapTable[] = []
    for (const s of full.data?.schemas ?? []) {
      for (const tb of s.tables) {
        if (tb.kind !== 'table') continue
        out.push({
          id: tableKey(s.name, tb.name),
          schema: s.name,
          name: tb.name,
          columns: tb.columns.map((c) => ({
            name: c.name,
            type: c.type,
            primaryKey: c.primary_key,
          })),
        })
      }
    }
    return out
  }, [full.data])

  const matched = useMemo(() => {
    const q = filter.trim().toLowerCase()
    return q ? all.filter((tb) => tb.id.toLowerCase().includes(q)) : all
  }, [all, filter])
  const tables = useMemo(() => matched.slice(0, MAX_TABLES), [matched])

  const structures = useQueries({
    queries: tables.map((tb) =>
      tableStructureQuery(databaseName, tb.schema, tb.name, true),
    ),
  })

  const edges = useMemo(() => {
    const ids = new Set(tables.map((tb) => tb.id))
    const out: MapEdge[] = []
    structures.forEach((q, i) => {
      for (const fk of q.data?.foreign_keys ?? []) {
        const to = tableKey(fk.ref_schema, fk.ref_table)
        if (!ids.has(to)) continue
        out.push({
          id: `${tables[i].id}:${fk.name}`,
          from: tables[i].id,
          to,
          fromColumn: fk.columns[0],
        })
      }
    })
    return out
  }, [structures, tables])

  const loadingFks = structures.some((q) => q.isLoading)
  const layout = useMemo(() => layoutSchemaMap(tables, edges), [tables, edges])
  const placed = useMemo(
    () => new Map(layout.nodes.map((n) => [n.id, n])),
    [layout],
  )
  const related = useMemo(() => {
    if (!hover) return null
    const s = new Set([hover])
    for (const e of edges) {
      if (e.from === hover) s.add(e.to)
      if (e.to === hover) s.add(e.from)
    }
    return s
  }, [hover, edges])

  const fit = useCallback(() => {
    const el = frame.current
    if (!el || layout.nodes.length === 0) return
    const k = clampZoom(
      Math.min(
        (el.clientWidth - FIT_PAD) / layout.width,
        (el.clientHeight - FIT_PAD) / layout.height,
        1,
      ),
    )
    setView({
      k,
      x: (el.clientWidth - layout.width * k) / 2,
      y: Math.max(FIT_PAD / 2, (el.clientHeight - layout.height * k) / 2),
    })
  }, [layout])

  useEffect(() => {
    fit()
  }, [fit])

  const zoomAt = useCallback((factor: number, cx: number, cy: number) => {
    setView((v) => {
      const k = clampZoom(v.k * factor)
      const r = k / v.k
      return { k, x: cx - (cx - v.x) * r, y: cy - (cy - v.y) * r }
    })
  }, [])

  // React wheel handlers are passive, so page scroll is cancelled natively.
  useEffect(() => {
    const el = frame.current
    if (!el) return
    const onWheel = (ev: WheelEvent) => {
      ev.preventDefault()
      const r = el.getBoundingClientRect()
      zoomAt(
        ev.deltaY < 0 ? ZOOM_STEP : 1 / ZOOM_STEP,
        ev.clientX - r.left,
        ev.clientY - r.top,
      )
    }
    el.addEventListener('wheel', onWheel, { passive: false })
    return () => el.removeEventListener('wheel', onWheel)
  }, [zoomAt, layout.nodes.length])

  function zoomButton(factor: number) {
    const el = frame.current
    if (el) zoomAt(factor, el.clientWidth / 2, el.clientHeight / 2)
  }

  function onPointerDown(ev: PointerEvent<HTMLDivElement>) {
    if ((ev.target as Element).closest('[data-node]')) return
    drag.current = { x: ev.clientX, y: ev.clientY }
    ev.currentTarget.setPointerCapture(ev.pointerId)
  }
  function onPointerMove(ev: PointerEvent<HTMLDivElement>) {
    const d = drag.current
    if (!d) return
    const dx = ev.clientX - d.x
    const dy = ev.clientY - d.y
    drag.current = { x: ev.clientX, y: ev.clientY }
    setView((v) => ({ ...v, x: v.x + dx, y: v.y + dy }))
  }
  function onPointerUp() {
    drag.current = null
  }

  function focusTable(id: string) {
    const n = placed.get(id)
    const el = frame.current
    if (!n || !el) return
    setHover(id)
    setView((v) => ({
      ...v,
      x: el.clientWidth / 2 - (n.x + n.w / 2) * v.k,
      y: el.clientHeight / 2 - (n.y + n.h / 2) * v.k,
    }))
  }

  function onSearchKey(ev: KeyboardEvent<HTMLInputElement>) {
    if (ev.key === 'Enter' && tables[0]) focusTable(tables[0].id)
  }

  function exportSvg() {
    const svg = buildSchemaSvg(layout, tables, edges, (n) =>
      t('viewer.schemaMap.more', { count: n }),
    )
    const url = URL.createObjectURL(
      new Blob([svg], { type: 'image/svg+xml;charset=utf-8' }),
    )
    const a = document.createElement('a')
    a.href = url
    a.download = `${databaseName}-schema-map.svg`
    a.click()
    URL.revokeObjectURL(url)
  }

  if (full.isLoading) return <ExplorerLoading />
  if (full.error) {
    return (
      <ExplorerReadError
        databaseName={databaseName}
        info={classifyExplorerError(full.error)}
        retrying={full.isFetching}
        onRetry={() => void full.refetch()}
      />
    )
  }
  if (all.length === 0) {
    return (
      <p className="rounded-lg border border-border bg-muted/30 px-4 py-6 text-center text-sm text-muted-foreground">
        {t('viewer.schemaMap.noTables')}
      </p>
    )
  }

  const fkError = structures.some((q) => q.isError)

  return (
    <div className="space-y-2">
      <div className="flex flex-wrap items-center gap-2">
        <div className="relative w-full max-w-64">
          <SearchGlassIcon
            className="pointer-events-none absolute top-1/2 left-2 size-4 -translate-y-1/2"
            aria-hidden="true"
          />
          <Input
            value={filter}
            onChange={(ev) => setFilter(ev.target.value)}
            onKeyDown={onSearchKey}
            aria-label={t('viewer.schemaMap.search')}
            placeholder={t('viewer.schemaMap.search')}
            className="pl-8"
          />
        </div>
        <div
          role="group"
          aria-label={t('viewer.schemaMap.zoomGroup')}
          className="flex items-center gap-1"
        >
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t('viewer.schemaMap.zoomOut')}
            onClick={() => zoomButton(1 / ZOOM_STEP)}
          >
            <MinusGlassIcon className="size-4" aria-hidden="true" />
          </Button>
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t('viewer.schemaMap.zoomIn')}
            onClick={() => zoomButton(ZOOM_STEP)}
          >
            <PlusGlassIcon className="size-4" aria-hidden="true" />
          </Button>
          <Button
            type="button"
            variant="outline"
            size="icon"
            aria-label={t('viewer.schemaMap.fit')}
            onClick={fit}
          >
            <MaximizeGlassIcon className="size-4" aria-hidden="true" />
          </Button>
        </div>
        <Button type="button" variant="outline" size="sm" onClick={exportSvg}>
          <DownloadGlassIcon className="size-4" aria-hidden="true" />
          {t('viewer.schemaMap.export')}
        </Button>
        <p className="text-xs text-muted-foreground" role="status">
          {matched.length > MAX_TABLES
            ? t('viewer.schemaMap.capped', {
                shown: MAX_TABLES,
                total: matched.length,
              })
            : t('viewer.schemaMap.count', { count: tables.length })}
          {loadingFks ? ` | ${t('viewer.schemaMap.loadingKeys')}` : ''}
          {!loadingFks && !fkError && edges.length === 0
            ? ` | ${t('viewer.schemaMap.noKeys')}`
            : ''}
          {fkError ? ` | ${t('viewer.schemaMap.keysError')}` : ''}
        </p>
      </div>

      {tables.length === 0 ? (
        <p className="rounded-lg border border-border bg-muted/30 px-4 py-6 text-center text-sm text-muted-foreground">
          {t('viewer.schemaMap.noMatch')}
        </p>
      ) : (
        <div
          ref={frame}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={onPointerUp}
          onPointerCancel={onPointerUp}
          className="h-[min(70vh,44rem)] min-h-80 cursor-grab touch-none overflow-hidden rounded-lg border border-border bg-muted/20 active:cursor-grabbing"
        >
          <svg
            role="group"
            aria-label={t('viewer.schemaMap.label')}
            className="size-full text-foreground"
          >
            <defs>
              <marker
                id="schema-map-foot"
                viewBox="0 0 10 10"
                refX="9"
                refY="5"
                markerWidth="10"
                markerHeight="10"
                orient="auto-start-reverse"
              >
                <path
                  d="M0 5 L10 0 M0 5 L10 10 M0 5 L10 5"
                  className="fill-none stroke-current"
                />
              </marker>
            </defs>
            <g transform={`translate(${view.x} ${view.y}) scale(${view.k})`}>
              {edges.map((e) => {
                const a = placed.get(e.from)
                const b = placed.get(e.to)
                if (!a || !b) return null
                const lit = hover === e.from || hover === e.to
                return (
                  <path
                    key={e.id}
                    d={routeEdge(a, b, e.fromColumn).d}
                    markerEnd="url(#schema-map-foot)"
                    className={cn(
                      'fill-none stroke-muted-foreground text-muted-foreground',
                      lit && 'stroke-primary text-primary',
                      related && !lit && 'opacity-20',
                    )}
                    strokeWidth={lit ? 2 : 1.25}
                  />
                )
              })}
              {layout.nodes.map((n) => {
                const tb = tables.find((x) => x.id === n.id)
                if (!tb) return null
                const dim = related !== null && !related.has(n.id)
                const label = `${tb.schema}.${tb.name}`
                return (
                  <g
                    key={n.id}
                    data-node
                    role="button"
                    tabIndex={0}
                    aria-label={t('viewer.schemaMap.openTable', {
                      table: label,
                    })}
                    transform={`translate(${n.x} ${n.y})`}
                    className={cn(
                      'cursor-pointer outline-none',
                      dim && 'opacity-30',
                    )}
                    onPointerEnter={() => setHover(n.id)}
                    onPointerLeave={() => setHover(null)}
                    onFocus={() => setHover(n.id)}
                    onBlur={() => setHover(null)}
                    onClick={() => onOpen(tb.schema, tb.name)}
                    onKeyDown={(ev) => {
                      if (ev.key === 'Enter') onOpen(tb.schema, tb.name)
                    }}
                  >
                    <rect
                      width={n.w}
                      height={n.h}
                      rx={6}
                      className={cn(
                        'fill-card stroke-border',
                        hover === n.id && 'stroke-primary',
                      )}
                      strokeWidth={hover === n.id ? 2 : 1}
                    />
                    <text
                      x={10}
                      y={18}
                      className="fill-current text-xs font-semibold"
                    >
                      {label}
                    </text>
                    {n.shown.map((c, i) => (
                      <g
                        key={c.name}
                        transform={`translate(0 ${HEADER_HEIGHT + i * ROW_HEIGHT})`}
                      >
                        <text
                          x={10}
                          y={13}
                          className={cn(
                            'fill-current text-xs',
                            c.primaryKey && 'font-bold',
                          )}
                        >
                          {c.name}
                        </text>
                        <text
                          x={n.w - 10}
                          y={13}
                          textAnchor="end"
                          className="fill-muted-foreground text-xs"
                        >
                          {c.type}
                        </text>
                      </g>
                    ))}
                    {n.more > 0 ? (
                      <text
                        x={10}
                        y={HEADER_HEIGHT + n.shown.length * ROW_HEIGHT + 13}
                        className="fill-muted-foreground text-xs"
                      >
                        {t('viewer.schemaMap.more', { count: n.more })}
                      </text>
                    ) : null}
                  </g>
                )
              })}
            </g>
          </svg>
        </div>
      )}
    </div>
  )
}
