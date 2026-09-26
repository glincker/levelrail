import * as React from 'react'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  ArrowClockwiseIcon,
  ClockCounterClockwiseIcon,
  DatabaseIcon,
  KeyboardIcon,
  RobotIcon,
  WarningCircleIcon,
  MagnifyingGlassIcon,
  RocketLaunchIcon,
  StackIcon,
  TerminalWindowIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  DialogPortal,
  DialogOverlay,
  DialogTitle,
} from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { fuzzyFilter } from '@/lib/fuzzy'
import { loadRecentKeys, pushRecentKey } from '@/lib/recentItems'
import { usePaletteAppActions } from '../hooks/usePaletteAppActions'
import { usePageActions } from '@/lib/pageActions'
import { appListQueryOptions } from '../queries/apps'
import { databaseListQueryOptions } from '../queries/databases'
import { useTheme, type Theme } from './ThemeProvider'
import {
  GROUP_ORDER,
  ROUTE_ENTRIES,
  THEME_ACTION,
  type PaletteItem,
} from './commandPaletteData'
import { PaletteFooter, ResultRow } from './commandPaletteEntries'
import { chordFor } from './shell/navModel'
import { buildPaletteSuggestions } from './shell/paletteSuggestions'

const MAX_APP_MATCHES = 3
const NO_HINT_KEYS = new Set(['action-create-app', 'action-templates'])
const NEXT_THEME: Record<Theme, Theme> = {
  light: 'dark',
  dark: 'system',
  system: 'light',
}

