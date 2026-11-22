import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  GitBranchGlassIcon,
  KeyGlassIcon,
  LinkGlassIcon,
  LockGlassIcon,
  NetworkGlassIcon,
  ArrowLeftRightGlassIcon,
  WebhookGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { formatAge } from '../../lib/format'
import { useApp } from '../../queries/apps'
import { useCertificates } from '../../queries/certificates'
import { useGitSource } from '../../queries/gitSources'
import { useSiteDeployTokens } from '../../queries/pagesCiTokens'
import { useSiteHook } from '../../queries/pagesHook'
import { useSiteSubmissions } from '../../queries/pagesSubmissions'
import { usePagesSiteDetail, useSiteSettings } from '../../queries/pagesSites'
import { useStaticRoutes } from '../../queries/staticRoutes'
import { deployStateKey, proxiedApps, repoLabel } from './overviewHelpers'
import { siteState, type SiteState } from './pageHelpers'
import { DOT, TLS_DOT, tlsTone, worstTone, type WireTone } from './tlsDot'
import type { PageTab } from './pageTabs'
import {
  CenterWires,
  EmptyRow,
  Group,
  LiveDot,
  SettingChip,
  ToneChip,
  WIRING_COLUMN,
  WIRING_GRID,
  WireItem,
} from './wiringKit'

export { Group }

const STATE_TONE: Record<SiteState, WireTone> = {
  live: 'ok',
  publishing: 'warn',
  notPublished: 'off',
}

/** Group open state is remembered per site so a tidy panel stays tidy. */
function key(name: string, group: string): string {
  return `lr.wiring.pages.${name}.${group}`
}

/** Domains listed inline before the rest collapse into a +N. */
const SHOWN_DOMAINS = 3

