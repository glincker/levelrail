import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useState,
  type ReactNode,
} from 'react'

export type Theme = 'light' | 'dark' | 'system'

// Same key the inline script in index.html reads synchronously before
// React mounts, see that file's head script for why this needs to exist
// outside React entirely: it runs before any bundle, module, or context
// is available.
const STORAGE_KEY = 'levelrail-theme'

interface ThemeContextValue {
  theme: Theme
  setTheme: (theme: Theme) => void
}

const ThemeContext = createContext<ThemeContextValue | null>(null)

function getStoredTheme(): Theme {
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    return stored === 'light' || stored === 'dark' || stored === 'system'
      ? stored
      : 'system'
  } catch {
    // localStorage can throw in private-browsing/storage-restricted
    // contexts (same defensive pattern as lib/authStore.ts). Falling
    // back to 'system' is always a safe default.
    return 'system'
  }
}

function systemPrefersDark(): boolean {
  return window.matchMedia('(prefers-color-scheme: dark)').matches
}

function resolveIsDark(theme: Theme): boolean {
  return theme === 'dark' || (theme === 'system' && systemPrefersDark())
}

function applyThemeClass(theme: Theme): void {
  document.documentElement.classList.toggle('dark', resolveIsDark(theme))
}

// Context provider for the app-wide light/dark/system theme. The
// flash-of-wrong-theme fix lives in index.html: a small synchronous
// script reads localStorage and matchMedia before the app bundle loads
// and applies the .dark class to <html> immediately. This provider reads
// the already-applied preference back out of localStorage as its initial
// state (getStoredTheme) rather than fighting what the inline script
// already did, then keeps the class in sync as the user changes their
// selection or, when 'system' is selected, as the OS preference changes.
export function ThemeProvider({ children }: { children: ReactNode }) {
  const [theme, setThemeState] = useState<Theme>(getStoredTheme)

  const setTheme = useCallback((next: Theme) => {
    setThemeState(next)
    try {
      window.localStorage.setItem(STORAGE_KEY, next)
    } catch {
      // Same storage-restricted-context guard as getStoredTheme above:
      // the in-memory theme still applies for this session even if it
      // cannot persist.
    }
  }, [])

  // Applies the resolved theme to <html> whenever the explicit selection
  // changes. Cheap to run redundantly on mount since the inline script
  // already set the right class, this just keeps React and the DOM
  // provably in sync going forward.
  useEffect(() => {
    applyThemeClass(theme)
  }, [theme])

  // Only relevant while 'system' is selected: listens for the OS
  // preference flipping (light <-> dark) while the tab is open and
  // re-resolves without requiring a reload.
  useEffect(() => {
    if (theme !== 'system') {
      return
    }
    const media = window.matchMedia('(prefers-color-scheme: dark)')
    const handleChange = () => {
      applyThemeClass('system')
    }
    media.addEventListener('change', handleChange)
    return () => {
      media.removeEventListener('change', handleChange)
    }
  }, [theme])

  const value = useMemo<ThemeContextValue>(
    () => ({ theme, setTheme }),
    [theme, setTheme],
  )

  return <ThemeContext.Provider value={value}>{children}</ThemeContext.Provider>
}

// Kept in this file rather than split into lib/themeContext.ts +
// hooks/useTheme.ts (the pattern components/BrandProvider.tsx uses) since
// this task's file boundaries only authorize creating ThemeProvider.tsx
// and ThemeToggle.tsx. The targeted disable below is the same shape as
// the existing eslint-disable-next-line comments in MetricsDashboard.tsx
// and LogSearchPanel.tsx: react-refresh/only-export-components flags a
// non-component export sharing a file with a component, but there is no
// Fast Refresh hazard in practice here (useTheme has no state of its
// own, it only reads the context defined two lines above it).
// eslint-disable-next-line react-refresh/only-export-components
export function useTheme(): ThemeContextValue {
  const ctx = useContext(ThemeContext)
  if (!ctx) {
    throw new Error('useTheme must be used within a ThemeProvider')
  }
  return ctx
}
