// The drag hook keeps its position in refs and only touches them in handlers, which the refs rule cannot see through.
/* eslint-disable react-hooks/refs */
import '@/styles/palette-tokens.css'
import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { Dialog as DialogPrimitive } from '@base-ui/react/dialog'
import { useNavigate, useRouterState } from '@tanstack/react-router'
import { SearchGlassIcon, XGlassIcon } from '@thesvg/ui-react'
import {
  DialogPortal,
  DialogOverlay,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
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
import type { PaletteItem, PreviewAction } from './commandPaletteData'
import { ChipBar, PaletteFooter } from './commandPaletteEntries'
import { PaletteList } from './paletteList'
import { ModePills } from './palette/ModePills'
import { RecentBubbles } from './palette/RecentBubbles'
import { usePaletteBubbles } from '../hooks/usePaletteBubbles'
import { clearHistory, recordVisit } from '@/lib/paletteHistory'
import { parseMode } from '@/lib/paletteMode'
import { recordModeUse } from '@/lib/paletteModeUse'
import { firstIndexOfGroup } from '@/lib/paletteJump'
import { PalettePreview, PREVIEW_ID } from './palette/PalettePreview'
import { usePreviewHasContent } from './palette/usePreviewHasContent'
import { usePaletteDrag } from '../hooks/usePaletteDrag'
import { usePinDrop } from '../hooks/usePinDrop'
import { PinGhost } from './palette/PinGhost'
import { pinEntryFor } from '@/lib/paletteBubbleLayout'
import { usePaletteBubbleLayout } from '../hooks/usePaletteBubbleLayout'
import { usePaletteLanguage } from '../hooks/usePaletteLanguage'
import { SaveSearchButton, SaveSearchForm } from './paletteSaveSearch'

// Below this width the preview pane drops out and the list is the only column.
const PREVIEW_QUERY = '(min-width: 768px)'
// Phones get a fixed palette; dragging needs room to move.
const DRAG_QUERY = '(min-width: 640px)'
const REDUCED_MOTION_QUERY = '(prefers-reduced-motion: reduce)'

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
  const [historyVersion, setHistoryVersion] = React.useState(0)
  const [expanded, setExpanded] = React.useState<ReadonlySet<string>>(new Set())
  const [pending, setPending] = React.useState<PaletteItem | null>(null)
  const [saving, setSaving] = React.useState(false)
  // Results stay folded away until the user types or asks for them with Down or Tab.
  const [panelForced, setPanelForced] = React.useState(false)
  const [step, setStep] = React.useState<'language' | null>(null)
  const composing = React.useRef(false)
  const inputRef = React.useRef<HTMLInputElement>(null)
  const wide = useMediaQuery(PREVIEW_QUERY)
  const reducedMotion = useMediaQuery(REDUCED_MOTION_QUERY)
  const canDrag = useMediaQuery(DRAG_QUERY)
  const rtl = typeof document !== 'undefined' && document.dir === 'rtl'
  const { t } = useTranslation('palette')
  const drag = usePaletteDrag({
    enabled: canDrag,
    rtl,
    reducedMotion,
    describe: (o) =>
      t('drag.moved', { x: Math.round(o.x), y: Math.round(o.y) }),
  })
  const ps = usePaletteSearch(open)
  const navigate = useNavigate()
  const layout = usePaletteBubbleLayout()
  const coreItems = usePaletteBaseItems(onShowShortcuts)
  const openLanguageStep = React.useCallback(() => {
    setStep('language')
    setPanelForced(true)
    setActiveIndex(0)
  }, [])
  const language = usePaletteLanguage(openLanguageStep)
  const baseItems = React.useMemo(
    () => [...coreItems, ...language.base],
    [coreItems, language.base],
  )
  const stepView = React.useMemo(
    () =>
      step === 'language'
        ? { header: t('language.title'), items: language.step }
        : null,
    [step, language.step, t],
  )
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
    expanded,
    expand,
    currentApp,
    step: stepView,
    recentSearches,
    applyQuery: ps.applyQuery,
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
      setPanelForced(false)
      setStep(null)
      setExpanded(new Set())
      setRecentKeys(loadRecentKeys())
      setRecentSearches(loadRecentSearches())
      setHistoryVersion((v) => v + 1)
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
    setPanelForced(false)
    setStep(null)
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
      if (item.noHistory) {
        // Action items record nothing: they are not destinations.
      } else if (item.searchRef) {
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
          recordVisit(originalKey)
          setHistoryVersion((v) => v + 1)
        }
      }
      item.run()
      if (!item.keepOpen) close()
    },
    [close, ps.serverQuery, ps.text],
  )

  const runPreviewAction = React.useCallback(
    (item: PaletteItem, action: PreviewAction) => {
      if (action.confirm) {
        select({
          ...item,
          key: `${item.key}:${action.id}`,
          label: `${action.label}: ${item.label}`,
          confirm: true,
          noHistory: true,
          searchRef: undefined,
          run: action.run,
        })
        return
      }
      action.run()
      close()
    },
    [select, close],
  )

  const openPath = React.useCallback(
    (path: string) => {
      void navigate({ to: path })
      close()
    },
    [navigate, close],
  )
  const bubbles = usePaletteBubbles({
    baseItems,
    recentKeys,
    pinned: layout.pinned,
    version: historyVersion,
    openPath,
    select,
  })
  const bubbleRowRef = React.useRef<HTMLDivElement>(null)
  const ghostRef = React.useRef<HTMLDivElement>(null)
  const rowDrag = usePinDrop({
    rowRef: bubbleRowRef,
    ghostRef,
    onPin: (entry, index) => layout.pinAt(entry, index, bubbles),
  })
  const panelOpen =
    panelForced ||
    step !== null ||
    pending !== null ||
    saving ||
    ps.text !== '' ||
    ps.chips.length > 0
  const layered = panelOpen
  // Keep the panel mounted until its exit animation ends so it slides away instead of vanishing.
  const [exited, setExited] = React.useState(true)
  if (panelOpen && exited) setExited(false)
  const panelVisible = panelOpen || (!exited && !reducedMotion)

  function dropLayer() {
    if (pending) setPending(null)
    else if (saving) setSaving(false)
    else if (step) setStep(null)
    else clearAll()
  }

  function handleInputKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (composing.current || e.nativeEvent.isComposing) return
    const item = results[activeIndex]
    const count = results.length
    const digit = /^Digit([1-9])$/.exec(e.code)
    if (e.altKey && digit) {
      const n = Number(digit[1])
      if (!panelOpen) {
        const bubble = bubbles[n - 1]
        if (bubble) {
          e.preventDefault()
          bubble.onPick()
        }
        return
      }
      const target = firstIndexOfGroup(entries, n)
      if (target !== null) {
        e.preventDefault()
        setActiveIndex(target)
      }
      return
    }
    if (e.altKey && e.code === 'KeyP' && panelOpen && item) {
      const hasAction = item.preview?.actions?.some((a) => a.shortcut === 'P')
      if (!hasAction) {
        e.preventDefault()
        const entry = pinEntryFor(item)
        if (entry) layout.toggle(entry, bubbles)
        else layout.say(t('bubbles.notPinnable'))
        return
      }
    }
    const letter = /^Key([A-Z])$/.exec(e.code)
    if (e.altKey && letter && panelOpen) {
      const action = item?.preview?.actions?.find(
        (a) => a.shortcut === letter[1],
      )
      if (action && item) {
        e.preventDefault()
        runPreviewAction(item, action)
        return
      }
    }
    if (e.key === 'ArrowDown') {
      e.preventDefault()
      if (!panelOpen) {
        setPanelForced(true)
        setActiveIndex(0)
        return
      }
      setActiveIndex((i) => (count === 0 ? 0 : (i + 1) % count))
    } else if (e.key === 'ArrowUp') {
      e.preventDefault()
      if (!panelOpen) return
      setActiveIndex((i) => (count === 0 ? 0 : (i - 1 + count) % count))
    } else if (e.key === 'Enter') {
      e.preventDefault()
      if (!panelOpen) {
        setPanelForced(true)
        return
      }
      if ((e.metaKey || e.ctrlKey) && item?.secondary) {
        item.secondary()
        close()
      } else if (pending) select(pending, true)
      else if (item) select(item)
    } else if (e.key === 'Tab') {
      if (!panelOpen && !e.shiftKey) {
        e.preventDefault()
        setPanelForced(true)
        setActiveIndex(0)
      } else if (item?.secondary) {
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
  const hasPreviewContent = usePreviewHasContent(activeItem)
  const showPreview =
    panelOpen && wide && results.length > 0 && !loading && hasPreviewContent

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
          ref={drag.attach}
          data-slot="command-palette"
          className="group/popup fixed top-24 left-1/2 z-50 flex w-[calc(100%-2rem)] max-w-lg translate-x-[calc(-50%+var(--pal-x,0px))] translate-y-(--pal-y,0px) flex-col gap-2 data-[snapping=true]:transition-[translate] data-[snapping=true]:duration-150 data-[snapping=true]:ease-out text-sm text-popover-foreground duration-100 outline-none md:max-w-4xl motion-reduce:animate-none motion-reduce:duration-0 data-open:animate-in data-open:fade-in-0 data-open:zoom-in-95 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-95"
        >
          <DialogTitle className="sr-only">{t('title')}</DialogTitle>
          <div className="group/pal flex shrink-0 flex-col gap-2">
            <div className="flex flex-wrap items-center gap-3">
              <div
                data-slot="command-palette-field"
                className="banner-edge palette-pill relative flex min-w-0 flex-1 basis-56 items-center gap-3 rounded-full px-5 transition-shadow focus-within:ring-2 focus-within:ring-ring motion-safe:animate-in motion-safe:fade-in-0 motion-safe:slide-in-from-top-3 motion-safe:duration-200"
              >
                {canDrag ? (
                  <button
                    type="button"
                    aria-label={t('drag.label')}
                    aria-describedby="command-palette-drag-hint"
                    title={t('drag.label')}
                    {...drag.gripProps}
                    className="-ms-3 hidden size-7 shrink-0 cursor-grab touch-none items-center justify-center rounded-full text-muted-foreground outline-none select-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring active:cursor-grabbing sm:flex"
                  >
                    <svg
                      viewBox="0 0 8 14"
                      className="h-3.5 w-2"
                      fill="currentColor"
                      aria-hidden="true"
                    >
                      <circle cx="2" cy="2" r="1" />
                      <circle cx="6" cy="2" r="1" />
                      <circle cx="2" cy="7" r="1" />
                      <circle cx="6" cy="7" r="1" />
                      <circle cx="2" cy="12" r="1" />
                      <circle cx="6" cy="12" r="1" />
                    </svg>
                  </button>
                ) : null}
                <SearchGlassIcon className="size-6 shrink-0 text-muted-foreground" />
                <ChipBar chips={ps.chips} onRemove={ps.removeChip} />
                <input
                  ref={inputRef}
                  type="text"
                  role="combobox"
                  aria-label={t('searchLabel')}
                  aria-expanded={panelOpen}
                  aria-controls={
                    panelOpen ? 'command-palette-listbox' : undefined
                  }
                  aria-autocomplete="list"
                  aria-activedescendant={
                    panelOpen && activeItem ? optionId(activeItem) : undefined
                  }
                  aria-describedby={showPreview ? PREVIEW_ID : undefined}
                  data-bare
                  autoComplete="off"
                  autoCorrect="off"
                  spellCheck={false}
                  value={ps.text}
                  onChange={(e) => {
                    ps.setInput(e.target.value)
                    setActiveIndex(0)
                    setPending(null)
                  }}
                  onCompositionStart={() => {
                    composing.current = true
                  }}
                  onCompositionEnd={() => {
                    composing.current = false
                  }}
                  onKeyDown={handleInputKeyDown}
                  placeholder={ps.chips.length > 0 ? '' : t('placeholder')}
                  className="h-14 min-w-32 flex-1 border-0 bg-transparent p-0 shadow-none text-base text-foreground outline-none placeholder:text-muted-foreground"
                />
                {searching && ps.search.isFetching ? (
                  <span
                    role="status"
                    aria-label={t('loading')}
                    className="size-4 shrink-0 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-foreground motion-reduce:animate-none"
                  />
                ) : null}
                {ps.text !== '' ? (
                  <button
                    type="button"
                    aria-label={t('clearInput')}
                    onClick={() => {
                      ps.setInput('')
                      setActiveIndex(0)
                      inputRef.current?.focus()
                    }}
                    className="flex size-8 shrink-0 items-center justify-center rounded-full text-muted-foreground outline-none hover:bg-foreground/10 hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    <XGlassIcon size={16} />
                  </button>
                ) : null}
              </div>
              <RecentBubbles
                bubbles={bubbles}
                query={ps.text}
                onReorder={layout.reorder}
                onUnpin={layout.unpin}
                rowRef={bubbleRowRef}
              />
            </div>
            {
              <div className="flex flex-wrap items-center gap-x-3 gap-y-1 px-2 opacity-0 transition-opacity duration-200 group-focus-within/pal:opacity-100 motion-reduce:transition-none">
                <ModePills
                  active={parseMode(ps.text)?.kind ?? null}
                  onPick={(kind, prefix) => {
                    recordModeUse(kind)
                    ps.setInput(prefix)
                    setActiveIndex(0)
                    inputRef.current?.focus()
                  }}
                />
                {bubbles.length > 0 ? (
                  <button
                    type="button"
                    tabIndex={-1}
                    onClick={() => {
                      clearHistory()
                      setRecentKeys([])
                      setRecentSearches([])
                      setHistoryVersion((v) => v + 1)
                    }}
                    className="ms-auto rounded-full px-2 py-0.5 text-xs text-muted-foreground underline-offset-2 outline-none hover:text-foreground hover:underline focus-visible:ring-2 focus-visible:ring-ring"
                  >
                    {t('hints.clear')}
                  </button>
                ) : null}
              </div>
            }
          </div>
          <p id="command-palette-drag-hint" className="sr-only">
            {t('drag.hint')}
          </p>
          <p aria-live="polite" className="sr-only">
            {drag.announcement}
          </p>
          <p aria-live="polite" className="sr-only">
            {layout.announcement}
          </p>
          <PinGhost ghostRef={ghostRef} />
          <p role="status" aria-live="polite" className="sr-only">
            {panelOpen && !loading
              ? t('resultCount', { count: results.length })
              : ''}
          </p>
          {panelVisible ? (
            <div
              data-slot="command-palette-panel"
              inert={!panelOpen}
              onAnimationEnd={() => {
                if (!panelOpen) setExited(true)
              }}
              className={cn(
                'banner-edge absolute inset-x-0 top-full z-10 mt-2 flex max-h-(--pal-maxh,calc(100dvh-12rem)) min-h-0 flex-col overflow-hidden rounded-3xl group-data-[dragging=true]/popup:invisible group-data-[flip=up]/popup:top-auto group-data-[flip=up]/popup:bottom-full group-data-[flip=up]/popup:mt-0 group-data-[flip=up]/popup:mb-2 motion-safe:duration-200 motion-safe:ease-out motion-safe:fill-mode-forwards palette-panel',
                panelOpen
                  ? 'motion-safe:animate-in motion-safe:fade-in-0 motion-safe:slide-in-from-top-2 group-data-[flip=up]/popup:motion-safe:slide-in-from-bottom-2'
                  : 'motion-safe:animate-out motion-safe:fade-out-0 motion-safe:slide-out-to-top-2 group-data-[flip=up]/popup:motion-safe:slide-out-to-bottom-2',
              )}
            >
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
              <div className="flex min-h-0 flex-1">
                <div className="flex min-h-0 min-w-0 flex-1 flex-col">
                  {results.length === 0 && !loading ? (
                    <div
                      id="command-palette-listbox"
                      role="listbox"
                      aria-label={t('resultsLabel')}
                      className="p-2"
                    >
                      <p className="px-2.5 py-4 text-center text-sm text-muted-foreground">
                        {t('empty')}{' '}
                        {searching && !failed ? t('emptyBroaden') : ''}
                      </p>
                      {lastChip ? (
                        <div className="flex justify-center pb-2">
                          <Button
                            size="xs"
                            variant="outline"
                            onClick={ps.popChip}
                          >
                            {t('emptyRemoveFilter', {
                              chip: chipLabel(lastChip),
                            })}
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
                      rowDrag={rowDrag}
                    />
                  )}
                </div>
                {showPreview ? (
                  <PalettePreview
                    item={activeItem}
                    onRun={(item) => select(item)}
                    onAction={runPreviewAction}
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
            </div>
          ) : null}
        </DialogPrimitive.Popup>
      </DialogPortal>
    </DialogPrimitive.Root>
  )
}
