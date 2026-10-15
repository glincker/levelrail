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
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from './ui/dropdown-menu'
import { useTheme, type Theme } from './ThemeProvider'
import { PAGE_WIDTHS, setPageWidth, usePageWidth } from '@/lib/pageWidth'

const THEMES: { value: Theme; icon: Icon }[] = [
  { value: 'light', icon: SunIcon },
  { value: 'dark', icon: MoonIcon },
  { value: 'system', icon: MonitorIcon },
]

export function ThemeToggle() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  const pageWidth = usePageWidth()
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
        <DropdownMenuSeparator />
        {PAGE_WIDTHS.map((w) => (
          <DropdownMenuItem key={w} onClick={() => setPageWidth(w)}>
            <span className="flex-1">{t(`shell.pageWidth.${w}`)}</span>
            {w === pageWidth ? <CheckIcon aria-hidden="true" /> : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
