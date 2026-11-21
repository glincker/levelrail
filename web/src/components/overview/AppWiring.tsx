import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { StatusPill } from '@/components/kit'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import { useCertificates } from '../../queries/certificates'
import { useGitSource } from '../../queries/gitSources'
import type { AppDetail } from '../../types/appDetail'
import type { ReconcileCondition } from '../../types/deploy'
import type { DeployAttempt } from '../../types/deployAttempt'
import { Group } from '../pages/SiteWiring'
import { TLS_DOT } from '../pages/tlsDot'
import { linkedDatabases, registryHost } from './wiringModel'
import { deriveHeroStatus } from './heroStatus'

/** Domains listed inline before the rest collapse into a +N. */
const SHOWN_DOMAINS = 3

type AppTab =
  '/apps/$name/domains' | '/apps/$name/volumes' | '/apps/$name/environment'

function EmptyLink({
  text,
  action,
  to,
  name,
}: Readonly<{ text: string; action: string; to: AppTab; name: string }>) {
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
      <p className="text-xs text-muted-foreground">{text}</p>
      <Link
        to={to}
        params={{ name }}
        className={buttonVariants({ variant: 'outline', size: 'xs' })}
      >
        {action}
      </Link>
    </div>
  )
}

function Domains({ app }: Readonly<{ app: AppDetail }>) {
  const { t } = useTranslation('appOverview')
  const certs = useCertificates().data ?? []
  const domains = app.domains ?? []
  return (
    <Group title={t('wiring.domains')} count={domains.length}>
      {domains.length === 0 ? (
        <EmptyLink
          text={t('wiring.noDomains')}
          action={t('wiring.addDomain')}
          to="/apps/$name/domains"
          name={app.name}
        />
      ) : (
        <ul className="space-y-1">
          {domains.slice(0, SHOWN_DOMAINS).map((d) => {
            const cert = certs.find((c) => c.domain === d)
            return (
              <li key={d} className="flex min-w-0 items-center gap-1.5 text-xs">
                <span
                  aria-hidden="true"
                  className={`size-1.5 shrink-0 rounded-full ${cert ? (TLS_DOT[cert.status] ?? 'bg-muted-foreground/50') : 'bg-muted-foreground/50'}`}
                />
                <span className="min-w-0 flex-1 truncate font-mono" title={d}>
                  {d}
                </span>
                <span className="shrink-0 text-muted-foreground">
                  {t(`wiring.tls.${cert?.status ?? 'unknown'}`, {
                    defaultValue: t('wiring.tls.unknown'),
                  })}
                </span>
              </li>
            )
          })}
        </ul>
      )}
      {domains.length > SHOWN_DOMAINS ? (
        <Link
          to="/apps/$name/domains"
          params={{ name: app.name }}
          className={buttonVariants({ variant: 'ghost', size: 'xs' })}
        >
          {t('wiring.more', { count: domains.length - SHOWN_DOMAINS })}
        </Link>
      ) : null}
    </Group>
  )
}

function Storage({ app }: Readonly<{ app: AppDetail }>) {
  const { t } = useTranslation('appOverview')
  const dbs = linkedDatabases(app)
  const volumes = app.volumes ?? []
  const total = dbs.length + volumes.length
  return (
    <Group title={t('wiring.storage')} count={total}>
      {total === 0 ? (
        <EmptyLink
          text={t('wiring.noStorage')}
          action={t('wiring.addStorage')}
          to="/apps/$name/volumes"
          name={app.name}
        />
      ) : (
        <ul className="space-y-1 text-xs">
          {dbs.map((d) => (
            <li key={`db-${d}`} className="flex min-w-0 gap-1.5">
              <span className="shrink-0 text-muted-foreground">
                {t('wiring.databases')}
              </span>
              <Link
                to="/databases/$name"
                params={{ name: d }}
                className="min-w-0 truncate underline underline-offset-2"
              >
                {d}
              </Link>
            </li>
          ))}
          {volumes.map((v) => (
            <li key={`vol-${v.name}`} className="flex min-w-0 gap-1.5">
              <span className="shrink-0 text-muted-foreground">
                {t('wiring.volumes')}
              </span>
              <span className="min-w-0 truncate font-mono">{v.name}</span>
            </li>
          ))}
        </ul>
      )}
    </Group>
  )
}

function Env({ app }: Readonly<{ app: AppDetail }>) {
  const { t } = useTranslation('appOverview')
  const vars = Object.keys(app.env ?? {}).length
  const secrets = app.secret_env?.length ?? 0
  return (
    <Group title={t('wiring.env')} count={vars + secrets}>
      {vars + secrets === 0 ? (
        <EmptyLink
          text={t('wiring.noEnv')}
          action={t('wiring.addEnv')}
          to="/apps/$name/environment"
          name={app.name}
        />
      ) : (
        <p className="text-xs text-muted-foreground">
          {t('wiring.envSummary', { vars, secrets })}
        </p>
      )}
    </Group>
  )
}

