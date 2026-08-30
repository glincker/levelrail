import * as React from 'react'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import {
  GaugeIcon,
  StackIcon,
  DatabaseIcon,
  FolderIcon,
  BuildingsIcon,
  HardDrivesIcon,
  GlobeIcon,
  UserIcon,
  ShieldIcon,
  KeyIcon,
  CloudArrowUpIcon,
  WebhooksLogoIcon,
  GithubLogoIcon,
  UsersIcon,
  EnvelopeIcon,
  GearIcon,
  MagnifyingGlassIcon,
  PackageIcon,
  ArrowCircleUpIcon,
  ClockCounterClockwiseIcon,
} from '@phosphor-icons/react/dist/ssr'
import { DialogPortal, DialogOverlay, DialogTitle } from '@/components/ui/dialog'
import { Input } from '@/components/ui/input'
import { cn } from '@/lib/utils'
import { appListQueryOptions } from '../queries/apps'
import { databaseListQueryOptions } from '../queries/databases'

interface ResultItem {
  key: string
  label: string
  group: string
  icon: React.ReactNode
  to: string
  params?: Record<string, string>
}

const STATIC_ENTRIES: ResultItem[] = [
  { key: 'nav-dashboard', label: 'Dashboard', group: 'Navigate', icon: <GaugeIcon />, to: '/' },
  { key: 'nav-apps', label: 'Apps', group: 'Navigate', icon: <StackIcon />, to: '/apps' },
  { key: 'nav-databases', label: 'Databases', group: 'Navigate', icon: <DatabaseIcon />, to: '/databases' },
  { key: 'nav-projects', label: 'Projects', group: 'Navigate', icon: <FolderIcon />, to: '/projects' },
  { key: 'nav-nodes', label: 'Nodes', group: 'Navigate', icon: <HardDrivesIcon />, to: '/nodes' },
  { key: 'nav-domains', label: 'Domains', group: 'Navigate', icon: <GlobeIcon />, to: '/domains' },
  { key: 'settings-hub', label: 'Settings', group: 'Settings', icon: <GearIcon />, to: '/settings' },
  { key: 'settings-account', label: 'Account', group: 'Settings', icon: <UserIcon />, to: '/settings/account' },
  { key: 'settings-security', label: 'Security', group: 'Settings', icon: <ShieldIcon />, to: '/settings/security' },
  { key: 'settings-tokens', label: 'API tokens', group: 'Settings', icon: <KeyIcon />, to: '/settings/tokens' },
  { key: 'settings-backup-targets', label: 'Backup targets', group: 'Settings', icon: <CloudArrowUpIcon />, to: '/settings/backup-targets' },
  { key: 'settings-registry-credentials', label: 'Registry credentials', group: 'Settings', icon: <PackageIcon />, to: '/settings/registry-credentials' },
  { key: 'settings-notification-channels', label: 'Notification channels', group: 'Settings', icon: <WebhooksLogoIcon />, to: '/settings/notification-channels' },
  { key: 'settings-github-app', label: 'GitHub App', group: 'Settings', icon: <GithubLogoIcon />, to: '/settings/github-app' },
  { key: 'settings-oauth', label: 'OAuth sign-in', group: 'Settings', icon: <KeyIcon />, to: '/settings/oauth' },
  { key: 'settings-organizations', label: 'Organizations', group: 'Settings', icon: <BuildingsIcon />, to: '/settings/organizations' },
  { key: 'settings-users', label: 'Users', group: 'Settings', icon: <UsersIcon />, to: '/settings/users' },
  { key: 'settings-email', label: 'Email', group: 'Settings', icon: <EnvelopeIcon />, to: '/settings/email' },
  { key: 'settings-general', label: 'General', group: 'Settings', icon: <GearIcon />, to: '/settings/general' },
  { key: 'settings-updates', label: 'Updates', group: 'Settings', icon: <ArrowCircleUpIcon />, to: '/settings/updates' },
  { key: 'settings-audit-log', label: 'Audit log', group: 'Settings', icon: <ClockCounterClockwiseIcon />, to: '/settings/audit-log' },
]

