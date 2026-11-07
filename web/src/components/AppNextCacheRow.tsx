import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import { Button } from '@/components/ui/button'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { useUpdateApp } from '../queries/apps'
import { useDatabases } from '../queries/databases'
import { useConnectDatabaseApp } from '../queries/databaseLinks'
import { useGitSource } from '../queries/gitSources'
import { useRebuildApp } from '../queries/rebuildApp'
import type { AppDetail } from '../types/appDetail'

const REDIS_ENGINE = 'redis'

function linkedNames(app: AppDetail): string[] {
  const names = Object.values(app.database_env ?? {}).map((e) => e.database)
  if (app.database_attachment) names.push(app.database_attachment.database_name)
  return names
}

// Next.js has no framework field on the app, so the row lives under a disclosure
// that opens by itself once the app has an explicit next_cache value.
export function AppNextCacheRow({ app }: { app: AppDetail }) {
  const { t } = useTranslation('resources', { keyPrefix: 'nextCache' })
  const update = useUpdateApp(app.name, { inlineError: true })
  const databases = useDatabases()
  const rebuild = useRebuildApp(app.name)
  const gitSource = useGitSource(app.name)
  const redisDbs = (databases.data ?? []).filter(
    (d) => d.engine === REDIS_ENGINE,
  )
  const names = linkedNames(app)
  const linked = redisDbs.find((d) => names.includes(d.name))
  const candidate = linked ? undefined : redisDbs[0]
  const connect = useConnectDatabaseApp(candidate?.name ?? '')
  const on = app.next_cache === 'redis'
  const pending = update.isPending || rebuild.isPending

  const fail = () => toast.add({ title: t('errorTitle'), type: 'error' })

  return (
    <details
      className="rounded-md border border-border px-3 py-2"
      open={Boolean(app.next_cache)}
    >
      <summary className="cursor-pointer text-sm font-medium text-foreground">
        {t('title')}
      </summary>
      <div className="mt-3 space-y-3">
        <p className="text-sm text-muted-foreground">{t('hint')}</p>
        <div className="flex items-center gap-3">
          <Switch
            id="next-cache-toggle"
            aria-labelledby="next-cache-toggle-label"
            checked={on}
            disabled={pending}
            onCheckedChange={(checked) =>
              update.mutate(
                { ...app, next_cache: checked ? 'redis' : 'none' },
                {
                  onSuccess: () =>
                    toast.add({ title: t('saved'), type: 'success' }),
                  onError: fail,
                },
              )
            }
          />
          <span
            id="next-cache-toggle-label"
            className="text-sm text-foreground"
          >
            {t('toggle')}
          </span>
        </div>
        {on ? (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            {linked ? (
              <p className="text-tone-success">
                {t('linked', { name: linked.name })}
              </p>
            ) : (
              <>
                <p className="min-w-0 text-tone-warning">{t('notLinked')}</p>
                {candidate ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    writeAction
                    disabled={connect.isPending}
                    onClick={() =>
                      connect.mutate(
                        { app: app.name },
                        {
                          onSuccess: () =>
                            toast.add({
                              title: t('linkedToast', { name: candidate.name }),
                              type: 'success',
                            }),
                          onError: fail,
                        },
                      )
                    }
                  >
                    {t('linkExisting', { name: candidate.name })}
                  </Button>
                ) : (
                  <Button
                    size="sm"
                    variant="outline"
                    render={<Link to="/databases" />}
                  >
                    {t('create')}
                  </Button>
                )}
              </>
            )}
          </div>
        ) : app.replicas > 1 ? (
          <p className="text-sm text-tone-warning">
            {t('suggest', { count: app.replicas })}
          </p>
        ) : null}
        <p className="text-xs text-muted-foreground">{t('autoNote')}</p>
        {gitSource.data ? (
          <Button
            type="button"
            size="sm"
            writeAction
            disabled={pending}
            onClick={() =>
              rebuild.mutate(
                { noCache: false },
                {
                  onSuccess: () =>
                    toast.add({
                      title: t('started', { name: app.name }),
                      type: 'success',
                    }),
                  onError: fail,
                },
              )
            }
          >
            {t('redeploy')}
          </Button>
        ) : gitSource.isPending ? null : (
          <p className="text-xs text-muted-foreground">{t('noGit')}</p>
        )}
      </div>
    </details>
  )
}
