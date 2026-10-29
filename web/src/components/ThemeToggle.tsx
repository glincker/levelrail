import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  MonitorGlassIcon,
  MoonGlassIcon,
  SunGlassIcon,
  type UiIcon,
} from '@thesvg/ui-react'
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

const THEMES: { value: Theme; icon: UiIcon }[] = [
  { value: 'light', icon: SunGlassIcon },
  { value: 'dark', icon: MoonGlassIcon },
  { value: 'system', icon: MonitorGlassIcon },
]

export function ThemeToggle() {
  const { t } = useTranslation()
  const { theme, setTheme } = useTheme()
  const pageWidth = usePageWidth()
  const Current =
    THEMES.find((o) => o.value === theme)?.icon ?? MonitorGlassIcon
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
            {value === theme ? (
              <CheckGlassIcon size="1.4em" aria-hidden="true" />
            ) : null}
          </DropdownMenuItem>
        ))}
        <DropdownMenuSeparator />
        {PAGE_WIDTHS.map((w) => (
          <DropdownMenuItem key={w} onClick={() => setPageWidth(w)}>
            <span className="flex-1">{t(`shell.pageWidth.${w}`)}</span>
            {w === pageWidth ? (
              <CheckGlassIcon size="1.4em" aria-hidden="true" />
            ) : null}
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  )
}
