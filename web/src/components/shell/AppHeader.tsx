import { MagnifyingGlassIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Separator } from '@/components/ui/separator'
import { SidebarTrigger } from '@/components/ui/sidebar'
import { Kbd } from '@/components/kit'
import { HelpMenu } from '../HelpMenu'
import { ThemeToggle } from '../ThemeToggle'
import { NotificationCenter } from './NotificationCenter'
import { StatusChip } from './StatusChip'

const isMac =
  typeof navigator !== 'undefined' && /mac/i.test(navigator.platform)

export function AppHeader({ onSearch }: { onSearch: () => void }) {
  return (
    <header className="flex h-14 shrink-0 items-center gap-2 border-b border-border px-4">
      <SidebarTrigger className="-ml-1" />
      <Separator orientation="vertical" className="mr-2 h-4" />
      <Button
        type="button"
        variant="outline"
        size="sm"
        className="w-64 justify-start rounded-full text-muted-foreground"
        aria-label="Search or run a command"
        onClick={onSearch}
      >
        <MagnifyingGlassIcon />
        <span className="flex-1 text-left">Search or jump to...</span>
        <Kbd keys={[isMac ? '⌘' : 'Ctrl', 'K']} />
      </Button>
      <div className="ml-auto flex items-center gap-2">
        <StatusChip />
        <NotificationCenter />
        <HelpMenu />
        <ThemeToggle />
      </div>
    </header>
  )
}
