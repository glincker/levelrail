import { useTranslation } from 'react-i18next'
import { KeyboardGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Kbd } from '@/components/kit'
import { DEPLOYMENT_SHORTCUTS } from '../../lib/deploymentsKeyboard'

function ShortcutList() {
  return (
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
  )
}

export interface ShortcutsHelpProps {
  open?: boolean
  onOpenChange?: (open: boolean) => void
}

// With open set it is a menu-driven dialog; otherwise a self-contained popover.
export function ShortcutsHelp({ open, onOpenChange }: ShortcutsHelpProps) {
  const { t } = useTranslation('deploys')
  if (open !== undefined) {
    return (
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t('deployList.shortcuts')}</DialogTitle>
          </DialogHeader>
          <ShortcutList />
        </DialogContent>
      </Dialog>
    )
  }
  return (
    <Popover>
      <PopoverTrigger
        render={
          <Button variant="ghost" size="sm" className="hidden md:inline-flex">
            <KeyboardGlassIcon size="1.4em" aria-hidden="true" />
            {t('deployList.shortcuts')}
          </Button>
        }
      />
      <PopoverContent align="end" className="w-72">
        <ShortcutList />
      </PopoverContent>
    </Popover>
  )
}