function ResultRow({
  item,
  active,
  onSelect,
}: {
  item: ResultItem
  active: boolean
  onSelect: () => void
}) {
  return (
    <button
      type="button"
      id={`command-palette-option-${item.key}`}
      role="option"
      aria-selected={active}
      data-active={active}
      onClick={onSelect}
      className={cn(
        'flex w-full items-center gap-2 rounded-md px-2.5 py-1.5 text-left text-sm outline-none',
        active ? 'bg-muted text-foreground' : 'text-foreground/90 hover:bg-muted/60',
      )}
    >
      <span className="flex size-4 shrink-0 items-center justify-center text-muted-foreground [&_svg]:size-4">
        {item.icon}
      </span>
      <span className="truncate">{item.label}</span>
    </button>
  )
}

// Command palette: Cmd+K (Mac) / Ctrl+K (elsewhere) opens it from anywhere
// in the app shell. Self-contained state, but `open` can also be driven
// externally (__root.tsx's header trigger) via the open/onOpenChange props.
export function CommandPalette({
  open: openProp,
  onOpenChange,
}: {
  open?: boolean
  onOpenChange?: (open: boolean) => void
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
  const inputRef = React.useRef<HTMLInputElement>(null)
  const navigate = useNavigate()

  const appsQuery = useQuery({ ...appListQueryOptions(), enabled: open })
  const databasesQuery = useQuery({ ...databaseListQueryOptions(), enabled: open })

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
  // (not an effect) per React's "adjusting state on prop change" pattern,
  // since setState-in-effect would cascade an extra render for no benefit.
  const [prevOpen, setPrevOpen] = React.useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (open) {
      setQuery('')
      setActiveIndex(0)
    }
  }

  React.useEffect(() => {
    if (open) {
      const id = window.setTimeout(() => inputRef.current?.focus(), 0)
      return () => window.clearTimeout(id)
    }
    return undefined
  }, [open])

  const results = React.useMemo<ResultItem[]>(() => {
    const dynamic: ResultItem[] = []
    for (const app of appsQuery.data ?? []) {
      dynamic.push({
        key: `app-${app.name}`,
        label: app.name,
        group: 'Apps',
        icon: <StackIcon />,
        to: '/apps/$name',
        params: { name: app.name },
      })
    }
    for (const db of databasesQuery.data ?? []) {
      dynamic.push({
        key: `db-${db.name}`,
        label: db.name,
        group: 'Databases',
        icon: <DatabaseIcon />,
        to: '/databases/$name',
        params: { name: db.name },
      })
    }
    const all = [...STATIC_ENTRIES, ...dynamic]
    const q = query.trim().toLowerCase()
    if (!q) return all
    return all.filter((item) => item.label.toLowerCase().includes(q))
  }, [query, appsQuery.data, databasesQuery.data])

  const groups = React.useMemo(() => {
    const order: string[] = []
    const byGroup = new Map<string, ResultItem[]>()
    for (const item of results) {
      if (!byGroup.has(item.group)) {
        byGroup.set(item.group, [])
        order.push(item.group)
      }
      byGroup.get(item.group)?.push(item)
    }
    return order.map((group) => ({ group, items: byGroup.get(group) ?? [] }))
  }, [results])

  const select = React.useCallback(
    (item: ResultItem) => {
      void navigate({ to: item.to, params: item.params })
      setOpen(false)
      setQuery('')
    },
    [navigate, setOpen],
  )

  function handleInputKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex((i) => (results.length === 0 ? 0 : (i + 1) % results.length))
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
          <DialogTitle className="sr-only">Search</DialogTitle>
          <div className="flex items-center gap-2 border-b border-border px-3">
            <MagnifyingGlassIcon className="size-4 shrink-0 text-muted-foreground" />
            <Input
              ref={inputRef}
              role="combobox"
              aria-expanded={open}
              aria-controls="command-palette-listbox"
              aria-activedescendant={
                results[activeIndex]
                  ? `command-palette-option-${results[activeIndex].key}`
                  : undefined
              }
              value={query}
              onChange={(e) => {
                setQuery(e.target.value)
                setActiveIndex(0)
              }}
              onKeyDown={handleInputKeyDown}
              placeholder="Search apps, databases, settings..."
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
                <div key={group} className="mb-2 last:mb-0">
                  <p className="px-2.5 py-1 text-xs font-medium text-muted-foreground">
                    {group}
                  </p>
                  {items.map((item) => {
                    flatIndex += 1
                    const index = flatIndex
                    return (
                      <ResultRow
                        key={item.key}
                        item={item}
                        active={index === activeIndex}
                        onSelect={() => select(item)}
                      />
                    )
                  })}
                </div>
              ))
            )}
          </div>
        </DialogPrimitive.Popup>
      </DialogPortal>
    </DialogPrimitive.Root>
  )
}
