import { useTranslation } from 'react-i18next'
import { Link } from '@tanstack/react-router'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { formatElapsed } from '../../lib/buildQueueFormat'
import { useBuildQueue } from '../../queries/buildQueue'

// Lists what holds the build slots and what waits; hidden when both are empty.
export function BuildQueuePanel() {
  const { t } = useTranslation('deploys')
  const { data: queue } = useBuildQueue()
  if (!queue || (queue.running.length === 0 && queue.queued.length === 0)) {
    return null
  }
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('buildQueue.panel.title')}</CardTitle>
        <CardDescription>
          {t('buildQueue.panel.summary', {
            running: queue.running.length,
            queued: queue.queued.length,
            used: queue.slots.used,
            total: queue.slots.total,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <ul className="divide-y divide-border text-sm">
          {queue.running.map((r) => (
            <li
              key={`r-${r.deploy_id ?? r.app}`}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2"
            >
              <Link
                to="/apps/$name/deploys"
                params={{ name: r.app }}
                className="font-medium hover:underline"
              >
                {r.app}
              </Link>
              <span className="text-muted-foreground">
                {t(`buildQueue.phase.${r.phase}`)}
                {r.node ? `, ${r.node}` : ''}
              </span>
              <span className="ml-auto font-mono text-xs text-muted-foreground tabular-nums">
                {formatElapsed(r.elapsed_seconds)}
              </span>
            </li>
          ))}
          {queue.queued.map((q) => (
            <li
              key={`q-${q.deploy_id}`}
              className="flex flex-wrap items-center gap-x-3 gap-y-1 py-2"
            >
              <span className="font-mono text-xs text-muted-foreground tabular-nums">
                {t('buildQueue.panel.position', { position: q.position })}
              </span>
              <Link
                to="/apps/$name/deploys"
                params={{ name: q.app }}
                className="font-medium hover:underline"
              >
                {q.app}
              </Link>
              {q.wait_reason ? (
                <span className="text-muted-foreground">
                  {t('buildQueue.panel.waiting', { reason: q.wait_reason })}
                </span>
              ) : null}
              <span className="ml-auto font-mono text-xs text-muted-foreground tabular-nums">
                {formatElapsed(q.wait_seconds)}
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  )
}
