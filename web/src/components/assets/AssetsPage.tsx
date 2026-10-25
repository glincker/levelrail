import { useTranslation } from 'react-i18next'
import { ImagesSquareIcon } from '@phosphor-icons/react/dist/ssr'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { EmptyState } from '@/components/ui/empty-state'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { useAssetBuckets } from '../../queries/assets'
import { BucketDetail } from './BucketDetail'
import { BucketList } from './BucketList'
import { ConnectBucketDialog } from './ConnectBucketDialog'

export function AssetsPage({
  bucketId,
  onSelect,
}: Readonly<{
  bucketId: string | undefined
  onSelect: (id: string | undefined) => void
}>) {
  const { t } = useTranslation('assets')
  const { data: buckets, isPending, isError } = useAssetBuckets()

  return (
    <PageTemplate
      icon={<ImagesSquareIcon />}
      title={t('title')}
      description={t('description')}
      width="wide"
      actions={<ConnectBucketDialog onCreated={onSelect} />}
    >
      {bucketId ? (
        <BucketDetail
          id={bucketId}
          onBack={() => {
            onSelect(undefined)
          }}
        />
      ) : isPending ? (
        <TableSkeleton columnCount={5} />
      ) : isError ? (
        <p className="text-sm text-destructive">{t('loadFailed')}</p>
      ) : buckets.length === 0 ? (
        <EmptyState
          className="gap-2 border-0 bg-transparent py-4"
          icon={<ImagesSquareIcon className="size-5" />}
          title={t('empty.title')}
          description={t('empty.description')}
        />
      ) : (
        <BucketList buckets={buckets} onOpen={onSelect} />
      )}
    </PageTemplate>
  )
}
