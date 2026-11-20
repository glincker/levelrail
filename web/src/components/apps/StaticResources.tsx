import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Skeleton } from '@/components/ui/skeleton'
import { RelativeTime } from '@/components/kit'
import { formatSize } from '../../lib/format'
import { usePagesSiteDetail } from '../../queries/pagesSites'
import { SiteTrafficCard } from '../pages/SiteTrafficCard'
import { StatTile } from './StatTile'

const nf = new Intl.NumberFormat()

/** Static sites have no container: disk, files and edge traffic are the whole resource story. */
export function StaticResources({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('appPages')
  const q = usePagesSiteDetail(name)
  const d = q.data
  return (
    <div className="space-y-4">
      <section
        aria-label={t('resources.summary')}
        className="grid grid-cols-2 gap-3 sm:grid-cols-4"
      >
        {q.isPending ? (
          <>
            <Skeleton className="h-[4.5rem]" />
            <Skeleton className="h-[4.5rem]" />
            <Skeleton className="h-[4.5rem]" />
            <Skeleton className="h-[4.5rem]" />
          </>
        ) : null}
        {d ? (
          <>
            <StatTile
              label={t('static.disk')}
              value={formatSize(d.size_bytes)}
            />
            <StatTile
              label={t('static.files')}
              value={nf.format(d.file_count)}
            />
            <StatTile
              label={t('static.published')}
              state={d.published ? 'running' : 'stopped'}
              value={
                d.published ? t('static.published') : t('static.unpublished')
              }
            />
            <StatTile
              label={t('static.updated')}
              value={d.updated_at ? <RelativeTime at={d.updated_at} /> : '-'}
            />
          </>
        ) : null}
      </section>
      {q.isError ? (
        <Alert variant="destructive" role="alert">
          <AlertDescription>{t('static.loadFailed')}</AlertDescription>
        </Alert>
      ) : null}
      <SiteTrafficCard name={name} />
    </div>
  )
}
