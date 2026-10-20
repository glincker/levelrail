import { KeyboardIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Kbd } from '@/components/kit'
import { DEPLOYMENT_SHORTCUTS } from '../../lib/deploymentsKeyboard'

export function ShortcutsHelp() {
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button variant="ghost" size="sm" className="hidden md:inline-flex">
            <KeyboardIcon aria-hidden="true" />
            Shortcuts
          </Button>
        }
      />
      <PopoverContent align="end" className="w-72">
        <ul className="flex flex-col gap-1.5">
          {DEPLOYMENT_SHORTCUTS.map((s) => (
            <li
              key={s.label}
              className="flex items-center justify-between gap-3 text-sm"
            >
              <span>{s.label}</span>
              <Kbd keys={s.keys} />
            </li>
          ))}
        </ul>
      </PopoverContent>
    </Popover>
  )
}
