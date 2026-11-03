import { useTranslation } from 'react-i18next'
import { EllipsisGlassIcon } from '@thesvg/ui-react'
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

// Holds every header control that does not fit one row on a phone; the header
// shows this only below its @md container width.
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