function Domains({
  name,
  onOpenTab,
}: Readonly<{ name: string; onOpenTab: (tab: PageTab) => void }>) {
  const { t } = useTranslation('pagesSites')
  const { data: app } = useApp(name)
  const certs = useCertificates().data ?? []
  const domains = app.domains ?? []
  const tones = domains.map((d) =>
    tlsTone(certs.find((c) => c.domain === d)?.status),
  )
  return (
    <Group
      title={t('overview.wiring.domains')}
      count={domains.length}
      tone={worstTone(tones)}
      storageKey={key(name, 'domains')}
    >
      {domains.length === 0 ? (
        <EmptyRow
          text={t('overview.wiring.noDomains')}
          action={t('success.addDomain')}
          onAction={() => onOpenTab('domains')}
        />
      ) : (
        <ul className="space-y-1">
          {domains.slice(0, SHOWN_DOMAINS).map((d, i) => {
            const cert = certs.find((c) => c.domain === d)
            return (
              <li key={d} className="flex min-w-0 items-center gap-1.5 text-xs">
                <span
                  aria-hidden="true"
                  className={`size-1.5 shrink-0 rounded-full ${cert ? (TLS_DOT[cert.status] ?? DOT.warn) : DOT.warn}`}
                />
                <a
                  href={`https://${d}`}
                  target="_blank"
                  rel="noreferrer"
                  title={d}
                  className="min-w-0 flex-1 truncate font-mono underline-offset-2 hover:underline focus-visible:underline"
                >
                  {d}
                </a>
                <ToneChip tone={tones[i] ?? 'warn'}>
                  {t(`overview.wiring.tls.${cert?.status ?? 'unknown'}`)}
                </ToneChip>
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
    <Group
      title={t('overview.wiring.forms')}
      count={forms}
      tone={forms > 0 ? 'ok' : 'off'}
      storageKey={key(name, 'forms')}
    >
      {forms === 0 ? (
        <EmptyRow
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
    <Group
      title={t('overview.wiring.routes')}
      count={rules + headers}
      tone={rules + headers > 0 ? 'ok' : 'off'}
      storageKey={key(name, 'routes')}
    >
      {rules + headers === 0 ? (
        <EmptyRow
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

function Center({
  name,
  onOpenTab,
}: Readonly<{ name: string; onOpenTab: (tab: PageTab) => void }>) {
  const { t } = useTranslation('pagesSites')
  const detail = usePagesSiteDetail(name).data
  const settings = useSiteSettings(name).data
  const state = siteState(detail)
  const tone = state ? STATE_TONE[state] : 'off'
  const last = detail?.latest_deploy
  const when = last?.finished_at ?? last?.started_at
  const open = () => onOpenTab('settings')
  return (
    <div className="relative min-w-0 space-y-2 rounded-lg border border-border bg-muted/30 p-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h3 className="truncate text-sm font-semibold">{name}</h3>
        {state ? (
          <span className="inline-flex items-center gap-1.5 text-sm font-medium">
            <LiveDot tone={tone} pulse={state !== 'notPublished'} />
            <span role="status">{t(`hero.status.${state}`)}</span>
          </span>
        ) : null}
      </div>
      <p className="text-xs text-muted-foreground">
        {last
          ? t('overview.wiring.lastDeploy', {
              state: t(`overview.state.${deployStateKey(last.status)}`),
            })
          : t('overview.wiring.noDeploy')}
        {when ? ` (${formatAge(when)})` : null}
      </p>
      {settings ? (
        <div className="flex flex-wrap gap-1">
          <SettingChip
            icon={<LockGlassIcon aria-hidden="true" />}
            label={t('overview.wiring.password')}
            on={settings.protected}
            onLabel={t('overview.wiring.on')}
            offLabel={t('overview.wiring.off')}
            onClick={open}
          />
          <SettingChip
            icon={<LinkGlassIcon aria-hidden="true" />}
            label={t('overview.wiring.cleanUrls')}
            on={settings.effective.clean_urls ?? settings.clean_urls === true}
            onLabel={t('overview.wiring.on')}
            offLabel={t('overview.wiring.off')}
            onClick={open}
          />
          <SettingChip
            icon={<ArrowLeftRightGlassIcon aria-hidden="true" />}
            label={t('overview.wiring.spa')}
            on={settings.effective.spa ?? settings.spa_fallback === true}
            onLabel={t('overview.wiring.on')}
            offLabel={t('overview.wiring.off')}
            onClick={open}
          />
        </div>
      ) : null}
      <Button size="xs" onClick={open}>
        {t('overview.wiring.openSettings')}
      </Button>
      <CenterWires tone={tone} />
    </div>
  )
}

function LabelButton({
  onClick,
  children,
}: Readonly<{ onClick: () => void; children: ReactNode }>) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="max-w-full truncate rounded font-medium underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-ring"
    >
      {children}
    </button>
  )
}

function Outbound({
  name,
  onOpenTab,
}: Readonly<{ name: string; onOpenTab: (tab: PageTab) => void }>) {
  const { t } = useTranslation('pagesSites')
  const apps = proxiedApps(useStaticRoutes(name).data?.rules ?? [])
  const hook = useSiteHook(name).data?.enabled === true
  const tokens = (useSiteDeployTokens(name).data ?? []).filter(
    (k) => !k.revoked_at,
  ).length
  const git = useGitSource(name).data
  const toSettings = () => onOpenTab('settings')
  return (
    <div className={WIRING_COLUMN}>
      <h3 className="sr-only">{t('overview.wiring.outTitle')}</h3>
      <WireItem
        tone={apps.length > 0 ? 'ok' : 'off'}
        icon={<NetworkGlassIcon />}
        label={
          <LabelButton onClick={() => onOpenTab('routes')}>
            {t('overview.wiring.apps')}
          </LabelButton>
        }
        value={
          apps.length === 0 ? undefined : (
            <span className="flex flex-wrap gap-x-2">
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
          )
        }
        chip={
          apps.length > 0
            ? t('overview.wiring.connected')
            : t('overview.wiring.noApps')
        }
      />
      <WireItem
        tone={git ? 'ok' : 'off'}
        icon={<GitBranchGlassIcon />}
        label={
          <LabelButton onClick={toSettings}>
            {t('overview.wiring.repo')}
          </LabelButton>
        }
        value={
          git ? (
            repoLabel(git.repo_url)
          ) : (
            <button
              type="button"
              onClick={toSettings}
              className="rounded font-medium text-tone-accent underline-offset-2 hover:underline focus-visible:ring-2 focus-visible:ring-ring"
            >
              {t('overview.wiring.connect')}
            </button>
          )
        }
        chip={
          git
            ? t('overview.wiring.connected')
            : t('overview.wiring.notConnected')
        }
      />
      <WireItem
        tone={hook ? 'ok' : 'off'}
        icon={<WebhookGlassIcon />}
        label={
          <LabelButton onClick={toSettings}>
            {t('overview.wiring.hook')}
          </LabelButton>
        }
        chip={hook ? t('overview.wiring.on') : t('overview.wiring.off')}
      />
      <WireItem
        tone={tokens > 0 ? 'ok' : 'off'}
        icon={<KeyGlassIcon />}
        label={
          <LabelButton onClick={toSettings}>
            {t('overview.wiring.tokens')}
          </LabelButton>
        }
        chip={
          tokens > 0
            ? t('overview.wiring.tokensActive', { count: tokens })
            : t('overview.wiring.none')
        }
      />
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
      <div className={WIRING_GRID}>
        <div className={WIRING_COLUMN}>
          <Domains name={name} onOpenTab={onOpenTab} />
          <Forms name={name} onOpenTab={onOpenTab} />
          <Routes name={name} onOpenTab={onOpenTab} />
        </div>
        <Center name={name} onOpenTab={onOpenTab} />
        <Outbound name={name} onOpenTab={onOpenTab} />
      </div>
    </section>
  )
}
