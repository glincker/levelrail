import { useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import { TrashGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Checkbox } from '@/components/ui/checkbox'
import { RelativeTime } from '@/components/kit'
import { cn } from '@/lib/utils'
import type { ImageEntry } from '../../queries/imageInventory'
import { imageTitle, shortId, sizeLabel, uniqueApps } from './imageFormat'

export const VIRTUALIZE_ABOVE = 50
const ROW_HEIGHT_PX = 64
const GRID =
  'grid grid-cols-[1.5rem_minmax(0,1fr)_auto] md:grid-cols-[1.5rem_minmax(0,2fr)_6rem_minmax(0,1.2fr)_minmax(0,1.6fr)_7rem_6rem] items-center gap-3 px-4'

interface RowProps {
  image: ImageEntry
  selected: boolean
  leaving: boolean
  canRemove: boolean
  onToggle: (image: ImageEntry, checked: boolean, range: boolean) => void
  onOpen: (image: ImageEntry) => void
  onRemove: (image: ImageEntry) => void
}

function ImageRow({
  image,
  selected,
  leaving,
  canRemove,
  onToggle,
  onOpen,
  onRemove,
}: RowProps) {
  const { t } = useTranslation('images')
  const apps = uniqueApps(image)
  const title = imageTitle(image) ?? t('untagged')
  const blocked =
    image.blocked_reason ?? image.protect_reason ?? t('row.notRemovable')
  const statusText = image.removable
    ? t('status.removable')
    : t('status.kept', { reason: blocked })

  return (
    <div
      data-leaving={leaving ? 'true' : undefined}
      onClick={() => {
        onOpen(image)
      }}
      className={cn(
        GRID,
        'h-16 cursor-pointer overflow-hidden border-b border-border text-sm hover:bg-muted/50 motion-safe:transition-[height,opacity] motion-safe:duration-300',
        selected && 'bg-muted/40',
        leaving && 'pointer-events-none h-0 border-b-0 opacity-0',
      )}
    >
      <span
        className="flex items-center"
        title={!image.removable ? blocked : undefined}
        onClick={(e) => {
          e.stopPropagation()
        }}
      >
        {canRemove ? (
          <Checkbox
            checked={selected}
            disabled={!image.removable}
            aria-label={t('row.select', { name: title })}
            onCheckedChange={(checked, details) => {
              const ev = details.event
              onToggle(image, checked, 'shiftKey' in ev && ev.shiftKey === true)
            }}
          />
        ) : null}
      </span>
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation()
          onOpen(image)
        }}
        className="min-w-0 rounded-sm text-left focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
      >
        <span className="flex items-center gap-2">
          <span className="truncate font-medium">{title}</span>
          {image.unmanaged ? (
            <Badge variant="muted">{t('badge.other')}</Badge>
          ) : null}
          {image.in_use ? (
            <Badge variant="info">{t('badge.inUse')}</Badge>
          ) : null}
        </span>
        <span className="block truncate font-mono text-xs text-muted-foreground">
          {shortId(image.id)}
          <span className="md:hidden">
            {' '}
            · {sizeLabel(image.size_bytes)} · {statusText}
          </span>
        </span>
      </button>
      <span className="hidden tabular-nums md:block">
        {sizeLabel(image.size_bytes)}
      </span>
      <span className="hidden truncate text-muted-foreground md:block">
        {apps.length > 0 ? apps.join(', ') : '-'}
      </span>
      <span
        className={cn(
          'hidden truncate md:block',
          image.removable
            ? 'font-medium text-tone-warning'
            : 'text-muted-foreground',
        )}
        title={statusText}
      >
        {statusText}
      </span>
      <span className="hidden text-muted-foreground md:block">
        <RelativeTime at={image.created_at} />
      </span>
      <span
        className="flex justify-end"
        onClick={(e) => {
          e.stopPropagation()
        }}
      >
        {canRemove && image.removable ? (
          <Button
            variant="ghost"
            size="sm"
            aria-label={t('row.removeNamed', { name: title })}
            onClick={() => {
              onRemove(image)
            }}
          >
            <TrashGlassIcon size="1.4em" aria-hidden="true" />
            <span className="hidden md:inline">{t('row.remove')}</span>
          </Button>
        ) : null}
      </span>
    </div>
  )
}

interface ListProps {
  images: ImageEntry[]
  selected: ReadonlySet<string>
  leaving: ReadonlySet<string>
  canRemove: boolean
  allSelected: boolean
  someSelected: boolean
  onToggleAll: (checked: boolean) => void
  onToggle: RowProps['onToggle']
  onOpen: RowProps['onOpen']
  onRemove: RowProps['onRemove']
}

export function ImageList({
  images,
  selected,
  leaving,
  canRemove,
  allSelected,
  someSelected,
  onToggleAll,
  onToggle,
  onOpen,
  onRemove,
}: ListProps) {
  const { t } = useTranslation('images')
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualize = images.length > VIRTUALIZE_ABOVE
  const virtualizer = useVirtualizer({
    count: images.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 8,
  })

  const row = (image: ImageEntry) => (
    <ImageRow
      image={image}
      selected={selected.has(image.id)}
      leaving={leaving.has(image.id)}
      canRemove={canRemove}
      onToggle={onToggle}
      onOpen={onOpen}
      onRemove={onRemove}
    />
  )

  return (
    <div
      ref={parentRef}
      className={cn(
        'overflow-auto rounded-lg border border-border bg-card',
        virtualize && 'h-[60vh]',
      )}
    >
      <div
        className={cn(
          GRID,
          'sticky top-0 z-10 border-b border-border bg-card py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase',
        )}
      >
        <span className="flex items-center">
          {canRemove ? (
            <Checkbox
              checked={allSelected}
              indeterminate={someSelected && !allSelected}
              aria-label={t('row.selectAll')}
              onCheckedChange={(checked) => {
                onToggleAll(checked)
              }}
            />
          ) : null}
        </span>
        <span>{t('columns.image')}</span>
        <span className="hidden md:block">{t('columns.size')}</span>
        <span className="hidden md:block">{t('columns.apps')}</span>
        <span className="hidden md:block">{t('columns.status')}</span>
        <span className="hidden md:block">{t('columns.created')}</span>
        <span className="sr-only">{t('columns.actions')}</span>
      </div>
      {virtualize ? (
        <div
          className="relative"
          style={{ height: virtualizer.getTotalSize() }}
        >
          {virtualizer.getVirtualItems().map((v) => {
            const image = images[v.index]
            if (!image) return null
            return (
              <div
                key={image.id}
                className="absolute top-0 left-0 w-full"
                style={{ transform: `translateY(${v.start}px)` }}
              >
                {row(image)}
              </div>
            )
          })}
        </div>
      ) : (
        images.map((image) => <div key={image.id}>{row(image)}</div>)
      )}
    </div>
  )
}
