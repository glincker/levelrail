import { useRef } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  ArchiveGlassIcon,
  DatabaseGlassIcon,
  GlobeGlassIcon,
  LayersGlassIcon,
  ServerGlassIcon,
  Share2GlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Checkbox } from '@/components/ui/checkbox'
import { resourceKey, resourceTarget } from '@/lib/resourceLinks'
import type { ConsoleResource, ResourceKind } from '@/queries/resources'

const ROW_HEIGHT = 48
const GRID =
  'grid grid-cols-[1.5rem_minmax(0,1fr)] items-center gap-3 sm:grid-cols-[1.5rem_minmax(0,1.2fr)_minmax(0,1fr)_minmax(0,1.2fr)]'

const KIND_ICON: Record<ResourceKind, React.ReactNode> = {
  app: <LayersGlassIcon size="1.2em" />,
  database: <DatabaseGlassIcon size="1.2em" />,
  node: <ServerGlassIcon size="1.2em" />,
  domain: <GlobeGlassIcon size="1.2em" />,
  bucket: <ArchiveGlassIcon size="1.2em" />,
  network: <Share2GlassIcon size="1.2em" />,
}

function Row({
  r,
  checked,
  onToggle,
  onTag,
}: {
  r: ConsoleResource
  checked: boolean
  onToggle: () => void
  onTag: (tag: string) => void
}) {
  const { t } = useTranslation('resources')
  const navigate = useNavigate()
  const target = resourceTarget(r)
  return (
    <div
      role="row"
      aria-selected={checked}
      className={`${GRID} h-12 border-b border-border px-3 text-sm`}
    >
      <span role="cell">
        <Checkbox
          checked={checked}
          onCheckedChange={onToggle}
          aria-label={t('list.selectOne', { name: r.name })}
        />
      </span>
      <span role="cell" className="flex min-w-0 items-center gap-2">
        <span className="text-muted-foreground" aria-hidden="true">
          {KIND_ICON[r.kind]}
        </span>
        <button
          type="button"
          className="truncate font-medium text-foreground hover:underline"
          onClick={() => void navigate(target)}
        >
          {r.name}
        </button>
        <span className="hidden shrink-0 text-xs text-muted-foreground md:inline">
          {t(`kind.${r.kind}One`)}
        </span>
      </span>
      <span
        role="cell"
        className="hidden truncate text-muted-foreground sm:block"
      >
        {r.detail}
      </span>
      <span
        role="cell"
        className="hidden flex-wrap gap-1 overflow-hidden sm:flex"
      >
        {r.tags.slice(0, 3).map((tag) => (
          <button key={tag} type="button" onClick={() => onTag(tag)}>
            <Badge variant="muted">{tag}</Badge>
          </button>
        ))}
        {r.tags.length > 3 ? (
          <Badge variant="outline">+{r.tags.length - 3}</Badge>
        ) : null}
      </span>
    </div>
  )
}

// Always virtualized: an estate can reach thousands of resources.
export function ResourceRows({
  items,
  selected,
  onToggle,
  onToggleAll,
  onTag,
}: {
  items: ConsoleResource[]
  selected: Set<string>
  onToggle: (r: ConsoleResource) => void
  onToggleAll: () => void
  onTag: (tag: string) => void
}) {
  const { t } = useTranslation('resources')
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualizer = useVirtualizer({
    count: items.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT,
    overscan: 10,
  })
  const allChecked =
    items.length > 0 && items.every((r) => selected.has(resourceKey(r)))
  return (
    <div
      role="table"
      aria-label={t('list.label')}
      aria-rowcount={items.length + 1}
      className="rounded-lg border border-border"
    >
      <div
        role="row"
        className={`${GRID} border-b border-border px-3 py-2 text-xs font-medium text-muted-foreground`}
      >
        <span role="columnheader">
          <Checkbox
            checked={allChecked}
            onCheckedChange={onToggleAll}
            aria-label={t('list.selectAll')}
          />
        </span>
        <span role="columnheader">{t('list.name')}</span>
        <span role="columnheader" className="hidden sm:block">
          {t('list.detail')}
        </span>
        <span role="columnheader" className="hidden sm:block">
          {t('list.tags')}
        </span>
      </div>
      <div ref={parentRef} className="max-h-[65vh] overflow-auto">
        <div
          className="relative w-full"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((v) => {
            const r = items[v.index]
            if (!r) return null
            return (
              <div
                key={resourceKey(r)}
                className="absolute left-0 top-0 w-full"
                style={{ transform: `translateY(${v.start}px)` }}
              >
                <Row
                  r={r}
                  checked={selected.has(resourceKey(r))}
                  onToggle={() => onToggle(r)}
                  onTag={onTag}
                />
              </div>
            )
          })}
        </div>
      </div>
    </div>
  )
}
