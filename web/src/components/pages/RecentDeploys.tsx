import { lazy, Suspense, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { Button } from '@/components/ui/button'
import { useDeployProgress } from '../../hooks/useDeployProgress'
import { formatAge } from '../../lib/format'

const DeployRowActions = lazy(() => import('./DeployRowActions'))

const DeployLogDialog = lazy(() => import('./DeployLogDialog'))
const DeployFailTail = lazy(() => import('./DeployFailTail'))

const SHOWN = 5
/** Lines of a failed build shown inline under its row. */
const FAIL_TAIL_LINES = 5

const DOT: Record<string, string> = {
  succeeded: 'bg-success',
  running: 'bg-warning motion-safe:animate-pulse',
  failed: 'bg-destructive',
}

/** Latest deploys as a timeline; the URL and promote actions appear on hover or focus and always on touch. */
export function RecentDeploys({
  name,
  onViewAll,
}: Readonly<{ name: string; onViewAll: () => void }>) {
  const { t } = useTranslation('pagesSites')
  const { attempts } = useDeployProgress(name)
  const [logFor, setLogFor] = useState<string | null>(null)
  const rows = attempts.slice(0, SHOWN)
  return (
    <section
      aria-labelledby="pages-recent-title"
      className="space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <div className="flex items-center justify-between gap-2">
        <h2 id="pages-recent-title" className="text-sm font-semibold">
          {t('recent.title')}
        </h2>
        <Button size="xs" variant="ghost" onClick={onViewAll}>
          {t('recent.viewAll')}
        </Button>
      </div>
      {rows.length === 0 ? (
        <p className="text-xs text-muted-foreground">{t('recent.none')}</p>
      ) : (
        <ol className="divide-y divide-border">
          {rows.map((a) => (
            <li key={a.id} className="group py-1.5 text-sm">
              <div className="flex min-h-9 items-center gap-3">
                <span
                  aria-hidden="true"
                  className={`size-2 shrink-0 rounded-full ${DOT[a.status] ?? 'bg-muted-foreground/50'}`}
                />
                <code className="shrink-0 font-mono text-xs">
                  {a.commit_sha?.slice(0, 7) ?? a.id.slice(0, 8)}
                </code>
                <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground">
                  {a.live ? `${t('recent.live')} · ` : ''}
                  {a.author ?? a.source ?? ''}
                </span>
                <span className="shrink-0 text-xs text-muted-foreground tabular-nums">
                  {formatAge(a.finished_at ?? a.started_at)}
                </span>
                <Button
                  size="xs"
                  variant="ghost"
                  onClick={() => setLogFor(a.id)}
                >
                  {t('logs.action')}
                </Button>
                {a.status === 'succeeded' ? (
                  <span className="transition-opacity duration-150 motion-reduce:transition-none [@media(hover:hover)]:opacity-0 [@media(hover:hover)]:group-focus-within:opacity-100 [@media(hover:hover)]:group-hover:opacity-100">
                    <Suspense fallback={null}>
                      <DeployRowActions name={name} deployId={a.id} />
                    </Suspense>
                  </span>
                ) : null}
              </div>
              {a.status === 'failed' ? (
                <Suspense fallback={null}>
                  <DeployFailTail
                    name={name}
                    deployId={a.id}
                    lines={FAIL_TAIL_LINES}
                  />
                </Suspense>
              ) : null}
            </li>
          ))}
        </ol>
      )}
      {logFor ? (
        <Suspense fallback={null}>
          <DeployLogDialog
            name={name}
            deployId={logFor}
            open
            onOpenChange={(o) => {
              if (!o) setLogFor(null)
            }}
          />
        </Suspense>
      ) : null}
    </section>
  )
}
