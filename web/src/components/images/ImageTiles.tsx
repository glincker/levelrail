import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  BroomGlassIcon,
  LayersGlassIcon,
  ServerGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { AnimatedNumber } from '@/components/kit/AnimatedNumber'
import type { ImageInventory } from '../../queries/imageInventory'
import { sizeLabel } from './imageFormat'

function Tile({
  label,
  icon,
  children,
  caption,
  tone = 'text-foreground',
}: {
  label: string
  icon?: ReactNode
  children: ReactNode
  caption?: ReactNode
  tone?: string
}) {
  return (
    <div className="flex h-full flex-col gap-1.5 rounded-lg border border-border bg-card p-4 shadow-raised">
      <div className="flex items-center gap-1.5 text-xs text-muted-foreground [&_svg]:size-5">
        {icon}
        <span className="truncate">{label}</span>
      </div>
      <div className={`text-2xl font-semibold tabular-nums ${tone}`}>
        {children}
      </div>
      {caption}
    </div>
  )
}

interface Props {
  totals: ImageInventory['totals']
  inUseCount: number
  canRemove: boolean
  onReview: () => void
}

export function ImageTiles({ totals, inUseCount, canRemove, onReview }: Props) {
  const { t } = useTranslation('images')
  const freeable = totals.reclaimable_bytes + totals.unmanaged_unused_bytes
  return (
    <div className="grid grid-cols-2 gap-3 lg:grid-cols-4">
      <Tile
        label={t('tiles.count')}
        icon={<LayersGlassIcon size="1.4em" />}
        caption={
          <span className="text-xs text-muted-foreground">
            {t('tiles.countSplit', {
              platform: totals.app_images,
              other: totals.other_images,
            })}
          </span>
        }
      >
        <AnimatedNumber value={totals.count} />
      </Tile>
      <Tile label={t('tiles.size')} icon={<ServerGlassIcon size="1.4em" />}>
        <AnimatedNumber value={totals.size_bytes} format={sizeLabel} />
      </Tile>
      <Tile
        label={t('tiles.reclaimable')}
        tone={freeable > 0 ? 'text-tone-warning' : 'text-foreground'}
        caption={
          <div className="flex flex-col gap-1.5">
            <span className="text-xs text-muted-foreground">
              {t('tiles.freeableSplit', {
                platform: sizeLabel(totals.reclaimable_bytes),
                other: sizeLabel(totals.unmanaged_unused_bytes),
              })}
            </span>
            {canRemove ? (
              <Button
                size="sm"
                disabled={freeable === 0}
                onClick={onReview}
                className="self-start"
              >
                <BroomGlassIcon size="1.4em" aria-hidden="true" />
                {t('tiles.review')}
              </Button>
            ) : null}
          </div>
        }
      >
        <AnimatedNumber value={freeable} format={sizeLabel} />
      </Tile>
      <Tile
        label={t('tiles.protected')}
        tone="text-tone-success"
        caption={
          <span className="text-xs text-muted-foreground">
            {t('tiles.keptSplit', {
              kept: totals.protected_count,
              used: inUseCount,
            })}
          </span>
        }
      >
        <AnimatedNumber value={totals.protected_count} />
      </Tile>
    </div>
  )
}
