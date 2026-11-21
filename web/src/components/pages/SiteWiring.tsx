import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ChevronRightGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { useApp } from '../../queries/apps'
import { useCertificates } from '../../queries/certificates'
import { useGitSource } from '../../queries/gitSources'
import { useSiteDeployTokens } from '../../queries/pagesCiTokens'
import { useSiteHook } from '../../queries/pagesHook'
import { useSiteSubmissions } from '../../queries/pagesSubmissions'
import { usePagesSiteDetail, useSiteSettings } from '../../queries/pagesSites'
import { useStaticRoutes } from '../../queries/staticRoutes'
import { StatusDot } from './SiteHero'
import { deployStateKey, proxiedApps, repoLabel } from './overviewHelpers'
import { siteState } from './pageHelpers'
import type { PageTab } from './pageTabs'

/** Domains listed inline before the rest collapse into a +N. */
const SHOWN_DOMAINS = 3

const TLS_DOT: Record<string, string> = {
  healthy: 'bg-success',
  expiring_soon: 'bg-warning',
  expired: 'bg-destructive',
}

function Group({
  title,
  count,
  children,
}: Readonly<{ title: string; count: number; children: ReactNode }>) {
  return (
    <Collapsible defaultOpen className="min-w-0">
      <CollapsibleTrigger className="group/trg flex items-center gap-1.5 rounded-md py-1 text-xs font-medium focus-visible:ring-2 focus-visible:ring-ring">
        <ChevronRightGlassIcon
          aria-hidden="true"
          className="size-3.5 text-muted-foreground transition-transform duration-150 group-data-[panel-open]/trg:rotate-90 motion-reduce:transition-none"
        />
        {title}
        <span className="ml-auto text-muted-foreground tabular-nums">
          {count}
        </span>
      </CollapsibleTrigger>
      <CollapsiblePanel className="space-y-1 pt-1 pl-5">
        {children}
      </CollapsiblePanel>
    </Collapsible>
  )
}

function Empty({
  text,
  action,
  onAction,
}: Readonly<{ text: string; action: string; onAction: () => void }>) {
  return (
    <div className="flex flex-wrap items-center gap-x-2 gap-y-0.5">
      <p className="text-xs text-muted-foreground">{text}</p>
      <Button size="xs" variant="outline" onClick={onAction}>
        {action}
      </Button>
    </div>
  )
}

function Domains({
  name,
  onOpenTab,
}: Readonly<{ name: string; onOpenTab: (tab: PageTab) => void }>) {
  const { t } = useTranslation('pagesSites')
  const { data: app } = useApp(name)
  const certs = useCertificates().data ?? []
  const domains = app.domains ?? []
  return (
    <Group title={t('overview.wiring.domains')} count={domains.length}>
      {domains.length === 0 ? (
        <Empty
          text={t('overview.wiring.noDomains')}
          action={t('success.addDomain')}
          onAction={() => onOpenTab('domains')}
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
                <span className="min-w-0 flex-1 truncate font-mono">{d}</span>
                <span className="shrink-0 text-muted-foreground">
                  {t(`overview.wiring.tls.${cert?.status ?? 'unknown'}`)}
                </span>
              </li>
            )
          })}
        </ul>
      )}
      {domains.length > SHOWN_DOMAINS ? (
        <Button size="xs" variant="ghost" onClick={() => onOpenTab('domains')}>
          {t('overview.wiring.more', { count: domains.length - SHOWN_DOMAINS })}
        </Button>
      ) : null}
    </Group>
  )
}

function Forms({
  name,
  onOpenTab,
}: Readonly<{ name: string; onOpenTab: (tab: PageTab) => void }>) {
  const { t } = useTranslation('pagesSites')
  const rows = (useSiteSubmissions(name).data ?? []).filter((s) => !s.spam)
  const forms = new Set(rows.map((s) => s.form)).size
  return (
    <Group title={t('overview.wiring.forms')} count={forms}>
      {forms === 0 ? (
        <Empty
          text={t('overview.wiring.noForms')}
          action={t('overview.wiring.addForm')}
          onAction={() => onOpenTab('settings')}
        />
      ) : (
        <p className="text-xs text-muted-foreground">
          {t('overview.wiring.submissions', { count: rows.length })}
        </p>
      )}
    </Group>
  )
}

function Routes({
  name,
  onOpenTab,
}: Readonly<{ name: string; onOpenTab: (tab: PageTab) => void }>) {
  const { t } = useTranslation('pagesSites')
  const routes = useStaticRoutes(name).data
  const rules = routes?.rules.length ?? 0
  const headers = routes?.headers.length ?? 0
  const proxied = proxiedApps(routes?.rules ?? []).length
  return (
    <Group title={t('overview.wiring.routes')} count={rules + headers}>
      {rules + headers === 0 ? (
        <Empty
          text={t('overview.wiring.noRoutes')}
          action={t('overview.wiring.addRule')}
          onAction={() => onOpenTab('routes')}
        />
      ) : (
        <p className="text-xs text-muted-foreground">
          {t('overview.wiring.ruleSummary', { rules, headers, proxied })}
        </p>
      )}
    </Group>
  )
}