// Command palette: Cmd+K (Mac) / Ctrl+K (elsewhere) opens it from anywhere
// in the app shell. Self-contained state, but `open` can also be driven
// externally (__root.tsx's header trigger) via the open/onOpenChange props.
export function CommandPalette({
  open: openProp,
  onOpenChange,
  onShowShortcuts,
}: {
  open?: boolean
  onOpenChange?: (open: boolean) => void
  onShowShortcuts?: () => void
} = {}) {
  const [internalOpen, setInternalOpen] = React.useState(false)
  const open = openProp ?? internalOpen
  const setOpen = React.useCallback(
    (next: boolean) => {
      setInternalOpen(next)
      onOpenChange?.(next)
    },
    [onOpenChange],
  )

  const [query, setQuery] = React.useState('')
  const [activeIndex, setActiveIndex] = React.useState(0)
  const [recentKeys, setRecentKeys] = React.useState<string[]>(loadRecentKeys)
  const inputRef = React.useRef<HTMLInputElement>(null)
  const navigate = useNavigate()
  const { theme, setTheme } = useTheme()
  const { restartApp, redeployApp } = usePaletteAppActions()
  const pageActions = usePageActions()
  const currentApp = useRouterState({
    select: (st) => /^\/apps\/([^/]+)/.exec(st.location.pathname)?.[1],
  })

  // Both lists come from the shared query cache: they only refetch when
  // stale on open, never per keystroke (filtering is client-side).
  const appsQuery = useQuery({ ...appListQueryOptions(), enabled: open })
  const databasesQuery = useQuery({
    ...databaseListQueryOptions(),
    enabled: open,
  })

  React.useEffect(() => {
    function handleKeyDown(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        setOpen(!open)
      }
    }
    window.addEventListener('keydown', handleKeyDown)
    return () => window.removeEventListener('keydown', handleKeyDown)
  }, [open, setOpen])

  // Reset filter state at the moment `open` flips, computed during render
  // (not an effect) per React's "adjusting state on prop change" pattern.
  const [prevOpen, setPrevOpen] = React.useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (open) {
      setQuery('')
      setActiveIndex(0)
      setRecentKeys(loadRecentKeys())
    }
  }

  React.useEffect(() => {
    if (open) {
      const id = window.setTimeout(() => inputRef.current?.focus(), 0)
      return () => window.clearTimeout(id)
    }
    return undefined
  }, [open])

  const baseItems = React.useMemo<PaletteItem[]>(() => {
    const go =
      (
        to: string,
        params?: Record<string, string>,
        search?: Record<string, string>,
      ) =>
      () =>
        void navigate(search ? { to, params, search } : { to, params })
    const items: PaletteItem[] = ROUTE_ENTRIES.map((e) => ({
      key: e.key,
      label: e.label,
      group: e.group,
      icon: e.icon,
      run: go(e.to, undefined, e.search),
      hint: NO_HINT_KEYS.has(e.key) ? undefined : chordFor(e.to),
    }))
    for (const a of pageActions) {
      items.push({ ...a, group: 'Actions' })
    }
    items.push({
      ...THEME_ACTION,
      run: () => setTheme(NEXT_THEME[theme]),
    })
    if (onShowShortcuts) {
      items.push({
        key: 'action-shortcuts',
        label: 'Keyboard shortcuts',
        group: 'Actions',
        icon: <KeyboardIcon />,
        run: onShowShortcuts,
      })
    }
    for (const app of appsQuery.data ?? []) {
      items.push({
        key: `app-${app.name}`,
        label: app.name,
        group: 'Apps',
        icon: <StackIcon />,
        run: go('/apps/$name', { name: app.name }),
      })
    }
    for (const db of databasesQuery.data ?? []) {
      items.push({
        key: `db-${db.name}`,
        label: db.name,
        group: 'Databases',
        icon: <DatabaseIcon />,
        run: go('/databases/$name', { name: db.name }),
      })
    }
    return items
  }, [
    navigate,
    setTheme,
    theme,
    appsQuery.data,
    databasesQuery.data,
    onShowShortcuts,
    pageActions,
  ])

  const groups = React.useMemo(() => {
    const q = query.trim()
    const byGroup = new Map<string, PaletteItem[]>()
    const go = (to: string, params?: Record<string, string>) => () =>
      void navigate({ to, params })

    if (!q) {
      const byKey = new Map(baseItems.map((i) => [i.key, i]))
      const recent = recentKeys.flatMap((k) => {
        const item = byKey.get(k)
        return item
          ? [{ ...item, key: `recent-${item.key}`, group: 'Recent' }]
          : []
      })
      const appName = currentApp ? decodeURIComponent(currentApp) : undefined
      const specs = buildPaletteSuggestions({
        currentApp: appName,
        apps: (appsQuery.data ?? []).map((a) => ({
          name: a.name,
          failing: a.status.variant === 'destructive',
        })),
        recentKeys,
      })
      const image = (n: string) =>
        appsQuery.data?.find((a) => a.name === n)?.image ?? ''
      const suggested: PaletteItem[] = specs.map((sp) => {
        const app = sp.app ?? ''
        const base = { key: sp.key, label: sp.label, group: 'Suggested' }
        if (sp.kind === 'app-action' && sp.action === 'restart') {
          return {
            ...base,
            icon: <ArrowClockwiseIcon />,
            run: () => restartApp(app),
          }
        }
        if (sp.kind === 'app-action') {
          return {
            ...base,
            icon: <RocketLaunchIcon />,
            run: () => redeployApp(app, image(app)),
          }
        }
        if (sp.kind === 'assistant') {
          return { ...base, icon: <RobotIcon />, run: go('/ai-assistant') }
        }
        return {
          ...base,
          icon:
            sp.kind === 'failing-app' ? <WarningCircleIcon /> : <StackIcon />,
          run: go('/apps/$name', { name: app }),
        }
      })
      byGroup.set('Suggested', suggested)
      if (recent.length > 0) byGroup.set('Recent', recent)
      for (const item of baseItems) {
        byGroup.set(item.group, [...(byGroup.get(item.group) ?? []), item])
      }
    } else {
      const apps = fuzzyFilter(appsQuery.data ?? [], q, (a) => a.name)
      const matches = q.length >= 2 ? apps : []
      const appActions: PaletteItem[] = []
      for (const app of matches.slice(0, MAX_APP_MATCHES)) {
        const n = app.name
        const act = (
          id: string,
          label: string,
          icon: React.ReactNode,
          run: () => void,
        ): PaletteItem => ({
          key: `app-action-${id}-${n}`,
          label: `${label} ${n}`,
          group: 'App actions',
          icon,
          run,
        })
        appActions.push(
          act('restart', 'Restart', <ArrowClockwiseIcon />, () =>
            restartApp(n),
          ),
          act('redeploy', 'Redeploy', <RocketLaunchIcon />, () =>
            redeployApp(n, app.image),
          ),
          act(
            'logs',
            'Open logs for',
            <TerminalWindowIcon />,
            () =>
              void navigate({ to: '/apps/$name/logs', params: { name: n } }),
          ),
          act(
            'deploys',
            'Open deploys for',
            <ClockCounterClockwiseIcon />,
            () =>
              void navigate({ to: '/apps/$name/deploys', params: { name: n } }),
          ),
        )
      }
      if (appActions.length > 0) byGroup.set('App actions', appActions)
      for (const item of fuzzyFilter(baseItems, q, (i) => i.label)) {
        byGroup.set(item.group, [...(byGroup.get(item.group) ?? []), item])
      }
    }

    return GROUP_ORDER.flatMap((group) => {
      const items = byGroup.get(group)
      return items && items.length > 0 ? [{ group, items }] : []
    })
  }, [
    query,
    baseItems,
    recentKeys,
    appsQuery.data,
    navigate,
    restartApp,
    redeployApp,
    currentApp,
  ])

  const results = React.useMemo(() => groups.flatMap((g) => g.items), [groups])

  const select = React.useCallback(
    (item: PaletteItem) => {
      const originalKey = item.key.replace(/^recent-/, '')
      if (
        !originalKey.startsWith('app-action-') &&
        !originalKey.startsWith('suggest-')
      ) {
        setRecentKeys(pushRecentKey(originalKey))
      }
      item.run()
      setOpen(false)
      setQuery('')
    },
    [setOpen],
  )

  function handleInputKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex((i) =>
        results.length === 0 ? 0 : (i + 1) % results.length,
      )
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex((i) =>
        results.length === 0 ? 0 : (i - 1 + results.length) % results.length,
      )
    } else if (e.key === 'Enter') {
      e.preventDefault()
      const item = results[activeIndex]
      if (item) select(item)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      setOpen(false)
    }
  }

  const optionId = (item: PaletteItem) => `command-palette-option-${item.key}`
  const activeItem = results[activeIndex]
  let flatIndex = -1

  return (
    <DialogPrimitive.Root
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
      }}
    >
      <DialogPortal>
        <DialogOverlay />
        <DialogPrimitive.Popup
          data-slot="command-palette"
          className="fixed top-24 left-1/2 z-50 grid w-full max-w-lg -translate-x-1/2 gap-0 overflow-hidden rounded-xl bg-popover text-sm text-popover-foreground ring-1 ring-foreground/10 duration-100 outline-none data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
        >
          <DialogTitle className="sr-only">Command palette</DialogTitle>
          <div className="flex items-center gap-2 border-b border-border px-3">
            <MagnifyingGlassIcon className="size-4 shrink-0 text-muted-foreground" />
            <Input
              ref={inputRef}
              role="combobox"
              aria-label="Search commands"
              aria-expanded={open}
              aria-controls="command-palette-listbox"
              aria-autocomplete="list"
              aria-activedescendant={
                activeItem ? optionId(activeItem) : undefined
              }
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
                setActiveIndex(0)
              }}
              onKeyDown={handleInputKeyDown}
              placeholder="Search apps, actions, settings..."
              className="h-11 border-none px-0 shadow-none focus-visible:ring-0"
            />
          </div>
          <div
            id="command-palette-listbox"
            role="listbox"
            aria-label="Search results"
            className="max-h-80 overflow-y-auto p-2"
          >
            {results.length === 0 ? (
              <p className="px-2.5 py-4 text-center text-sm text-muted-foreground">
                No results.
              </p>
            ) : (
              groups.map(({ group, items }) => (
                <div
                  key={group}
                  role="group"
                  aria-label={group}
                  className="mb-2 last:mb-0"
                >
                  <p
                    aria-hidden="true"
                    className="px-2.5 py-1 text-xs font-medium text-muted-foreground"
                  >
                    {group}
                  </p>
                  {items.map((item) => {
                    flatIndex += 1
                    const index = flatIndex
                    return (
                      <ResultRow
                        key={item.key}
                        item={item}
                        optionId={optionId(item)}
                        active={index === activeIndex}
                        query={query}
                        onSelect={() => select(item)}
                      />
                    )
                  })}
                </div>
              ))
            )}
          </div>
          <PaletteFooter />
        </DialogPrimitive.Popup>
      </DialogPortal>
    </DialogPrimitive.Root>
  )
}
