import { useEffect, useRef, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { LockGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EngineMark } from '@/components/EngineMark'
import { toast } from '@/components/ui/toast'
import { ApiError } from '../../lib/apiError'
import { useWriteUnlock } from '../../hooks/useWriteUnlock'
import { summarizeDatabaseStatus } from '../../lib/databaseStatus'
import { useDatabase, useDatabaseStatus } from '../../queries/databases'
import {
  useExplorerCapabilities,
  useStatementAudit,
} from '../../queries/databaseViewer'
import { StatementActivityDrawer } from './StatementActivityDrawer'
import { WriteUnlockDialog } from './WriteUnlockDialog'

function formatLeft(ms: number): string {
  const total = Math.max(0, Math.ceil(ms / 1000))
  const m = Math.floor(total / 60)
  const s = total % 60
  return `${String(m)}:${String(s).padStart(2, '0')}`
}

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
  const [unlockOpen, setUnlockOpen] = useState(false)
  const [activityOpen, setActivityOpen] = useState(false)
  // A one-row probe tells whether the caller may read the audit trail.
  const probe = useStatementAudit(databaseName, 1)
  const canSeeActivity =
    !probe.isError ||
    !(
      probe.error instanceof ApiError &&
      (probe.error.status === 403 || probe.error.status === 404)
    )
  const [now, setNow] = useState(() => Date.now())
  const lastUntil = useRef<number | null>(null)
  const running = summarizeDatabaseStatus(conditions).variant !== 'destructive'
  const failed = !running || caps.isError

  const external = database.external
  const path = external
    ? t('external', { target: `${external.host}:${external.port}` })
    : database.node_is_local === false
      ? t('viaNode', { node: database.node_name ?? database.node_id ?? '' })
      : t('direct')

  useEffect(() => {
    if (!write.until) return
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, 1000)
    return () => {
      window.clearInterval(id)
    }
  }, [write.until])

  // Only a lapsed window toasts; a manual Lock now clears lastUntil first.
  useEffect(() => {
    if (write.until) {
      lastUntil.current = write.until
      return
    }
    if (lastUntil.current !== null && lastUntil.current <= Date.now()) {
      toast.add({ title: t('mode.relocked'), type: 'info' })
    }
    lastUntil.current = null
  }, [write.until, t])

  const left = write.until ? formatLeft(write.until - now) : null

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
            <>
              <Badge variant="destructive">
                <TriangleAlertGlassIcon className="size-4" aria-hidden="true" />
                {left
                  ? t('mode.writesLeft', { time: left })
                  : t('mode.writesOn')}
              </Badge>
              <Button
                variant="outline"
                size="sm"
                onClick={() => {
                  lastUntil.current = null
                  write.lock()
                }}
              >
                {t('lockNow')}
              </Button>
            </>
          ) : (
            <>
              <Badge variant="muted">
                <LockGlassIcon className="size-4" aria-hidden="true" />
                {t('mode.readOnly')}
              </Badge>
              {writable ? (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    setUnlockOpen(true)
                  }}
                >
                  {t('unlock.open')}
                </Button>
              ) : null}
            </>
          )}
          <Field label={t('login')}>{t('superuser')}</Field>
          <Field label={t('path')}>{path}</Field>
          <Field label={t('credentials')}>
            {external ? t('stored') : t('managed')}
          </Field>
        </>
      )}
      <span className="ml-auto flex items-center gap-3">
        {canSeeActivity ? (
          <Button
            variant="ghost"
            size="sm"
            onClick={() => {
              setActivityOpen(true)
            }}
          >
            {t('activity.open')}
          </Button>
        ) : null}
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
      <WriteUnlockDialog
        databaseName={databaseName}
        open={unlockOpen}
        onOpenChange={setUnlockOpen}
        onUnlock={(minutes) => {
          write.unlock(minutes)
        }}
      />
      <StatementActivityDrawer
        databaseName={databaseName}
        open={activityOpen}
        onOpenChange={setActivityOpen}
      />
    </div>
  )
}
