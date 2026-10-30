import { useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useVirtualizer } from '@tanstack/react-virtual'
import {
  BroomGlassIcon,
  LayersGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { MetricTile, RelativeTime, SkeletonList } from '@/components/kit'
import { PageTemplate } from '../shell/PageTemplate'
import { ImagesBanner } from './ImagesBanner'
import { useIsRoot } from '../../hooks/useIsRoot'
import { useNodeListOptional } from '../../queries/nodes'
import {
  useImageInventory,
  type ImageEntry,
} from '../../queries/imageInventory'
import { DockerHealthCard } from './DockerHealthCard'
import { DockerfileLintCard } from './DockerfileLintCard'
import { ImageCleanupDialog } from './ImageCleanupDialog'
import { ImageDetailSheet } from './ImageDetailSheet'
import { SignaturePolicyCard } from './SignaturePolicyCard'
import { imageTitle, shortId, sizeLabel, uniqueApps } from './imageFormat'

const VIRTUALIZE_ABOVE = 50
const ROW_HEIGHT_PX = 56
const GRID =
  'grid grid-cols-[minmax(0,2fr)_6rem_minmax(0,1.2fr)_minmax(0,1.6fr)_7rem] items-center gap-3 px-4'

function ImageRow({
  image,
  onOpen,
}: {
  image: ImageEntry
  onOpen: (image: ImageEntry) => void
}) {
  const { t } = useTranslation('images')
  const apps = uniqueApps(image)
  return (
    <button
      type="button"
      onClick={() => {
        onOpen(image)
      }}
      className={`${GRID} h-14 w-full border-b border-border text-left text-sm hover:bg-muted/50 focus-visible:bg-muted/50 focus-visible:outline-none`}
    >
      <span className="min-w-0">
        <span className="block truncate font-medium">
          {imageTitle(image) ?? t('untagged')}
        </span>
        <span className="block truncate font-mono text-xs text-muted-foreground">
          {shortId(image.id)}
        </span>
      </span>
      <span className="tabular-nums">{sizeLabel(image.size_bytes)}</span>
      <span className="truncate text-muted-foreground">
        {apps.length > 0 ? apps.join(', ') : '-'}
      </span>
      <span
        className={
          image.protected
            ? 'truncate text-muted-foreground'
            : 'truncate font-medium text-tone-warning'
        }
      >
        {image.protected
          ? t('status.kept', { reason: image.protect_reason })
          : t('status.removable')}
      </span>
      <span className="text-muted-foreground">
        <RelativeTime at={image.created_at} />
      </span>
    </button>
  )
}

function ImageTable({
  images,
  onOpen,
}: {
  images: ImageEntry[]
  onOpen: (image: ImageEntry) => void
}) {
  const { t } = useTranslation('images')
  const parentRef = useRef<HTMLDivElement>(null)
  const virtualize = images.length > VIRTUALIZE_ABOVE
  const virtualizer = useVirtualizer({
    count: images.length,
    getScrollElement: () => parentRef.current,
    estimateSize: () => ROW_HEIGHT_PX,
    overscan: 8,
  })

  return (
    <div
      ref={parentRef}
      className={`overflow-auto rounded-lg border border-border bg-card ${virtualize ? 'h-[60vh]' : ''}`}
    >
      <div
        className={`${GRID} sticky top-0 z-10 border-b border-border bg-card py-2 text-xs font-medium tracking-wide text-muted-foreground uppercase`}
      >
        <span>{t('columns.image')}</span>
        <span>{t('columns.size')}</span>
        <span>{t('columns.apps')}</span>
        <span>{t('columns.status')}</span>
        <span>{t('columns.created')}</span>
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
                <ImageRow image={image} onOpen={onOpen} />
              </div>
            )
          })}
        </div>
      ) : (
        images.map((image) => (
          <ImageRow key={image.id} image={image} onOpen={onOpen} />
        ))
      )}
    </div>
  )
}

export function ImagesPage() {
  const { t } = useTranslation('images')
  const [node, setNode] = useState('')
  const [selected, setSelected] = useState<ImageEntry | null>(null)
  const [cleanupOpen, setCleanupOpen] = useState(false)
  const isRoot = useIsRoot()
  const nodes = useNodeListOptional().data
  const inventory = useImageInventory(node)
  const data = inventory.data
  const removable = data?.images.filter((i) => !i.protected) ?? []

  const actions = (
    <div className="flex flex-wrap items-center gap-2">
      {nodes && nodes.length > 0 ? (
        <label className="flex items-center gap-2 text-sm text-muted-foreground">
          {t('node.label')}
          <select
            className="h-8 rounded-md border border-input bg-background px-2 text-sm text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
            value={node}
            onChange={(e) => {
              setNode(e.target.value)
            }}
          >
            <option value="">{t('node.local')}</option>
            {nodes.map((n) => (
              <option key={n.id} value={n.id}>
                {n.name}
              </option>
            ))}
          </select>
        </label>
      ) : null}
      {isRoot && data ? (
        <Button
          disabled={removable.length === 0}
          onClick={() => {
            setCleanupOpen(true)
          }}
        >
          <BroomGlassIcon size="1.4em" aria-hidden="true" />
          {removable.length === 0
            ? t('cleanup.nothing')
            : t('cleanup.button', {
                size: sizeLabel(data.totals.reclaimable_bytes),
              })}
        </Button>
      ) : null}
    </div>
  )

  return (
    <PageTemplate
      title={t('title')}
      description={t('subtitle')}
      width="wide"
      banner={
        <ImagesBanner
          empty={data !== undefined && data.images.length === 0}
          action={actions}
        />
      }
    >
      <div className="flex flex-col gap-4">
        <DockerHealthCard
          canFix={isRoot}
          onFreeSpace={() => {
            setCleanupOpen(true)
          }}
        />
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
            <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
              <MetricTile
                label={t('tiles.count')}
                value={data.totals.count}
                icon={<LayersGlassIcon size="1.4em" />}
              />
              <MetricTile
                label={t('tiles.size')}
                value={sizeLabel(data.totals.size_bytes)}
                icon={<ServerGlassIcon size="1.4em" />}
              />
              <MetricTile
                label={t('tiles.reclaimable')}
                value={sizeLabel(data.totals.reclaimable_bytes)}
                tone={data.totals.reclaimable_bytes > 0 ? 'warning' : 'neutral'}
              />
              <MetricTile
                label={t('tiles.protected')}
                value={data.totals.protected_count}
                tone="success"
              />
            </div>
            <SignaturePolicyCard mode={data.signatures.mode} />
            {data.images.length === 0 ? null : (
              <ImageTable images={data.images} onOpen={setSelected} />
            )}
            <ImageCleanupDialog
              node={node}
              open={cleanupOpen}
              onOpenChange={setCleanupOpen}
              candidates={removable}
              keepPerApp={data.retention.keep_per_app}
              reclaimBytes={data.totals.reclaimable_bytes}
            />
          </>
        ) : null}
      </div>
      <div className="mt-4">
        <DockerfileLintCard />
      </div>
      <ImageDetailSheet
        node={node}
        image={selected}
        onClose={() => {
          setSelected(null)
        }}
      />
    </PageTemplate>
  )
}
