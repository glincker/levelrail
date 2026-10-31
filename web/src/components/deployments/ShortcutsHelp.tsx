import { useTranslation } from 'react-i18next'
import { KeyboardGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  Popover,
  PopoverContent,
  PopoverTrigger,
} from '@/components/ui/popover'
import { Kbd } from '@/components/kit'
import { DEPLOYMENT_SHORTCUTS } from '../../lib/deploymentsKeyboard'

export function ShortcutsHelp() {
  const { t } = useTranslation('deploys')
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
