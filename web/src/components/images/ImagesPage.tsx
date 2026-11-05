import { useMemo, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SearchGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { SkeletonList } from '@/components/kit'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { prefersReducedMotion } from '@/lib/reducedMotion'
import { useIsRoot } from '../../hooks/useIsRoot'
import {
  useImageInventory,
  type ImageEntry,
} from '../../queries/imageInventory'
import { DockerfileLintCard } from './DockerfileLintCard'
import { ImageCleanupDialog } from './ImageCleanupDialog'
import { ImageDetailSheet } from './ImageDetailSheet'
import { ImageList } from './ImageList'
import { ImageRemoveDialog } from './ImageRemoveDialog'
import { ImageTiles } from './ImageTiles'
import {
  IMAGE_FILTERS,
  filterImages,
  freedBytes,
  type ImageFilter,
  type ImageSort,
} from './imageFilters'
import { sizeLabel } from './imageFormat'

interface SectionProps {
  node: string
  gcOpen: boolean
  onGcOpenChange: (open: boolean) => void
  filter?: ImageFilter
  onFilterChange?: (next: ImageFilter) => void
}

const LEAVE_MS = 320

/** ImagesSection is the image inventory body: tiles, filters, list, cleanup and removal dialogs. */
export function ImagesSection({
  node,
  gcOpen,
  onGcOpenChange,
  filter,
  onFilterChange,
}: SectionProps) {
  const { t } = useTranslation('images')
  const [detail, setDetail] = useState<ImageEntry | null>(null)
  const [localFilter, setLocalFilter] = useState<ImageFilter>('all')
  const [query, setQuery] = useState('')
  const [sort, setSort] = useState<ImageSort>('size')
  const [selected, setSelected] = useState<ReadonlySet<string>>(new Set())
  const [leaving, setLeaving] = useState<ReadonlySet<string>>(new Set())
  const [removeOpen, setRemoveOpen] = useState(false)
  const [removeIds, setRemoveIds] = useState<string[]>([])
  const [removeCleanup, setRemoveCleanup] = useState(false)
  const [announce, setAnnounce] = useState('')
  const anchor = useRef<number | null>(null)
  const [snapshot, setSnapshot] = useState<ImageEntry[]>([])
  const isRoot = useIsRoot()
  const inventory = useImageInventory(node)
  const data = inventory.data
  const activeFilter = filter ?? localFilter

  const current = useMemo(() => data?.images ?? [], [data])
  // Removed rows stay mounted until their exit animation ends.
  const source = useMemo(() => {
    if (leaving.size === 0) return current
    const have = new Set(current.map((i) => i.id))
    return snapshot.filter((i) => have.has(i.id) || leaving.has(i.id))
  }, [current, leaving, snapshot])

  const visible = useMemo(
    () => filterImages(source, activeFilter, query, sort),
    [source, activeFilter, query, sort],
  )
  const selectable = visible.filter((i) => i.removable && !leaving.has(i.id))
  const chosen = current.filter((i) => selected.has(i.id))
  const allSelected =
    selectable.length > 0 && selectable.every((i) => selected.has(i.id))
  const inUseCount = current.filter((i) => i.in_use).length

  const setFilter = (next: ImageFilter) => {
    setLocalFilter(next)
    onFilterChange?.(next)
    anchor.current = null
  }

  const toggle = (image: ImageEntry, checked: boolean, range: boolean) => {
    const idx = visible.findIndex((i) => i.id === image.id)
    const from = anchor.current
    setSelected((prev) => {
      const next = new Set(prev)
      if (range && from !== null && idx >= 0) {
        const [lo, hi] = from < idx ? [from, idx] : [idx, from]
        for (const i of visible.slice(lo, hi + 1)) {
          if (!i.removable) continue
          if (checked) next.add(i.id)
          else next.delete(i.id)
        }
      } else if (checked) next.add(image.id)
      else next.delete(image.id)
      return next
    })
    anchor.current = idx
  }

  const toggleAll = (checked: boolean) => {
    setSelected((prev) => {
      const next = new Set(prev)
      for (const i of selectable) {
        if (checked) next.add(i.id)
        else next.delete(i.id)
      }
      return next
    })
  }

  const openRemove = (ids: string[], cleanup: boolean) => {
    setRemoveIds(ids)
    setRemoveCleanup(cleanup)
    setRemoveOpen(true)
  }

  const onRemoved = (ids: string[]) => {
    setAnnounce(t('remove.announce', { count: ids.length }))
    setSelected(new Set())
    setSnapshot(current)
    setLeaving(new Set(ids))
    window.setTimeout(
      () => {
        setLeaving(new Set())
      },
      prefersReducedMotion() ? 0 : LEAVE_MS,
    )
  }

  const managedRemovable = current.filter((i) => i.removable && !i.unmanaged)

  return (
    <>
      <div className="flex flex-col gap-4">
        {inventory.isPending ? <SkeletonList rows={6} /> : null}

        {inventory.isError ? (
          <Alert variant="destructive">
            <AlertTitle>{t('error.title')}</AlertTitle>
            <AlertDescription className="flex flex-wrap items-center gap-3">
              {inventory.error.message}
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  void inventory.refetch()
                }}
              >
                {t('error.retry')}
              </Button>
            </AlertDescription>
          </Alert>
        ) : null}

        {data ? (
          <>
            <ImageTiles
              totals={data.totals}
              inUseCount={inUseCount}
              canRemove={isRoot}
              onReview={() => {
                openRemove(
                  managedRemovable.map((i) => i.id),
                  true,
                )
              }}
            />
            {!isRoot ? (
              <p className="text-sm text-muted-foreground">{t('rootOnly')}</p>
            ) : null}
            {data.images.length === 0 ? null : (
              <>
                <div className="flex flex-wrap items-center gap-2">
                  <div
                    role="group"
                    aria-label={t('filters.label')}
                    className="flex flex-wrap gap-1.5"
                  >
                    {IMAGE_FILTERS.map((f) => (
                      <Button
                        key={f}
                        size="sm"
                        variant={activeFilter === f ? 'default' : 'outline'}
                        aria-pressed={activeFilter === f}
                        onClick={() => {
                          setFilter(f)
                        }}
                      >
                        {t(`filters.${f}`)}
                      </Button>
                    ))}
                  </div>
                  <div className="relative min-w-40 flex-1 sm:max-w-xs">
                    <SearchGlassIcon
                      size="1.2em"
                      aria-hidden="true"
                      className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-muted-foreground"
                    />
                    <Input
                      type="search"
                      value={query}
                      aria-label={t('filters.search')}
                      placeholder={t('filters.search')}
                      className="h-8 pl-8"
                      onChange={(e) => {
                        setQuery(e.target.value)
                      }}
                    />
                  </div>
                  <label className="ml-auto flex items-center gap-2 text-sm text-muted-foreground">
                    {t('filters.sort')}
                    <select
                      className="h-8 rounded-md border border-input bg-background px-2 text-sm text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
                      value={sort}
                      onChange={(e) => {
                        setSort(e.target.value === 'age' ? 'age' : 'size')
                      }}
                    >
                      <option value="size">{t('filters.sortSize')}</option>
                      <option value="age">{t('filters.sortAge')}</option>
                    </select>
                  </label>
                </div>
                {visible.length === 0 ? (
                  <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border p-8 text-center text-sm text-muted-foreground">
                    {t('filters.empty')}
                    <Button
                      variant="outline"
                      size="sm"
                      onClick={() => {
                        setFilter('all')
                        setQuery('')
                      }}
                    >
                      {t('filters.reset')}
                    </Button>
                  </div>
                ) : (
                  <ImageList
                    images={visible}
                    selected={selected}
                    leaving={leaving}
                    canRemove={isRoot}
                    allSelected={allSelected}
                    someSelected={selected.size > 0}
                    onToggleAll={toggleAll}
                    onToggle={toggle}
                    onOpen={setDetail}
                    onRemove={(image) => {
                      openRemove([image.id], false)
                    }}
                  />
                )}
              </>
            )}
            {isRoot && chosen.length > 0 ? (
              <div
                role="region"
                aria-label={t('bar.label')}
                className={cn(
                  'sticky bottom-4 z-20 flex flex-wrap items-center gap-3 rounded-lg border border-border bg-card p-3 shadow-raised',
                  'motion-safe:animate-in motion-safe:fade-in motion-safe:slide-in-from-bottom-4',
                )}
              >
                <span className="text-sm font-medium">
                  {t('bar.summary', {
                    count: chosen.length,
                    size: sizeLabel(freedBytes(chosen)),
                  })}
                </span>
                <div className="ml-auto flex w-full gap-2 sm:w-auto">
                  <Button
                    variant="outline"
                    className="flex-1 sm:flex-none"
                    onClick={() => {
                      setSelected(new Set())
                    }}
                  >
                    <XGlassIcon size="1.4em" aria-hidden="true" />
                    {t('bar.clear')}
                  </Button>
                  <Button
                    variant="destructive"
                    className="flex-1 sm:flex-none"
                    onClick={() => {
                      openRemove(
                        chosen.map((i) => i.id),
                        false,
                      )
                    }}
                  >
                    {t('bar.remove')}
                  </Button>
                </div>
              </div>
            ) : null}
            <ImageCleanupDialog
              node={node}
              open={gcOpen}
              onOpenChange={onGcOpenChange}
              candidates={managedRemovable}
              keepPerApp={data.retention.keep_per_app}
              reclaimBytes={data.totals.reclaimable_bytes}
            />
            <ImageRemoveDialog
              node={node}
              open={removeOpen}
              onOpenChange={setRemoveOpen}
              ids={removeIds}
              allowUnmanaged={removeCleanup}
              onRemoved={onRemoved}
            />
          </>
        ) : null}
        <p role="status" aria-live="polite" className="sr-only">
          {announce}
        </p>
      </div>
      <div className="mt-4">
        <DockerfileLintCard />
      </div>
      <ImageDetailSheet
        node={node}
        image={detail}
        onClose={() => {
          setDetail(null)
        }}
      />
    </>
  )
}
