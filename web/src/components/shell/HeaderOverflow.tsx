import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CircleQuestionMarkGlassIcon,
  EllipsisGlassIcon,
  MessageCircleGlassIcon,
  MonitorGlassIcon,
  MoonGlassIcon,
  SparklesGlassIcon,
  SunGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLinkItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { changelogQueryOptions } from '../../queries/changelog'
import { toggleDock, useDockState } from '../dock/dockStore'
import { useTheme, type Theme } from '../ThemeProvider'
import {
  requestOpenWhatsNew,
  unreadCount,
  useLastSeenVersion,
} from './whatsNew'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { TeamSwitcher } from '../teams/TeamSwitcher'
import { ScopePicker } from '../environments/ScopePicker'
import { DockToggle } from '../dock/DockToggle'
import { HelpMenu } from '../HelpMenu'
import { ThemeToggle } from '../ThemeToggle'
import { CliStatusBadge } from '../cli/CliStatusBadge'
import { NotificationCenter } from './NotificationCenter'
import { WhatsNewPanel } from './WhatsNewPanel'

const THEMES: { value: Theme; icon: UiIcon }[] = [
  { value: 'light', icon: SunGlassIcon },
  { value: 'dark', icon: MoonGlassIcon },
  { value: 'oled', icon: MoonGlassIcon },
  { value: 'system', icon: MonitorGlassIcon },
]

// Holds every header control that does not fit one row on a phone; the header
// shows this only below its @md container width.
// Rarely used utilities sit in one labeled menu so the bar stays calm.
// The hidden panel and dock toggle stay mounted: they own the What's new
// popover anchor and the Ctrl+J shortcut.
export function HeaderUtilities() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  const changelog = useQuery(changelogQueryOptions())
  const lastSeen = useLastSeenVersion()
  const unread = unreadCount(changelog.data?.entries ?? [], lastSeen)
  const dockOpen = useDockState().view === 'open'
  return (
    <>
      <div
        inert
        aria-hidden="true"
        className="pointer-events-none absolute right-0 bottom-0 size-px overflow-hidden opacity-0 @max-md:hidden"
      >
        <WhatsNewPanel />
        <DockToggle />
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="outline"
              size="icon"
              aria-label={t('shell.headerUtilities')}
              className="@max-md:hidden"
            />
          }
        >
          <EllipsisGlassIcon size="1.4em" aria-hidden="true" />
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="end"
          aria-label={t('shell.headerUtilities')}
          className="w-60 bg-popover shadow-floating"
        >
          <DropdownMenuItem onClick={requestOpenWhatsNew}>
            <SparklesGlassIcon aria-hidden="true" />
            <span className="flex-1">{t('shell.whatsNew.title')}</span>
            {unread > 0 ? (
              <span className="text-xs text-muted-foreground tabular-nums">
                {unread}
              </span>
            ) : null}
          </DropdownMenuItem>
          <DropdownMenuItem onClick={toggleDock}>
            <MessageCircleGlassIcon aria-hidden="true" />
            <span className="flex-1">{t('shell.chat')}</span>
            {dockOpen ? (
              <CheckGlassIcon aria-hidden="true" />
            ) : (
              <span className="text-xs text-muted-foreground">Ctrl J</span>
            )}
          </DropdownMenuItem>
          <DropdownMenuLinkItem render={<Link to="/help" />}>
            <CircleQuestionMarkGlassIcon aria-hidden="true" />
            <span className="flex-1">{t('shell.help.label')}</span>
          </DropdownMenuLinkItem>
          <DropdownMenuSeparator />
          <p className="px-2 py-1 text-xs text-muted-foreground">
            {t('shell.theme.heading')}
          </p>
          {THEMES.map(({ value, icon: ItemIcon }) => (
            <DropdownMenuItem key={value} onClick={() => setTheme(value)}>
              <ItemIcon aria-hidden="true" />
              <span className="flex-1">{t(`shell.theme.${value}`)}</span>
              {value === theme ? <CheckGlassIcon aria-hidden="true" /> : null}
            </DropdownMenuItem>
          ))}
        </DropdownMenuContent>
      </DropdownMenu>
    </>
  )
}

// Phone-width tray: holds every control the single header row cannot fit.
export function HeaderOverflow() {
  const { t } = useTranslation()
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t('shell.headerMore')}
            className="@max-md:size-10 @md:hidden"
          />
        }
      >
        <EllipsisGlassIcon size="1.4em" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-[min(20rem,calc(100vw-1.5rem))]">
        <div className="empty:hidden">
          <ScopePicker />
        </div>
        <div className="flex flex-wrap items-center gap-1">
          <TeamSwitcher />
          <CliStatusBadge />
          <NotificationCenter />
          <WhatsNewPanel />
          <DockToggle />
          <HelpMenu />
          <ThemeToggle />
        </div>
      </PopoverContent>
    </Popover>
  )
}