function Toggle({ label, on }: Readonly<{ label: string; on: boolean }>) {
  const { t } = useTranslation('pagesSites')
  return (
    <Badge variant={on ? 'success' : 'muted'}>
      {label}
      <span className="sr-only">: </span>
      <span>{on ? t('overview.wiring.on') : t('overview.wiring.off')}</span>
    </Badge>
  )
}

function Center({
  name,
  onOpenTab,
}: Readonly<{ name: string; onOpenTab: (tab: PageTab) => void }>) {
  const { t } = useTranslation('pagesSites')
  const detail = usePagesSiteDetail(name).data
  const settings = useSiteSettings(name).data
  const state = siteState(detail)
  const last = detail?.latest_deploy
  return (
    <div className="min-w-0 space-y-2 rounded-lg border border-border bg-muted/30 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="truncate text-sm font-semibold">{name}</h3>
        {state ? <StatusDot state={state} /> : null}
      </div>
      <p className="text-xs text-muted-foreground">
        {last
          ? t('overview.wiring.lastDeploy', {
              state: t(`overview.state.${deployStateKey(last.status)}`),
            })
          : t('overview.wiring.noDeploy')}
      </p>
      {settings ? (
        <div className="flex flex-wrap gap-1">
          <Toggle
            label={t('overview.wiring.password')}
            on={settings.protected}
          />
          <Toggle
            label={t('overview.wiring.cleanUrls')}
            on={settings.effective.clean_urls ?? settings.clean_urls === true}
          />
          <Toggle
            label={t('overview.wiring.spa')}
            on={settings.effective.spa ?? settings.spa_fallback === true}
          />
        </div>
      ) : null}
      <Button size="xs" variant="ghost" onClick={() => onOpenTab('settings')}>
        {t('overview.wiring.openSettings')}
      </Button>
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

function Outbound({ name }: Readonly<{ name: string }>) {
  const { t } = useTranslation('pagesSites')
  const apps = proxiedApps(useStaticRoutes(name).data?.rules ?? [])
  const hook = useSiteHook(name).data?.enabled === true
  const tokens = (useSiteDeployTokens(name).data ?? []).filter(
    (k) => !k.revoked_at,
  ).length
  const git = useGitSource(name).data
  return (
    <div className="min-w-0 space-y-1.5">
      <h3 className="text-xs font-medium">{t('overview.wiring.outTitle')}</h3>
      <OutRow label={t('overview.wiring.apps')}>
        {apps.length === 0 ? (
          t('overview.wiring.noApps')
        ) : (
          <span className="flex flex-wrap justify-end gap-x-2">
            {apps.map((a) => (
              <Link
                key={a}
                to="/apps/$name"
                params={{ name: a }}
                className="text-foreground underline underline-offset-2"
              >
                {a}
              </Link>
            ))}
          </span>
        )}
      </OutRow>
      <OutRow label={t('overview.wiring.repo')}>
        {git ? repoLabel(git.repo_url) : t('overview.wiring.notConnected')}
      </OutRow>
      <OutRow label={t('overview.wiring.hook')}>
        {hook ? t('overview.wiring.on') : t('overview.wiring.off')}
      </OutRow>
      <OutRow label={t('overview.wiring.tokens')}>
        {tokens > 0
          ? t('overview.wiring.tokensActive', { count: tokens })
          : t('overview.wiring.none')}
      </OutRow>
    </div>
  )
}

/** How the site is wired: what feeds it on the left, the site itself, and what it reaches on the right. */
export function SiteWiring({
  name,
  onOpenTab,
}: Readonly<{ name: string; onOpenTab: (tab: PageTab) => void }>) {
  const { t } = useTranslation('pagesSites')
  return (
    <section
      aria-labelledby="pages-wiring-title"
      className="@container space-y-2 rounded-lg border border-border bg-card p-3"
    >
      <h2 id="pages-wiring-title" className="text-sm font-semibold">
        {t('overview.wiring.title')}
      </h2>
      <div className="grid gap-3 @xl:grid-cols-[minmax(0,1fr)_1.25rem_minmax(0,1.1fr)_1.25rem_minmax(0,1fr)] @xl:items-center">
        <div className="min-w-0 space-y-1">
          <Domains name={name} onOpenTab={onOpenTab} />
          <Forms name={name} onOpenTab={onOpenTab} />
          <Routes name={name} onOpenTab={onOpenTab} />
        </div>
        <span aria-hidden="true" className="hidden h-px bg-border @xl:block" />
        <Center name={name} onOpenTab={onOpenTab} />
        <span aria-hidden="true" className="hidden h-px bg-border @xl:block" />
        <Outbound name={name} />
      </div>
    </section>
  )
}
