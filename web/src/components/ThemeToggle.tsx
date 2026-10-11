import { useTranslation } from 'react-i18next'
import {
  CheckIcon,
  MonitorIcon,
  MoonIcon,
  SunIcon,
} from '@phosphor-icons/react/dist/ssr'
import type { Icon } from '@phosphor-icons/react'
import { Button } from './ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from './ui/dropdown-menu'
import { useTheme, type Theme } from './ThemeProvider'

const THEMES: { value: Theme; icon: Icon }[] = [
  { value: 'light', icon: SunIcon },
  { value: 'dark', icon: MoonIcon },
  { value: 'system', icon: MonitorIcon },
]

export function ThemeToggle() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  const Current = THEMES.find((o) => o.value === theme)?.icon ?? MonitorIcon
  const currentLabel = t(`shell.theme.${theme}`)

  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="outline"
            size="icon-sm"
            aria-label={t('shell.theme.label', { theme: currentLabel })}
            title={t('shell.theme.label', { theme: currentLabel })}
          />
        }
      >
        <Current />
      </DropdownMenuTrigger>
      <DropdownMenuContent aria-label={t('shell.theme.menu')}>
        {THEMES.map(({ value, icon: ItemIcon }) => (
          <DropdownMenuItem key={value} onClick={() => setTheme(value)}>
            <ItemIcon />
            <span className="flex-1">{t(`shell.theme.${value}`)}</span>
            {value === theme ? <CheckIcon aria-hidden="true" /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
