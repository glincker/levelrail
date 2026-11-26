import { useTranslation } from 'react-i18next'
import { Skeleton } from '@/components/ui/skeleton'
import { useRedisInfo, useRedisSlowlog } from '../../queries/databaseViewer'

// Allow listed INFO sections and a slowlog table. Slowlog arguments are
// never sent by the server, only the command name and sizes.
export function RedisInfoPanel({ databaseName }: { databaseName: string }) {
  const { t } = useTranslation('databases')
  const info = useRedisInfo(databaseName, true)
  const slow = useRedisSlowlog(databaseName, true)

  return (
    <div className="space-y-6">
      <section className="space-y-2">
        <h2 className="text-sm font-medium">{t('viewer.keys.infoTitle')}</h2>
        {info.isLoading ? (
          <Skeleton className="h-32 w-full" />
        ) : info.error ? (
          <p className="text-sm text-destructive" role="alert">
            {info.error.message}
          </p>
        ) : (
          <div className="grid gap-3 md:grid-cols-2">
            {(info.data?.sections ?? []).map((s) => (
              <div key={s.name} className="rounded-lg border border-border p-3">
                <h3 className="mb-1 text-xs font-medium uppercase text-muted-foreground">
                  {s.name}
                </h3>
                <dl className="grid grid-cols-[minmax(0,1fr)_minmax(0,1fr)] gap-x-2 gap-y-0.5 font-mono text-xs">
                  {s.fields.map((f) => (
                    <div key={f.key} className="contents">
                      <dt className="truncate text-muted-foreground">
                        {f.key}
                      </dt>
                      <dd className="truncate">{f.value}</dd>
                    </div>
                  ))}
                </dl>
              </div>
            ))}
          </div>
        )}
      </section>

      <section className="space-y-2">
        <h2 className="text-sm font-medium">{t('viewer.keys.slowlogTitle')}</h2>
        <p className="text-xs text-muted-foreground">
          {t('viewer.keys.slowlogNote')}
        </p>
        {slow.isLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : slow.error ? (
          <p className="text-sm text-destructive" role="alert">
            {slow.error.message}
          </p>
        ) : (slow.data?.entries.length ?? 0) === 0 ? (
          <p className="rounded-lg border border-border bg-muted/30 px-4 py-4 text-center text-sm text-muted-foreground">
            {t('viewer.keys.slowlogEmpty')}
          </p>
        ) : (
          <div className="overflow-auto rounded-lg border border-border">
            <table className="w-full text-left font-mono text-xs">
              <thead className="bg-muted/40 text-muted-foreground">
                <tr>
                  <th className="px-2 py-1">{t('viewer.keys.slowId')}</th>
                  <th className="px-2 py-1">{t('viewer.keys.slowTime')}</th>
                  <th className="px-2 py-1">{t('viewer.keys.slowDuration')}</th>
                  <th className="px-2 py-1">{t('viewer.keys.slowCommand')}</th>
                  <th className="px-2 py-1">{t('viewer.keys.slowArgs')}</th>
                  <th className="px-2 py-1">{t('viewer.keys.slowClient')}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-border/60">
                {slow.data?.entries.map((e) => (
                  <tr key={e.id}>
                    <td className="px-2 py-1">{e.id}</td>
                    <td className="px-2 py-1">
                      {new Date(e.time * 1000).toLocaleString()}
                    </td>
                    <td className="px-2 py-1">{e.durationUs}</td>
                    <td className="px-2 py-1">{e.command}</td>
                    <td className="px-2 py-1">{e.argCount}</td>
                    <td className="px-2 py-1">{e.client}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </section>
    </div>
  )
}
