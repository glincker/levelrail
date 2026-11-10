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
  return (
    <OverflowMenu label="shell.headerMore" triggerClass="@md:hidden" full />
  )
}

// Rarely used utilities (what's new, chat, help, theme) live behind one
// button on wider headers so the bar keeps a single calm cluster of alerts.
export function HeaderUtilities() {
  return (
    <OverflowMenu
      label="shell.headerUtilities"
      triggerClass="@max-md:hidden"
      full={false}
    />
  )
}

function OverflowMenu({
  label,
  triggerClass,
  full,
}: {
  label: string
  triggerClass: string
  full: boolean
}) {
  const { t } = useTranslation()
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size="icon"
            aria-label={t(label)}
            className={`@max-md:size-10 ${triggerClass}`}
          />
        }
      >
        <EllipsisGlassIcon size="1.4em" aria-hidden="true" />
      </PopoverTrigger>
      <PopoverContent align="end" className="w-[min(20rem,calc(100vw-1.5rem))]">
        {full ? (
          <div className="empty:hidden">
            <ScopePicker />
          </div>
        ) : null}
        <div className="flex flex-wrap items-center gap-1">
          {full ? (
            <>
              <TeamSwitcher />
              <CliStatusBadge />
              <NotificationCenter />
            </>
          ) : null}
          <WhatsNewPanel />
          <DockToggle />
          <HelpMenu />
          <ThemeToggle />
        </div>
      </PopoverContent>
    </Popover>
  )
}