function Center({
  app,
  conditions,
  latest,
  serverOutcome,
}: Readonly<{
  app: AppDetail
  conditions: ReconcileCondition[]
  latest?: DeployAttempt
  serverOutcome?: { outcome?: string; reason?: string }
}>) {
  const { t } = useTranslation('appOverview')
  const git = useGitSource(app.name).data
  const status = deriveHeroStatus({
    conditions,
    suspended: app.suspended,
    envDirty: app.env_dirty,
    deploying: latest?.status === 'running',
    latestAttemptStatus: latest?.status,
    serverOutcome,
  })
  return (
    <div className="min-w-0 space-y-2 rounded-lg border border-border bg-muted/30 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="truncate text-sm font-semibold">{app.name}</h3>
        <StatusPill
          tone={status.tone}
          label={status.label}
          live={status.live}
          title={status.title}
        />
      </div>
      <div className="flex flex-wrap gap-1">
        <Badge variant="muted">
          {git ? t('wiring.buildGit') : t('wiring.buildImage')}
        </Badge>
        <Badge variant="muted">{t('wiring.port', { port: app.port })}</Badge>
        <Badge variant="muted">
          {t('wiring.replicas', { count: app.replicas })}
        </Badge>
        <Badge variant="muted">
          {t('wiring.strategy', { strategy: app.strategy })}
        </Badge>
      </div>
      <Link
        to="/apps/$name/deploy-settings"
        params={{ name: app.name }}
        className={buttonVariants({ variant: 'ghost', size: 'xs' })}
      >
        {t('wiring.openSettings')}
      </Link>
    </div>
  )
}

function OutRow({
  label,
  children,
}: Readonly<{ label: string; children: ReactNode }>) {
  return (
    <div className="flex min-w-0 items-center justify-between gap-2 text-xs">
      <span className="font-medium">{label}</span>
      <span className="min-w-0 truncate text-muted-foreground">{children}</span>
    </div>
  )
}

function Outbound({ app }: Readonly<{ app: AppDetail }>) {
  const { t } = useTranslation('appOverview')
  const dbs = linkedDatabases(app)
  const deps = app.depends_on ?? []
  const registry = registryHost(app.image)
  return (
    <div className="min-w-0 space-y-1.5">
      <h3 className="text-xs font-medium">{t('wiring.outTitle')}</h3>
      <OutRow label={t('wiring.databasesOut')}>
        {dbs.length === 0 ? (
          t('wiring.none')
        ) : (
          <span className="flex flex-wrap justify-end gap-x-2">
            {dbs.map((d) => (
              <Link
                key={d}
                to="/databases/$name"
                params={{ name: d }}
                className="text-foreground underline underline-offset-2"
              >
                {d}
              </Link>
            ))}
          </span>
        )}
      </OutRow>
      {deps.length > 0 ? (
        <OutRow label={t('wiring.dependsOn')}>{deps.join(', ')}</OutRow>
      ) : null}
      <OutRow label={t('wiring.node')}>
        {app.node_name
          ? `${app.node_name}${app.node_offline ? ` (${t('wiring.offline')})` : ''}`
          : t('wiring.noNode')}
      </OutRow>
      {registry ? (
        <OutRow label={t('wiring.registry')}>
          <span className="font-mono">{registry}</span>
        </OutRow>
      ) : null}
    </div>
  )
}

/** How the app is wired: what feeds it on the left, the app itself, and what it reaches on the right. */
export function AppWiring({
  app,
  conditions,
  latest,
  serverOutcome,
}: Readonly<{
  app: AppDetail
  conditions: ReconcileCondition[]
  latest?: DeployAttempt
  serverOutcome?: { outcome?: string; reason?: string }
}>) {
  const { t } = useTranslation('appOverview')
  return (
    <section
      aria-labelledby="app-wiring-title"
      className="@container space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <h2 id="app-wiring-title" className="text-sm font-semibold">
        {t('wiring.title')}
      </h2>
      <div className="grid gap-3 @xl:grid-cols-[minmax(0,1fr)_1.25rem_minmax(0,1.1fr)_1.25rem_minmax(0,1fr)] @xl:items-center">
        <div className="min-w-0 space-y-1">
          <Domains app={app} />
          <Storage app={app} />
          <Env app={app} />
        </div>
        <span aria-hidden="true" className="hidden h-px bg-border @xl:block" />
        <Center
          app={app}
          conditions={conditions}
          latest={latest}
          serverOutcome={serverOutcome}
        />
        <span aria-hidden="true" className="hidden h-px bg-border @xl:block" />
        <Outbound app={app} />
      </div>
    </section>
  )
}
