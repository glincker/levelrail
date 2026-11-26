import { Link, useRouterState } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArchiveRestoreGlassIcon,
  ArrowLeftGlassIcon,
  GitBranchGlassIcon,
  GaugeGlassIcon,
  KeyGlassIcon,
  NetworkGlassIcon,
  LayoutGridGlassIcon,
  PuzzleGlassIcon,
  SettingsGlassIcon,
  SpreadsheetGlassIcon,
  TerminalGlassIcon,
  UsersGlassIcon,
} from '@thesvg/ui-react'
import {
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from '@/components/ui/sidebar'
import { Badge } from '@/components/ui/badge'
import { EngineMark } from '@/components/EngineMark'
import { engineHasBackups } from '@/lib/databaseCatalog'
import { BetaBadge } from './BetaBadge'
import { useDatabase, useDatabaseStatus } from '../queries/databases'
import { useExplorerCapabilities } from '../queries/databaseViewer'
import { summarizeDatabaseStatus } from '../lib/databaseStatus'

const MONITORING_PATHS = ['/metrics', '/logs', '/slow-queries']
const SETTINGS_PATHS = ['/settings']

// Per-database nav grouped like a console: the database home first, then the
// data tools whose labels follow the engine's explorer capabilities.
// Reads from the query cache the layout route primed, no fetch of its own
// (the capabilities query is cached for minutes and shared with the explorer).

export function DatabaseScopedSidebar({ name }: { name: string }) {
  const { t } = useTranslation('databases')
  const { t: tn } = useTranslation('dbNav')
  const { data: database } = useDatabase(name)
  const { data: conditions } = useDatabaseStatus(name)
  const caps = useExplorerCapabilities(name).data
  const status = summarizeDatabaseStatus(conditions)
  const pathname = useRouterState({ select: (s) => s.location.pathname })
  const is = (suffix: string) => pathname.endsWith(suffix)
  const inGroup = (paths: string[]) => paths.some((p) => is(p))

  const explorerLabel =
    caps?.kind === 'sql'
      ? t('home.nav.tables')
      : caps?.kind === 'document'
        ? t('home.nav.collections')
        : caps?.kind === 'keyvalue'
          ? t('home.nav.keys')
          : t('home.nav.explorer')
  const showQuery = caps?.capabilities.query.status !== 'unsupported'
  const showBackups = !database.external && engineHasBackups(database.engine)

  return (
    <>
      <SidebarGroup>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/databases" />}
                tooltip={t('home.nav.back')}
              >
                <ArrowLeftGlassIcon size="1.4em" />
                <span>{t('home.nav.back')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>

      <SidebarGroup>
        <SidebarGroupLabel className="flex h-auto items-center gap-2 py-1.5">
          <EngineMark engine={database.engine} className="size-4 shrink-0" />
          <span className="truncate font-semibold text-sidebar-foreground">
            {database.name}
          </span>
          <Badge variant={status.variant} className="shrink-0">
            {status.label}
          </Badge>
        </SidebarGroupLabel>
        <SidebarGroupContent>
          <SidebarMenu>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link to="/databases/$name/overview" params={{ name }} />
                }
                isActive={is('/overview')}
                tooltip={t('home.nav.overview')}
              >
                <LayoutGridGlassIcon size="1.4em" />
                <span>{t('home.nav.overview')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link to="/databases/$name/explorer" params={{ name }} />
                }
                isActive={is('/explorer')}
                tooltip={explorerLabel}
              >
                <SpreadsheetGlassIcon size="1.4em" />
                <span>{explorerLabel}</span>
                <BetaBadge className="ml-auto" />
              </SidebarMenuButton>
            </SidebarMenuItem>
            {showQuery ? (
              <SidebarMenuItem>
                <SidebarMenuButton
                  render={
                    <Link to="/databases/$name/console" params={{ name }} />
                  }
                  isActive={is('/console')}
                  tooltip={t('home.nav.query')}
                >
                  <TerminalGlassIcon size="1.4em" />
                  <span>{t('home.nav.query')}</span>
                  <BetaBadge className="ml-auto" />
                </SidebarMenuButton>
              </SidebarMenuItem>
            ) : null}
            {showBackups ? (
              <SidebarMenuItem>
                <SidebarMenuButton
                  render={
                    <Link
                      to="/databases/$name/overview"
                      params={{ name }}
                      hash="backups"
                    />
                  }
                  tooltip={t('home.nav.backups')}
                >
                  <ArchiveRestoreGlassIcon size="1.4em" />
                  <span>{t('home.nav.backups')}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ) : null}
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link to="/databases/$name/branches" params={{ name }} />
                }
                isActive={is('/branches')}
                tooltip={t('home.nav.branches')}
              >
                <GitBranchGlassIcon size="1.4em" />
                <span>{t('home.nav.branches')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link to="/databases/$name/networking" params={{ name }} />
                }
                isActive={is('/networking')}
                tooltip={tn('nav.networking')}
              >
                <NetworkGlassIcon size="1.4em" />
                <span>{tn('nav.networking')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={<Link to="/databases/$name/access" params={{ name }} />}
                isActive={is('/access')}
                tooltip={tn('nav.access')}
              >
                <UsersGlassIcon size="1.4em" />
                <span>{tn('nav.access')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link to="/databases/$name/credentials" params={{ name }} />
                }
                isActive={is('/credentials')}
                tooltip={tn('nav.credentials')}
              >
                <KeyGlassIcon size="1.4em" />
                <span>{tn('nav.credentials')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            {!database.external ? (
              <SidebarMenuItem>
                <SidebarMenuButton
                  render={
                    <Link to="/databases/$name/metrics" params={{ name }} />
                  }
                  isActive={inGroup(MONITORING_PATHS)}
                  tooltip={t('home.nav.monitoring')}
                >
                  <GaugeGlassIcon size="1.4em" />
                  <span>{t('home.nav.monitoring')}</span>
                </SidebarMenuButton>
              </SidebarMenuItem>
            ) : null}
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link to="/databases/$name/integrations" params={{ name }} />
                }
                isActive={is('/integrations')}
                tooltip={t('home.nav.integrations')}
              >
                <PuzzleGlassIcon size="1.4em" />
                <span>{t('home.nav.integrations')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
            <SidebarMenuItem>
              <SidebarMenuButton
                render={
                  <Link
                    to="/databases/$name/settings"
                    params={{ name }}
                    search={{ section: 'general' }}
                  />
                }
                isActive={inGroup(SETTINGS_PATHS)}
                tooltip={t('home.nav.settings')}
              >
                <SettingsGlassIcon size="1.4em" />
                <span>{t('home.nav.settings')}</span>
              </SidebarMenuButton>
            </SidebarMenuItem>
          </SidebarMenu>
        </SidebarGroupContent>
      </SidebarGroup>
    </>
  )
}
