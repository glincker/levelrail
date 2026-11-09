import { useTranslation } from 'react-i18next'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { MetricTile, SkeletonList } from '@/components/kit'
import { useBuildCacheQuota } from '../../queries/buildCenter'
import type { BuildCacheReclaim } from '../../queries/buildCenter'
import { sizeLabel } from './buildFormat'

function reclaimNoteKey(
  r: BuildCacheReclaim,
): 'quota.note.running' | 'quota.note.bounded' | 'quota.note.partial' | null {
  if (r.skipped) return 'quota.note.running'
  if (r.partial) return 'quota.note.partial'
  if (r.bounded) return 'quota.note.bounded'
  return null
}

export default function BuildCacheQuotaCard() {
  const { t } = useTranslation('buildCenter')
  const quota = useBuildCacheQuota()
  const data = quota.data

  return (
    <section
      className="flex flex-col gap-3"
      aria-labelledby="build-cache-quota-title"
    >
      <h2 id="build-cache-quota-title" className="text-base font-semibold">
        {t('quota.title')}
      </h2>
      {quota.isPending ? <SkeletonList rows={1} /> : null}
      {quota.isError ? (
        <Alert variant="destructive">
          <AlertTitle>{t('quota.errorTitle')}</AlertTitle>
          <AlertDescription>{quota.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {data ? (
        <>
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
            <MetricTile
              label={t('quota.appQuota')}
              value={
                data.app_quota_gb > 0
                  ? t('quota.gib', { count: data.app_quota_gb })
                  : t('quota.off')
              }
              tone={data.app_quota_gb > 0 ? 'info' : 'neutral'}
            />
            <MetricTile
              label={t('quota.maxRecords')}
              value={data.reclaim_max_records}
            />
          </div>
          <p className="text-sm text-muted-foreground">
            {t('quota.restartNote')}
          </p>
          {data.reclaims.length === 0 ? (
            <p className="text-sm text-muted-foreground">{t('quota.empty')}</p>
          ) : (
            <div className="overflow-x-auto rounded-lg border border-border">
              <table className="w-full text-sm">
                <thead className="text-left text-muted-foreground">
                  <tr>
                    <th className="px-3 py-2 font-medium">
                      {t('quota.col.app')}
                    </th>
                    <th className="px-3 py-2 font-medium">
                      {t('quota.col.trigger')}
                    </th>
                    <th className="px-3 py-2 font-medium">
                      {t('quota.col.when')}
                    </th>
                    <th className="px-3 py-2 font-medium">
                      {t('quota.col.freed')}
                    </th>
                    <th className="px-3 py-2 font-medium">
                      {t('quota.col.note')}
                    </th>
                  </tr>
                </thead>
                <tbody>
                  {data.reclaims.map((r) => (
                    <tr
                      key={`${r.app}-${r.at}`}
                      className="border-t border-border"
                    >
                      <td className="px-3 py-2 font-medium">{r.app}</td>
                      <td className="px-3 py-2">
                        {t(`quota.trigger.${r.trigger}`)}
                      </td>
                      <td className="px-3 py-2 tabular-nums">
                        {new Date(r.at).toLocaleString()}
                      </td>
                      <td className="px-3 py-2 tabular-nums">
                        {sizeLabel(r.freed_bytes)}
                      </td>
                      <td className="px-3 py-2 text-muted-foreground">
                        {(() => {
                          const key = reclaimNoteKey(r)
                          return key ? t(key) : '-'
                        })()}
                      </td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}
        </>
      ) : null}
    </section>
  )
}
