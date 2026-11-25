import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EngineMark } from '@/components/EngineMark'
import { useWriteUnlock } from '../../hooks/useWriteUnlock'
import { summarizeDatabaseStatus } from '../../lib/databaseStatus'
import { useDatabase, useDatabaseStatus } from '../../queries/databases'
import { useExplorerCapabilities } from '../../queries/databaseViewer'

function Field({
  label,
  children,
}: Readonly<{ label: string; children: React.ReactNode }>) {
  return (
    <span className="inline-flex items-center gap-1">
      <span className="text-muted-foreground">{label}</span>
      <span className="font-medium">{children}</span>
    </span>
  )
}

/** ExplorerStatusStrip says how the explorer is connected: mode, login, path and credentials, with honest failure states. */
export function ExplorerStatusStrip({
  databaseName,
  writable = true,
}: Readonly<{ databaseName: string; writable?: boolean }>) {
  const { t } = useTranslation('dbExplorerStrip')
  const { data: database } = useDatabase(databaseName)
  const { data: conditions, refetch: refetchStatus } =
    useDatabaseStatus(databaseName)
  const caps = useExplorerCapabilities(databaseName)
  const write = useWriteUnlock(databaseName)
  const running = summarizeDatabaseStatus(conditions).variant !== 'destructive'
  const failed = !running || caps.isError

  const external = database.external
  const path = external
    ? t('external', { target: `${external.host}:${external.port}` })
    : database.node_is_local === false
      ? t('viaNode', { node: database.node_name ?? database.node_id ?? '' })
      : t('direct')

  const time = write.until
    ? new Date(write.until).toLocaleTimeString([], {
        hour: '2-digit',
        minute: '2-digit',
      })
    : null

  return (
    <div
      role="status"
      aria-label={t('label')}
      className="flex flex-wrap items-center gap-x-4 gap-y-1.5 rounded-md border bg-muted/40 px-3 py-1.5 text-xs"
    >
      <EngineMark engine={database.engine} className="size-4 shrink-0" />
      {failed ? (
        <>
          <span className="flex items-center gap-1.5 text-destructive">
            <TriangleAlertGlassIcon className="size-4" aria-hidden="true" />
            {running ? t('unreachable') : t('notRunning')}
          </span>
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              void refetchStatus()
              void caps.refetch()
            }}
          >
            {t('retry')}
          </Button>
        </>
      ) : (
        <>
          {writable && write.unlocked ? (
            <Badge variant="destructive">
              <TriangleAlertGlassIcon className="size-4" aria-hidden="true" />
              {time ? t('mode.writesUntil', { time }) : t('mode.writesOn')}
            </Badge>
          ) : (
            <Badge variant="muted">
              <LockGlassIcon className="size-4" aria-hidden="true" />
              {t('mode.readOnly')}
            </Badge>
          )}
          <Field label={t('login')}>{t('superuser')}</Field>
          <Field label={t('path')}>{path}</Field>
          <Field label={t('credentials')}>
            {external ? t('stored') : t('managed')}
          </Field>
        </>
      )}
      <span className="ml-auto flex items-center gap-3">
        <Link
          to="/databases/$name/networking"
          params={{ name: databaseName }}
          className="underline underline-offset-2"
        >
          {t('networking')}
        </Link>
        <Link
          to="/databases/$name/access"
          params={{ name: databaseName }}
          className="underline underline-offset-2"
        >
          {t('access')}
        </Link>
      </span>
    </div>
  )
}
