import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'
import { Skeleton } from '@/components/ui/skeleton'
import { useAssetUsage } from '../../queries/assets'
import { sizeLabel } from '../../lib/assetsFormat'
import type { AssetBucket } from '../../types/assets'
import { PresetLabel, ServingBadge, VisibilityBadge } from './AssetBadges'

function BucketSize({ id }: Readonly<{ id: string }>) {
  const { t } = useTranslation('assets')
  const { data, isPending } = useAssetUsage(id, 0)
  if (isPending) return <Skeleton className="h-4 w-16" />
  if (!data) return <span>{t('list.sizeUnknown')}</span>
  return (
    <span>
      {sizeLabel(data.bytes, data.at_least, (value) =>
        t('usage.atLeast', { value }),
      )}
    </span>
  )
}

export function BucketList({
  buckets,
  onOpen,
}: Readonly<{ buckets: AssetBucket[]; onOpen: (id: string) => void }>) {
  const { t } = useTranslation('assets')
  return (
    <ul className="divide-y divide-border rounded-lg border border-border bg-card">
      {buckets.map((b) => (
        <li key={b.id}>
          <button
            type="button"
            onClick={() => {
              onOpen(b.id)
            }}
            aria-label={t('list.open', { name: b.name })}
            className="flex w-full items-center gap-3 px-3 py-2 text-left text-sm hover:bg-muted/50 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none"
          >
            <span className="min-w-0 flex-1 truncate font-medium">
              {b.name}
            </span>
            <span className="hidden text-muted-foreground sm:inline">
              <PresetLabel preset={b.preset} />
            </span>
            <VisibilityBadge visibility={b.visibility} />
            <ServingBadge mode={b.serving_mode} />
            <span className="w-28 text-right text-muted-foreground tabular-nums">
              <BucketSize id={b.id} />
            </span>
            <ChevronRightGlassIcon
              aria-hidden="true"
              className="size-5 text-muted-foreground"
            />
          </button>
        </li>
      ))}
    </ul>
  )
}
