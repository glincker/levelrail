import { lazy, Suspense, useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  DatabaseGlassIcon,
  FolderGlassIcon,
  GlobeGlassIcon,
  ServerGlassIcon,
  ShareGlassIcon,
  ChevronRightGlassIcon,
} from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { TooltipProvider } from '@/components/ui/tooltip'
import { EngineMark } from '@/components/EngineMark'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { databaseHealth, type HealthState } from '@/lib/databaseHealth'
import { cn } from '@/lib/utils'
import type { WireTone } from '../../pages/tlsDot'
import { MoveToProjectDialog } from '../../MoveToProjectDialog'
import { useDatabaseLinks } from '../../../queries/databaseLinks'
import { useBackupHistory } from '../../../queries/backupHistory'
import type { DatabaseResource } from '../../../types/databaseDetail'
import type { ReconcileCondition } from '../../../types/deploy'
import { Edge, Spine, StatusDot, WireCard } from './WiringRow'

const ConnectDialog = lazy(() =>
  import('../ConnectDialog').then((m) => ({ default: m.ConnectDialog })),
)
const BackupSetupDialog = lazy(() =>
  import('../backupSetup/BackupSetupDialog').then((m) => ({
    default: m.BackupSetupDialog,
  })),
)

const HEALTH_TONE: Record<HealthState, WireTone> = {
  healthy: 'ok',
  settingUp: 'warn',
  stopped: 'off',
  attention: 'bad',
}

const CTA_CLASS = buttonVariants({ variant: 'outline', size: 'xs' })

function whenText(iso: string, lng: string): string {
  const diff = new Date(iso).getTime() - Date.now()
  const rtf = new Intl.RelativeTimeFormat(lng, { numeric: 'auto' })
  const hours = Math.round(diff / 3_600_000)
  if (Math.abs(hours) < 24) return rtf.format(hours, 'hour')
  return rtf.format(Math.round(hours / 24), 'day')
}

