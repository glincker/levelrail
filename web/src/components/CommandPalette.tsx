import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { useRouterState } from '@tanstack/react-router'
import { SearchGlassIcon } from '@thesvg/ui-react'
import {
  DialogPortal,
  DialogOverlay,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import {
  isRecentEligible,
  loadRecentKeys,
  pushRecentKey,
} from '@/lib/recentItems'
import { chipLabel } from '@/lib/paletteQuery'
import { loadRecentSearches, pushRecentSearch } from '@/lib/paletteSearches'
import { usePaletteBaseItems } from '../hooks/usePaletteBaseItems'
import { usePaletteEntries } from '../hooks/usePaletteEntries'
import { usePaletteSearch } from '../hooks/usePaletteSearch'
import { useMediaQuery } from '../hooks/useMediaQuery'
import { recordSearchUse } from '../queries/search'
import type { PaletteItem } from './commandPaletteData'
import { ChipBar, PaletteFooter } from './commandPaletteEntries'
import { PaletteList } from './paletteList'
import { PalettePreview, PREVIEW_ID } from './palette/PalettePreview'
import { SaveSearchButton, SaveSearchForm } from './paletteSaveSearch'

// Below this width the preview pane drops out and the list is the only column.
const PREVIEW_QUERY = '(min-width: 768px)'

// Command palette: Cmd+K (Mac) / Ctrl+K (elsewhere) opens it from anywhere
// in the app shell. `open` can also be driven externally (__root.tsx's
// header trigger) via the open/onOpenChange props.
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

  const [activeIndex, setActiveIndex] = React.useState(0)
  const [recentKeys, setRecentKeys] = React.useState<string[]>(loadRecentKeys)
  const [recentSearches, setRecentSearches] =
    React.useState<string[]>(loadRecentSearches)
  const [expanded, setExpanded] = React.useState<ReadonlySet<string>>(new Set())
  const [pending, setPending] = React.useState<PaletteItem | null>(null)
  const [saving, setSaving] = React.useState(false)
  const inputRef = React.useRef<HTMLInputElement>(null)
  const wide = useMediaQuery(PREVIEW_QUERY)
  const { t } = useTranslation('palette')
  const ps = usePaletteSearch(open)
  const baseItems = usePaletteBaseItems(onShowShortcuts)
  const currentApp = useRouterState({
    select: (st) => /^\/apps\/([^/]+)/.exec(st.location.pathname)?.[1],
  })
  const expand = React.useCallback(
    (kind: string) => setExpanded((prev) => new Set(prev).add(kind)),
    [],
  )
  const entries = usePaletteEntries({
    ps,
    baseItems,
    recentKeys,
    recentSearches,
    expanded,
    expand,
    currentApp,
  })
  const results = React.useMemo(
    () => entries.flatMap((e) => (e.type === 'item' ? [e.item] : [])),
    [entries],
  )

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

  // Reset at the moment `open` flips, during render per React's "adjusting state on prop change" pattern.
  const [prevOpen, setPrevOpen] = React.useState(open)
  if (open !== prevOpen) {
    setPrevOpen(open)
    if (open) {
      ps.reset()
      setActiveIndex(0)
      setPending(null)
      setSaving(false)
      setExpanded(new Set())
      setRecentKeys(loadRecentKeys())
      setRecentSearches(loadRecentSearches())
    }
  }

  React.useEffect(() => {
    if (open) {
      const id = window.setTimeout(() => inputRef.current?.focus(), 0)
      return () => window.clearTimeout(id)
    }
    return undefined
  }, [open])

  const searching = ps.serverQuery !== ''
  const loading = searching && ps.search.isFetching && !ps.search.data
  const failed = searching && ps.search.isError
  const lastChip = ps.chips[ps.chips.length - 1]

  const clearAll = React.useCallback(() => {
    ps.reset()
    setPending(null)
    setSaving(false)
    setExpanded(new Set())
  }, [ps])

  const close = React.useCallback(() => {
    setOpen(false)
    clearAll()
  }, [setOpen, clearAll])

  const select = React.useCallback(
    (item: PaletteItem, confirmed = false) => {
      if (item.confirm && !confirmed) {
        setPending(item)
        return
      }
      setPending(null)
      if (item.searchRef) {
        void recordSearchUse(item.searchRef)
        if (ps.serverQuery !== '') {
          setRecentSearches(pushRecentSearch(ps.serverQuery))
        }
      } else {
        if (ps.text.trim().length >= 2) {
          setRecentSearches(pushRecentSearch(ps.text))
        }
        const originalKey = item.key.replace(/^recent-/, '')
        if (isRecentEligible(originalKey)) {
          setRecentKeys(pushRecentKey(originalKey))
        }
      }
      item.run()
      if (!item.keepOpen) close()
    },
    [close, ps.serverQuery, ps.text],
  )

  const layered =
    pending !== null || saving || ps.text !== '' || ps.chips.length > 0

  function dropLayer() {
    if (pending) setPending(null)
    else if (saving) setSaving(false)
    else clearAll()
  }

  function handleInputKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    const item = results[activeIndex]
    const count = results.length
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      setActiveIndex((i) => (count === 0 ? 0 : (i + 1) % count))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      setActiveIndex((i) => (count === 0 ? 0 : (i - 1 + count) % count))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if ((e.metaKey || e.ctrlKey) && item?.secondary) {
        item.secondary()
        close()
      } else if (pending) select(pending, true)
      else if (item) select(item)
    } else if (e.key === 'Tab') {
      if (item?.secondary) {
        e.preventDefault()
        item.secondary()
        close()
      }
    } else if (e.key === 'Backspace' && ps.text === '' && lastChip) {
      e.preventDefault()
      ps.popChip()
    } else if (e.key === 'Escape') {
      e.preventDefault()
      if (layered) {
        e.stopPropagation()
        dropLayer()
      } else setOpen(false)
    }
  }

  const optionId = (item: PaletteItem) => `command-palette-option-${item.key}`
  const activeItem = results[activeIndex]
  const showPreview = wide && results.length > 0 && !loading

  return (
    <DialogPrimitive.Root
      open={open}
      onOpenChange={(next, details) => {
        if (!next && layered && details?.reason === 'escape-key') {
          details.cancel?.()
          dropLayer()
          return
        }
        setOpen(next)
      }}
    >
      <DialogPortal>
        <DialogOverlay />
        <DialogPrimitive.Popup
          data-slot="command-palette"
          className="glinui-glass-surface fixed top-24 left-1/2 z-50 grid w-full max-w-lg -translate-x-1/2 gap-0 overflow-hidden text-sm text-popover-foreground duration-100 outline-none md:max-w-3xl motion-reduce:animate-none motion-reduce:duration-0 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
        >
          <DialogTitle className="sr-only">{t('title')}</DialogTitle>
          <div className="flex flex-wrap items-center gap-2 border-b border-border px-3">
            <SearchGlassIcon className="size-5 shrink-0 text-muted-foreground" />
            <ChipBar chips={ps.chips} onRemove={ps.removeChip} />
            <Input
              ref={inputRef}
              role="combobox"
              aria-label={t('searchLabel')}
              aria-expanded={open}
              aria-controls="command-palette-listbox"
              aria-autocomplete="list"
              aria-activedescendant={
                activeItem ? optionId(activeItem) : undefined
              }
              aria-describedby={showPreview ? PREVIEW_ID : undefined}
              value={ps.text}
              onChange={(e) => {
                ps.setInput(e.target.value)
                setActiveIndex(0)
                setPending(null)
              }}
              onKeyDown={handleInputKeyDown}
              placeholder={ps.chips.length > 0 ? '' : t('placeholder')}
              className="h-11 min-w-32 flex-1 border-none px-0 shadow-none focus-visible:ring-0"
            />
          </div>
          {pending ? (
            <div
              role="alert"
              className="flex items-center gap-2 border-b border-border bg-muted/50 px-3 py-2"
            >
              <div className="min-w-0 flex-1">
                <p className="truncate font-medium">
                  {t('confirm.title', { action: pending.label })}
                </p>
                <p className="text-xs text-muted-foreground">
                  {t('confirm.description')}
                </p>
              </div>
              <Button
                size="sm"
                variant="ghost"
                onClick={() => setPending(null)}
              >
                {t('confirm.cancel')}
              </Button>
              <Button size="sm" onClick={() => select(pending, true)}>
                {t('confirm.run')}
              </Button>
            </div>
          ) : null}
          {loading ? (
            <p
              role="status"
              className="px-3 py-2 text-xs text-muted-foreground"
            >
              {t('loading')}
            </p>
          ) : null}
          {failed ? (
            <div
              role="alert"
              className="flex items-center gap-2 px-3 py-2 text-xs"
            >
              <p className="min-w-0 flex-1 text-muted-foreground">
                {t('error', { message: ps.search.error?.message ?? '' })}{' '}
                {t('errorOffline')}
              </p>
              <Button
                size="xs"
                variant="outline"
                onClick={() => void ps.search.refetch()}
              >
                {t('retry')}
              </Button>
            </div>
          ) : null}
          <div className="flex min-h-0">
            <div className="min-w-0 flex-1">
              {results.length === 0 && !loading ? (
                <div
                  id="command-palette-listbox"
                  role="listbox"
                  aria-label={t('resultsLabel')}
                  className="p-2"
                >
                  <p className="px-2.5 py-4 text-center text-sm text-muted-foreground">
                    {t('empty')} {searching && !failed ? t('emptyBroaden') : ''}
                  </p>
                  {lastChip ? (
                    <div className="flex justify-center pb-2">
                      <Button size="xs" variant="outline" onClick={ps.popChip}>
                        {t('emptyRemoveFilter', { chip: chipLabel(lastChip) })}
                      </Button>
                    </div>
                  ) : null}
                </div>
              ) : (
                <PaletteList
                  entries={entries}
                  label={t('resultsLabel')}
                  activeIndex={activeIndex}
                  query={ps.text}
                  optionId={optionId}
                  onSelect={select}
                />
              )}
            </div>
            {showPreview ? (
              <PalettePreview
                item={activeItem}
                onRun={(item) => select(item)}
                onSecondary={(item) => {
                  item.secondary?.()
                  close()
                }}
              />
            ) : null}
          </div>
          {saving ? (
            <SaveSearchForm
              query={ps.serverQuery}
              onDone={() => setSaving(false)}
            />
          ) : null}
          <PaletteFooter>
            {searching && !failed ? (
              <SaveSearchButton onToggle={() => setSaving((s) => !s)} />
            ) : null}
          </PaletteFooter>
        </DialogPrimitive.Popup>
      </DialogPortal>
    </DialogPrimitive.Root>
  )
}
