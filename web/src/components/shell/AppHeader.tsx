import { useTranslation } from 'react-i18next'
import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { Kbd } from '@/components/kit'
import { TeamSwitcher } from '../teams/TeamSwitcher'
import { EnvironmentSwitcher } from '../environments/EnvironmentSwitcher'
import { DockToggle } from '../dock/DockToggle'
import { HelpMenu } from '../HelpMenu'
import { ThemeToggle } from '../ThemeToggle'
import { CliModal } from '../cli/CliModal'
import { CliStatusBadge } from '../cli/CliStatusBadge'
import { InboxBadge } from './InboxBadge'
import { NotificationCenter } from './NotificationCenter'
import { StatusChip } from './StatusChip'
import { WhatsNewPanel } from './WhatsNewPanel'
import { usePageTitle } from './pageTitleStore'
import { useMainScrolled } from './useMainScrolled'

const isMac =
  typeof navigator !== 'undefined' && /mac/i.test(navigator.platform)

// Breakpoints are container queries on the header's own width, not the
// viewport: the open sidebar takes 16rem, so a 768px window leaves ~512px.
// Search is icon-only under @xl, What's new appears at @md, and the
// environment switcher sits on its own row until @4xl.
export function AppHeader({ onSearch }: { onSearch: () => void }) {
  const { t } = useTranslation()
  const scrolled = useMainScrolled()
  const pageTitle = usePageTitle()

  return (
    <>
      <header
        aria-label={t('shell.topBar')}
        data-scrolled={scrolled}
        className="@container group/bar sticky top-0 z-30 shrink-0"
      >
        <div
          aria-hidden="true"
          className="absolute inset-0 origin-top border-b border-border bg-background transition-transform duration-150 group-data-[scrolled=true]/bar:@4xl:scale-y-90 motion-reduce:transition-none"
        />
        <div
          aria-hidden="true"
          className="pointer-events-none absolute inset-0 shadow-header transition-opacity duration-150 not-group-data-[scrolled=true]/bar:opacity-0 motion-reduce:transition-none"
        />
        <div className="relative flex min-h-14 flex-wrap items-center gap-x-1 gap-y-2 px-3 py-2 @xl:gap-x-2 @xl:px-4 @4xl:flex-nowrap @4xl:py-0 @4xl:transition-transform @4xl:duration-150 motion-reduce:transition-none group-data-[scrolled=true]/bar:@4xl:-translate-y-1">
          <SidebarTrigger
            className="-ml-1"
            aria-label={t('shell.toggleSidebar')}
          />
          <Separator
            orientation="vertical"
            className="mr-1 hidden h-4 data-vertical:self-center @xl:mr-2 @xl:block"
          />
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="rounded-full text-muted-foreground max-@xl:w-7 max-@xl:justify-center max-@xl:px-0 @xl:w-44 @xl:justify-start @3xl:w-64"
            aria-label={t('shell.search.label')}
            aria-keyshortcuts="Control+K Meta+K"
            onClick={onSearch}
          >
            <MagnifyingGlassIcon />
            <span className="hidden flex-1 truncate text-left @xl:inline">
              {t('shell.search.placeholder')}
            </span>
            <span className="hidden @3xl:inline-flex">
              <Kbd keys={[isMac ? '⌘' : 'Ctrl', 'K']} />
            </span>
          </Button>
          <div className="order-last w-full empty:hidden @4xl:order-none @4xl:w-auto">
            <EnvironmentSwitcher />
          </div>
          {pageTitle ? (
            <span
              aria-hidden="true"
              className="hidden min-w-0 max-w-64 truncate text-sm font-medium text-foreground opacity-0 transition-opacity duration-150 group-data-[scrolled=true]/bar:opacity-100 motion-reduce:transition-none @4xl:block"
            >
              {pageTitle}
            </span>
          ) : null}
          <div className="ml-auto flex items-center gap-1 @xl:gap-2">
            <TeamSwitcher />
            <CliStatusBadge />
            <StatusChip />
            <InboxBadge />
            <NotificationCenter />
            <div className="hidden @md:block">
              <WhatsNewPanel />
            </div>
            <DockToggle />
            <HelpMenu />
            <ThemeToggle />
          </div>
        </div>
      </header>
      <CliModal />
    </>
  )
}
