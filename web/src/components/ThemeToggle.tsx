import { Monitor, Moon, Sun, type LucideIcon } from 'lucide-react'
import { Button } from './ui/button'
import { useTheme, type Theme } from './ThemeProvider'

// Cycle order for a single click: light -> dark -> system -> light. A
// three-way cycling icon button rather than a popover/select, to match
// the nav's existing compact, icon-forward style (see the sign-out
// button in routes/__root.tsx, which is the immediate neighbor this sits
// next to).
//
// A Record keyed by Theme (not array indexing) so the lookup is fully
// resolved at the type level, tsconfig.app.json's noUncheckedIndexedAccess
// makes `array[i]` come back as `Theme | undefined`, which a Record with
// a literal union key does not.
const NEXT_THEME: Record<Theme, Theme> = {
  light: 'dark',
  dark: 'system',
  system: 'light',
}

const THEME_LABEL: Record<Theme, string> = {
  light: 'Light',
  dark: 'Dark',
  system: 'System',
}

const THEME_ICON: Record<Theme, LucideIcon> = {
  light: Sun,
  dark: Moon,
  system: Monitor,
}

export function ThemeToggle() {
  const { theme, setTheme } = useTheme()
  const Icon = THEME_ICON[theme]

  const cycleTheme = () => {
    setTheme(NEXT_THEME[theme])
  }

  return (
    <Button
      type="button"
      variant="outline"
      size="icon-sm"
      onClick={cycleTheme}
      // Icon-only control: aria-label carries both the current state and
      // what activating it does, since there is no visible text label.
      // title mirrors it for a mouse-hover tooltip. Native <button>
      // semantics (via the shadcn Button primitive) already give this
      // keyboard operability for free, nothing here suppresses that.
      aria-label={`Theme: ${THEME_LABEL[theme]}. Click to switch theme.`}
      title={`Theme: ${THEME_LABEL[theme]}`}
    >
      <Icon />
    </Button>
  )
}
