import { useTranslation } from 'react-i18next'
import { ServerGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import type { AppImportSession } from '../queries/appImport'
import {
  useStartVolumeCopy,
  useVolumeCopy,
  type VolumeCopyState,
} from '../queries/appImportSSH'
import { formatBytes } from './migrationHubFormat'

const variant: Record<
  VolumeCopyState,
  'success' | 'warning' | 'destructive' | 'muted' | 'default'
> = {
  pending: 'muted',
  running: 'default',
  copied: 'success',
  failed: 'destructive',
  skipped: 'warning',
}

export function AppImportSSHVolumes({
  session,
}: {
  session: AppImportSession
}) {
  const { t } = useTranslation('migration')
  const status = useVolumeCopy(session.id, session.connected)
  const start = useStartVolumeCopy()
  const results = status.data?.results ?? []
  const running = status.data?.running ?? false

  return (
    <section className="space-y-3 rounded-md border p-3">
      <div className="space-y-1">
        <h4 className="text-sm font-medium">
          {t('appImport.ssh.volumes.title')}
        </h4>
        <p className="text-sm text-muted-foreground">
          {t('appImport.ssh.volumes.description', {
            size: formatBytes(status.data?.max_bytes ?? 0),
          })}
        </p>
      </div>
      {session.connected ? null : (
        <Alert>
          <AlertDescription>
            {t('appImport.ssh.volumes.reconnect')}
          </AlertDescription>
        </Alert>
      )}
      <div className="flex flex-wrap items-center gap-2">
        <Button
          type="button"
          disabled={!session.connected || running || start.isPending}
          onClick={() => start.mutate({ id: session.id })}
        >
          <ServerGlassIcon size="1.4em" aria-hidden />
          {results.length > 0
            ? t('appImport.ssh.volumes.again')
            : t('appImport.ssh.volumes.copy')}
        </Button>
        {running ? (
          <span className="text-xs text-muted-foreground" role="status">
            {t('appImport.ssh.volumes.running')}
          </span>
        ) : null}
      </div>
      {start.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{start.error.message}</AlertDescription>
        </Alert>
      ) : null}
      <ul className="space-y-2" aria-live="polite">
        {results.map((r) => (
          <li
            key={`${r.source_id}-${r.container_path}`}
            className="space-y-1 rounded-md border p-2 text-sm"
          >
            <div className="flex flex-wrap items-center gap-2">
              <span className="font-medium">{r.app}</span>
              <span className="font-mono text-xs">{r.container_path}</span>
              <Badge variant={variant[r.state]}>
                {t(`appImport.ssh.volumes.state.${r.state}`)}
              </Badge>
              {r.state === 'copied' ? (
                <span className="text-xs text-muted-foreground tabular-nums">
                  {t('appImport.ssh.volumes.done', {
                    files: r.files,
                    size: formatBytes(r.bytes),
                  })}
                </span>
              ) : null}
            </div>
            {r.error ? (
              <p className="text-xs text-destructive" role="alert">
                {r.error}
              </p>
            ) : null}
            {r.fallback ? (
              <p className="text-xs text-muted-foreground">{r.fallback}</p>
            ) : null}
            {(r.warnings ?? []).map((w) => (
              <p key={w} className="text-xs text-muted-foreground">
                {t('appImport.ssh.volumes.leftOut', { entry: w })}
              </p>
            ))}
          </li>
        ))}
      </ul>
      <p className="text-xs text-muted-foreground">
        {t('appImport.ssh.volumes.fallback')}
      </p>
    </section>
  )
}
