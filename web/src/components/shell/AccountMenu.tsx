import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  BookOpenGlassIcon,
  LogOutGlassIcon,
  MessageCircleGlassIcon,
  MonitorGlassIcon,
  MoonGlassIcon,
  SettingsGlassIcon,
  SquarePenGlassIcon,
  SunGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import { LifebuoyIcon } from '@phosphor-icons/react/dist/ssr'
import { SidebarMenuButton } from '@/components/ui/sidebar'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { useBrand } from '../../hooks/useBrand'
import { preferencesQueryOptions } from '../../queries/accountSelf'
import { sessionQueryOptions } from '../../queries/security'
import { useLogout } from '../../queries/auth'
import { UserAvatar } from '../account/UserAvatar'
import { useTheme, type Theme } from '../ThemeProvider'
import { useAttentionSummary } from './useAttentionSummary'

const THEMES: { value: Theme; icon: UiIcon }[] = [
  { value: 'system', icon: MonitorGlassIcon },
  { value: 'light', icon: SunGlassIcon },
  { value: 'dark', icon: MoonGlassIcon },
]

function ThemeRow() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  return (
    <div className="flex items-center justify-between gap-3 px-2 py-1.5 text-sm">
      <span>{t('shell.accountMenu.theme')}</span>
      <span
        role="radiogroup"
        aria-label={t('shell.accountMenu.theme')}
        className="flex rounded-full border border-border p-0.5"
      >
        {THEMES.map(({ value, icon: ThemeIcon }) => (
          <button
            key={value}
            type="button"
            role="radio"
            aria-checked={theme === value}
            aria-label={t(`shell.theme.${value}`)}
            onClick={() => {
              setTheme(value)
            }}
            className={cn(
              'rounded-full p-1.5 text-muted-foreground transition-colors hover:text-foreground',
              theme === value && 'bg-muted text-foreground',
            )}
          >
            <ThemeIcon className="size-3.5" />
          </button>
        ))}
      </span>
    </div>
  )
}

function StatusRow() {
  const { t } = useTranslation()
  const { level, count } = useAttentionSummary()
  const ok = level === 'ok'
  return (
    <DropdownMenuLinkItem
      render={<Link to="/status" />}
      className="justify-between px-3 py-2.5"
    >
      <span className={ok ? 'text-tone-info' : 'text-tone-danger'}>
        {ok
          ? t('shell.accountMenu.allNormal')
          : t('shell.accountMenu.issues', { count })}
      </span>
      <span
        aria-hidden="true"
        className={cn(
          'size-2 rounded-full',
          ok ? 'bg-tone-info-solid' : 'bg-tone-danger-solid',
        )}
      />
    </DropdownMenuLinkItem>
  )
}

/** AccountMenu is the sidebar footer: avatar and name opening the account, theme, help and sign-out menu. */
export function AccountMenu() {
  const { t } = useTranslation()
  const brand = useBrand()
  const username = useAuthUsername()
  const logout = useLogout()
  const session = useQuery(sessionQueryOptions()).data
  const prefs = useQuery(preferencesQueryOptions()).data
  const name = session?.display_name || username || '?'
  const email = session?.username.includes('@') ? session.username : undefined
  const avatar = (
    <UserAvatar
      name={name}
      seed={username ?? name}
      email={email}
      gravatar={prefs?.gravatar}
      size="sm"
    />
  )
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <SidebarMenuButton
            size="lg"
            tooltip={name}
            className="gap-2 !p-1.5"
          />
        }
      >
        {avatar}
        <span className="flex min-w-0 flex-col text-left group-data-[collapsible=icon]:hidden">
          <span className="truncate text-sm font-medium">{name}</span>
          {email ? (
            <span className="truncate text-[11px] text-sidebar-foreground/60">
              {email}
            </span>
          ) : null}
        </span>
      </DropdownMenuTrigger>
      <DropdownMenuContent side="top" align="start" className="w-72 p-0">
        <div className="flex items-center gap-3 border-b border-border px-3 py-3">
          {avatar}
          <span className="flex min-w-0 flex-1 flex-col">
            <span className="truncate text-sm font-semibold">{name}</span>
            {email ? (
              <span className="truncate text-xs text-muted-foreground">
                {email}
              </span>
            ) : null}
          </span>
          <Link
            to="/settings/account"
            aria-label={t('shell.accountMenu.account')}
            className="rounded-md p-1.5 text-muted-foreground hover:bg-muted hover:text-foreground"
          >
            <SettingsGlassIcon className="size-5" />
          </Link>
        </div>
        <div className="p-1">
          {brand.DiscussionsURL ? (
            <DropdownMenuLinkItem
              render={
                <a
                  href={brand.DiscussionsURL}
                  target="_blank"
                  rel="noreferrer"
                />
              }
            >
              <MessageCircleGlassIcon size="1.4em" />
              {t('shell.accountMenu.feedback')}
            </DropdownMenuLinkItem>
          ) : null}
          <ThemeRow />
          <DropdownMenuLinkItem render={<Link to="/settings/updates" />}>
            <SquarePenGlassIcon size="1.4em" />
            {t('shell.accountMenu.changelog')}
          </DropdownMenuLinkItem>
          <DropdownMenuLinkItem render={<Link to="/help" />}>
            <LifebuoyIcon />
            {t('shell.accountMenu.help')}
          </DropdownMenuLinkItem>
          {brand.DocsURL ? (
            <DropdownMenuLinkItem
              render={
                <a href={brand.DocsURL} target="_blank" rel="noreferrer" />
              }
            >
              <BookOpenGlassIcon size="1.4em" />
              {t('shell.accountMenu.docs')}
            </DropdownMenuLinkItem>
          ) : null}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            disabled={logout.isPending}
            onClick={() => {
              logout.mutate()
            }}
          >
            <LogOutGlassIcon size="1.4em" />
            {t('shell.account.signOut')}
          </DropdownMenuItem>
        </div>
        <div className="border-t border-border p-1">
          <StatusRow />
        </div>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