/** Left to right: the apps fed by this database, the database, and where it runs and who can reach it. */
export function DatabaseWiring({
  database,
  conditions,
  projectName,
}: Readonly<{
  database: DatabaseResource
  conditions: ReconcileCondition[]
  projectName?: string
}>) {
  const { t, i18n } = useTranslation('dbOverview')
  const { t: td } = useTranslation('databases')
  const name = database.name
  const links = useDatabaseLinks(name).data ?? []
  const history = useBackupHistory(name).data
  const lastBackup = history?.find((b) => b.finished_at)?.finished_at
  const health = databaseHealth(database, conditions)
  const publicOn = database.publicly_accessible === true
  const scheduled = Boolean(database.backup_schedule)
  const canBackup = engineHasBackups(database.engine)

  const [appsOpen, setAppsOpen] = useState(false)
  const [appDialog, setAppDialog] = useState(false)
  const [connectOpen, setConnectOpen] = useState(false)
  const [backupOpen, setBackupOpen] = useState(false)

  const tlsLabel =
    database.tls_enabled === undefined
      ? undefined
      : database.tls_enabled
        ? t('tlsOn')
        : t('tlsOff')
  const accessValue = publicOn
    ? t('accessBind', {
        bind: database.public_bind_address ?? t('accessPublic'),
        port: database.public_port,
      })
    : t('accessPrivate')
  const wireTone: WireTone = health.state === 'attention' ? 'bad' : 'ok'

  return (
    <TooltipProvider delay={250}>
      <section
        aria-labelledby="db-wiring-title"
        className="@container space-y-3 rounded-lg border border-border bg-card p-3"
      >
        <h2 id="db-wiring-title" className="text-sm font-semibold">
          {t('title')}
        </h2>
        <div className="grid gap-3 @xl:grid-cols-[minmax(0,1fr)_minmax(0,1.1fr)_minmax(0,1.2fr)] @xl:items-center @xl:gap-x-10">
          <div className="group/row relative">
            <Edge tone={links.length > 0 ? 'ok' : 'off'} side="right" />
            <WireCard
              control={
                <button
                  type="button"
                  aria-expanded={appsOpen}
                  onClick={() =>
                    links.length > 0
                      ? setAppsOpen((v) => !v)
                      : setAppDialog(true)
                  }
                />
              }
              icon={<ShareGlassIcon aria-hidden="true" />}
              label={t('apps')}
              value={t('appsCount', { count: links.length })}
              tone={links.length > 0 ? 'ok' : 'off'}
              hint={links.length > 0 ? t('apps') : t('appsEmptyHint')}
              next={
                links.length > 0
                  ? t('appsOpen', { name: links[0]?.app })
                  : t('appsNext')
              }
              cta={
                links.length === 0 ? (
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={() => setAppDialog(true)}
                  >
                    {t('appCta')}
                  </Button>
                ) : (
                  <ChevronRightGlassIcon
                    aria-hidden="true"
                    className={cn(
                      'size-3.5 text-muted-foreground transition-transform duration-150 motion-reduce:transition-none',
                      appsOpen && 'rotate-90',
                    )}
                  />
                )
              }
            >
              {appsOpen && links.length > 0 ? (
                <ul className="space-y-1 border-t border-border px-2.5 py-2">
                  {links.map((l) => (
                    <li
                      key={`${l.app}-${l.env_var}`}
                      className="flex min-w-0 items-baseline gap-1.5 text-xs"
                    >
                      <Link
                        to="/apps/$name"
                        params={{ name: l.app }}
                        aria-label={t('appsOpen', { name: l.app })}
                        className="truncate font-medium underline-offset-2 hover:underline"
                      >
                        {l.app}
                      </Link>
                      <span className="truncate font-mono text-muted-foreground">
                        {l.env_var}
                      </span>
                    </li>
                  ))}
                </ul>
              ) : null}
            </WireCard>
          </div>

          <div className="group/row relative min-w-0 rounded-lg border border-border bg-muted/30 p-3">
            <Edge tone={wireTone} side="left" />
            <Edge tone={wireTone} side="right" />
            <div className="flex items-center gap-2">
              <EngineMark engine={database.engine} className="size-6" />
              <div className="min-w-0 flex-1">
                <h3 className="truncate text-sm font-semibold">{name}</h3>
                <p className="truncate text-xs text-muted-foreground">
                  {database.engine} {database.version}
                </p>
              </div>
              <span className="flex items-center gap-1.5 text-xs text-muted-foreground">
                <StatusDot tone={HEALTH_TONE[health.state]} />
                {td(`status.${health.state}`)}
              </span>
            </div>
            <div className="mt-2.5 flex flex-wrap gap-1.5">
              <Button
                type="button"
                variant="outline"
                size="xs"
                onClick={() => setConnectOpen(true)}
              >
                <DatabaseGlassIcon aria-hidden="true" />
                {t('connect')}
              </Button>
              <Link
                to="/databases/$name/explorer"
                params={{ name }}
                className={CTA_CLASS}
              >
                {t('explorer')}
              </Link>
            </div>
          </div>

          <div>
            <h3 className="sr-only">{t('outTitle')}</h3>
            <ul className="flex flex-col gap-2">
              <li className="group/row relative">
                <Spine />
                <Edge tone={database.node_name ? 'ok' : 'off'} side="left" />
                <WireCard
                  control={
                    database.node_id ? (
                      <Link to="/nodes/$id" params={{ id: database.node_id }} />
                    ) : (
                      <span />
                    )
                  }
                  icon={<ServerGlassIcon aria-hidden="true" />}
                  label={t('nodeLabel')}
                  value={database.node_name ?? t('nodeNone')}
                  tone={database.node_name ? 'ok' : 'off'}
                  hint={t('nodeHint')}
                  next={t('nodeOpen')}
                />
              </li>
              <li className="group/row relative">
                <Spine />
                <Edge tone={publicOn ? 'warn' : 'ok'} side="left" />
                <WireCard
                  control={
                    <Link
                      to="/databases/$name/settings"
                      params={{ name }}
                      search={{ section: 'networking' }}
                    />
                  }
                  icon={<GlobeGlassIcon aria-hidden="true" />}
                  label={t('accessLabel')}
                  value={accessValue}
                  sub={tlsLabel}
                  tone={publicOn ? 'warn' : 'ok'}
                  hint={t('accessHint')}
                  next={publicOn ? t('accessLabel') : t('accessCta')}
                  cta={
                    publicOn ? undefined : (
                      <Link
                        to="/databases/$name/settings"
                        params={{ name }}
                        search={{ section: 'networking' }}
                        className={CTA_CLASS}
                      >
                        {t('accessCta')}
                      </Link>
                    )
                  }
                />
              </li>
              <li className="group/row relative">
                <Spine />
                <Edge tone={scheduled ? 'ok' : 'off'} side="left" />
                <WireCard
                  control={
                    <Link
                      to="/databases/$name/settings"
                      params={{ name }}
                      search={{ section: 'backups' }}
                    />
                  }
                  icon={<DatabaseGlassIcon aria-hidden="true" />}
                  label={t('backupsLabel')}
                  value={database.backup_schedule ?? t('backupsNone')}
                  sub={
                    scheduled
                      ? lastBackup
                        ? t('backupsLast', {
                            when: whenText(lastBackup, i18n.language),
                          })
                        : t('backupsNever')
                      : undefined
                  }
                  tone={scheduled ? 'ok' : 'off'}
                  hint={t('backupsHint')}
                  next={scheduled ? t('backupsLabel') : t('backupsCta')}
                  cta={
                    !scheduled && canBackup ? (
                      <Button
                        type="button"
                        variant="outline"
                        size="xs"
                        onClick={() => setBackupOpen(true)}
                      >
                        {t('backupsCta')}
                      </Button>
                    ) : undefined
                  }
                />
              </li>
              <li className="group/row relative">
                <Spine />
                <Edge tone={database.project_id ? 'ok' : 'off'} side="left" />
                <WireCard
                  control={<span />}
                  icon={<FolderGlassIcon aria-hidden="true" />}
                  label={t('projectLabel')}
                  value={
                    database.project_id
                      ? (projectName ?? database.project_id)
                      : t('projectNone')
                  }
                  tone={database.project_id ? 'ok' : 'off'}
                  hint={t('projectHint')}
                  next={t('projectHint')}
                  cta={
                    <MoveToProjectDialog
                      kind="database"
                      name={name}
                      currentProjectId={database.project_id}
                    />
                  }
                />
              </li>
            </ul>
          </div>
        </div>

        <Dialog open={appDialog} onOpenChange={setAppDialog}>
          <DialogContent>
            <DialogHeader>
              <DialogTitle>{t('appDialogTitle')}</DialogTitle>
              <DialogDescription>{t('appDialogBody')}</DialogDescription>
            </DialogHeader>
            <p className="text-sm text-muted-foreground">{t('appDialogEnv')}</p>
            <DialogFooter>
              <Button
                type="button"
                variant="ghost"
                onClick={() => setAppDialog(false)}
              >
                {t('close')}
              </Button>
              <Link to="/apps" className={buttonVariants()}>
                {t('appDialogLink')}
              </Link>
            </DialogFooter>
          </DialogContent>
        </Dialog>
        {connectOpen ? (
          <Suspense fallback={null}>
            <ConnectDialog
              database={database}
              open={connectOpen}
              onOpenChange={setConnectOpen}
            />
          </Suspense>
        ) : null}
        {backupOpen ? (
          <Suspense fallback={null}>
            <BackupSetupDialog
              database={database}
              open={backupOpen}
              onOpenChange={setBackupOpen}
            />
          </Suspense>
        ) : null}
      </section>
    </TooltipProvider>
  )
}
