import { useLayoutEffect, useRef, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { sizeLabel } from '../images/imageFormat'
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from '../ui/tooltip'
import {
  buildDiskSegments,
  columnWeights,
  gridTemplate,
  innerTemplate,
  type DiskCategory,
  type DiskCategoryInput,
  type DiskSegment,
} from './diskBar'

// Static class names so Tailwind generates them.
const FILL: Record<DiskCategory, string> = {
  appData: 'bg-[var(--chart-series-1)]',
  images: 'bg-[var(--chart-series-4)]',
  buildCache: 'bg-[var(--chart-series-5)]',
  containersLogs: 'bg-[var(--chart-series-6)]',
  other: 'bg-muted-foreground/50',
}

const OUTLINE: Record<DiskCategory, string> = {
  appData: 'border-[var(--chart-series-1)]',
  images: 'border-[var(--chart-series-4)]',
  buildCache: 'border-[var(--chart-series-5)]',
  containersLogs: 'border-[var(--chart-series-6)]',
  other: 'border-muted-foreground/50',
}

const HATCH =
  'bg-[repeating-linear-gradient(135deg,transparent_0_3px,var(--background)_3px_5px)]'

export interface StorageDiskBarProps {
  totalBytes: number
  freeBytes: number
  /** A missing key hides the category; `bytes: null` marks it unmeasured. */
  categories: Partial<Record<DiskCategory, DiskCategoryInput>>
  active?: DiskCategory | null
  onSelect: (category: DiskCategory) => void
}

/** Fractional column widths are not expressible as Tailwind classes, so the template is set on the node. */
function FractionGrid({
  template,
  className,
  children,
}: {
  template: string
  className?: string
  children: ReactNode
}) {
  const ref = useRef<HTMLDivElement>(null)
  useLayoutEffect(() => {
    ref.current?.style.setProperty('grid-template-columns', template)
  }, [template])
  return (
    <div ref={ref} className={cn('grid', className)}>
      {children}
    </div>
  )
}

function percent(share: number): string {
  if (share <= 0) return '0%'
  if (share < 0.01) return '<1%'
  return `${(share * 100).toFixed(share < 0.1 ? 1 : 0)}%`
}

function SegmentCard({ s }: { s: DiskSegment }) {
  const { t } = useTranslation('storage')
  return (
    <div className="flex flex-col gap-1 text-left">
      <p className="text-[13px] font-medium">{t(`disk.categories.${s.id}`)}</p>
      {s.bytes === null ? (
        <p className="text-xs opacity-80">{t('disk.unmeasured')}</p>
      ) : (
        <>
          <p className="text-xs tabular-nums">
            {t('disk.tip.sizeShare', {
              size: sizeLabel(s.bytes),
              share: percent(s.share),
            })}
          </p>
          <p className="text-xs tabular-nums">
            {s.reclaimableBytes > 0
              ? t('disk.tip.reclaimable', {
                  size: sizeLabel(s.reclaimableBytes),
                })
              : t('disk.tip.nothingToFree')}
          </p>
          {s.topItems.length > 0 ? (
            <ul className="flex flex-col gap-0.5 text-xs">
              {s.topItems.map((i) => (
                <li key={i.name} className="flex justify-between gap-3">
                  <span className="truncate">{i.name}</span>
                  <span className="tabular-nums opacity-80">
                    {sizeLabel(i.bytes)}
                  </span>
                </li>
              ))}
            </ul>
          ) : null}
        </>
      )}
      <p className="text-xs opacity-80">{t(`disk.meaning.${s.id}`)}</p>
    </div>
  )
}

/** StorageDiskBar is the whole-disk bar: one pressable segment per category, hatch marks what is safe to remove. */
export function StorageDiskBar({
  totalBytes,
  freeBytes,
  categories,
  active = null,
  onSelect,
}: StorageDiskBarProps) {
  const { t } = useTranslation('storage')
  const segments = buildDiskSegments({ totalBytes, freeBytes, categories })
  const cols = columnWeights(segments, totalBytes, freeBytes)
  const template = gridTemplate(cols.map((c) => c.weight))
  const byId = new Map(segments.map((s) => [s.id, s]))

  return (
    <section aria-label={t('disk.label')} className="flex flex-col gap-3">
      <TooltipProvider delay={0}>
        <FractionGrid template={template} className="h-9 gap-0.5">
          {cols.map((c) => {
            if (c.id === 'free') {
              return (
                <div
                  key="free"
                  aria-hidden="true"
                  className="rounded-sm rounded-r-md bg-muted"
                />
              )
            }
            const s = byId.get(c.id)
            if (!s) return null
            const pressed = active === s.id
            const label = t('disk.segmentLabel', {
              name: t(`disk.categories.${s.id}`),
              size:
                s.bytes === null ? t('disk.unmeasured') : sizeLabel(s.bytes),
            })
            return (
              <Tooltip key={s.id}>
                <TooltipTrigger
                  render={
                    <button
                      type="button"
                      aria-pressed={pressed}
                      aria-label={label}
                      onClick={() => onSelect(s.id)}
                      className={cn(
                        'relative min-w-0 overflow-hidden rounded-sm outline-none first:rounded-l-md focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 focus-visible:ring-offset-background',
                        'transition-[opacity,box-shadow] duration-150 motion-reduce:transition-none',
                        active && !pressed && 'opacity-45',
                        pressed && 'ring-2 ring-foreground/70',
                        s.bytes === null
                          ? cn(
                              'border border-dashed bg-transparent',
                              OUTLINE[s.id],
                            )
                          : FILL[s.id],
                      )}
                    />
                  }
                >
                  {s.bytes !== null && s.reclaimableBytes > 0 ? (
                    <FractionGrid
                      template={innerTemplate(s.bytes, s.reclaimableBytes)}
                      className="absolute inset-0"
                    >
                      <span />
                      <span data-testid={`hatch-${s.id}`} className={HATCH} />
                    </FractionGrid>
                  ) : null}
                </TooltipTrigger>
                <TooltipContent
                  side="top"
                  className="max-w-64 flex-col items-stretch gap-0 py-2"
                >
                  <SegmentCard s={s} />
                </TooltipContent>
              </Tooltip>
            )
          })}
        </FractionGrid>
      </TooltipProvider>
      <ul
        aria-label={t('disk.legend')}
        className="flex flex-wrap gap-x-4 gap-y-1.5"
      >
        {segments.map((s) => (
          <li key={s.id}>
            <button
              type="button"
              aria-pressed={active === s.id}
              onClick={() => onSelect(s.id)}
              className="flex items-center gap-2 rounded-sm px-1 py-0.5 text-[13px] text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring aria-pressed:text-foreground"
            >
              <span
                aria-hidden="true"
                className={cn(
                  'size-2.5 shrink-0 rounded-full',
                  s.bytes === null
                    ? cn('border border-dashed', OUTLINE[s.id])
                    : FILL[s.id],
                )}
              />
              <span>{t(`disk.categories.${s.id}`)}</span>
              <span className="tabular-nums">
                {s.bytes === null ? t('disk.unmeasured') : sizeLabel(s.bytes)}
              </span>
            </button>
          </li>
        ))}
        <li className="flex items-center gap-2 px-1 py-0.5 text-[13px] text-muted-foreground">
          <span aria-hidden="true" className="size-2.5 rounded-full bg-muted" />
          <span>{t('disk.categories.free')}</span>
          <span className="tabular-nums">{sizeLabel(freeBytes)}</span>
        </li>
      </ul>
    </section>
  )
}
