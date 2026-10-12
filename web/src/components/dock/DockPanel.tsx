import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { cn } from '@/lib/utils'
import { filterByFeature } from '../../lib/experimental'
import { useExperimentalFeatures } from '../../hooks/useExperimental'
import { MAIN_CONTENT_ID } from '../shell/SkipLink'
import { DockHeader } from './DockHeader'
import { DockLauncher } from './DockLauncher'
import { DockSplitter } from './DockSplitter'
import { DOCK_TABS } from './dockTabs'
import type { DockLayout, FloatEdge } from './dockGeometry'
import {
  clearDockUnread,
  setDockMode,
  setDockPush,
  setDockTab,
  setDockView,
  toggleDockMaximized,
  useDockState,
} from './dockStore'
import { useDockBox } from './useDockBox'
import { useDragMove } from './useDragMove'

const SHEET_DISMISS_PX = 80

const LAYOUT_CLASS: Record<DockLayout, string> = {
  float: 'top-0 left-0 rounded-xl',
  bottom: 'inset-x-0 bottom-0 rounded-t-xl border-b-0',
  side: 'top-14 right-0 bottom-0 rounded-l-xl border-r-0',
  sheet:
    'inset-x-0 bottom-0 h-[85dvh] rounded-t-2xl border-b-0 pb-[env(safe-area-inset-bottom)]',
}

const SPLITTERS: Record<DockLayout, readonly FloatEdge[]> = {
  float: ['l', 't', 'b', 'bl'],
  bottom: ['t'],
  side: ['l'],
  sheet: [],
}

const SPLITTER_CLASS: Record<DockLayout, Partial<Record<FloatEdge, string>>> = {
  float: {
    l: 'inset-y-3 left-0 w-1.5',
    t: 'inset-x-3 top-0 h-1.5',
    b: 'inset-x-3 bottom-0 h-1.5',
    bl: 'bottom-0 left-0 size-3',
  },
  bottom: { t: 'inset-x-0 top-0 h-1.5' },
  side: { l: 'inset-y-0 left-0 w-1.5' },
  sheet: {},
}

function focusPage() {
  document.getElementById(MAIN_CONTENT_ID)?.focus({ preventScroll: true })
}

export default function DockPanel() {
  const { t } = useTranslation()
  const state = useDockState()
  const experimental = useExperimentalFeatures()
  const tabs = filterByFeature(DOCK_TABS, experimental)
  const active = tabs.find((x) => x.id === state.tab) ?? tabs[0]
  const regionRef = React.useRef<HTMLElement>(null)
  const box = useDockBox(regionRef)
  const [visited, setVisited] = React.useState<readonly string[]>([])
  const { layout } = box
  const open = state.view === 'open'

  const activeId = active?.id ?? ''
  if (open && activeId && !visited.includes(activeId)) {
    setVisited([...visited, activeId])
  }

  React.useEffect(() => {
    if (open && activeId === 'ai-chat') clearDockUnread()
  }, [open, activeId])

  const wasOpen = React.useRef(open)
  React.useEffect(() => {
    if (open && !wasOpen.current) {
      regionRef.current?.focus({ preventScroll: true })
    }
    wasOpen.current = open
  }, [open])

  const sheetDrag = useDragMove({
    enabled: layout === 'sheet',
    onDrag: (_dx, dy) => {
      if (regionRef.current) {
        regionRef.current.style.transform = `translate3d(0,${String(Math.max(0, dy))}px,0)`
      }
    },
    onEnd: (_dx, dy) => {
      if (regionRef.current) regionRef.current.style.transform = ''
      if (dy > SHEET_DISMISS_PX) setDockView('minimized')
    },
  })

  function onKeyDown(e: React.KeyboardEvent<HTMLElement>) {
    if (e.key !== 'Escape' || e.defaultPrevented) return
    const target = e.target as HTMLElement
    if (!regionRef.current?.contains(target)) return
    e.stopPropagation()
    if (target.closest('.xterm')) {
      e.preventDefault()
      focusPage()
      return
    }
    setDockView('minimized')
  }

  if (!active) return null

  return (
    <>
      <section
        ref={regionRef}
        role={layout === 'side' ? 'complementary' : 'region'}
        aria-label={layout === 'side' ? t('dock.regionSide') : t('dock.region')}
        data-layout={layout}
        tabIndex={-1}
        onKeyDown={onKeyDown}
        className={cn(
          'fixed z-40 animate-in flex-col overflow-hidden border border-border bg-background shadow-dialog transition-[opacity] duration-150 outline-none fade-in-0 data-[dragging=true]:transition-none motion-reduce:animate-none motion-reduce:transition-none',
          LAYOUT_CLASS[layout],
          open ? 'flex' : 'hidden',
        )}
      >
        {SPLITTERS[layout].map((edge) => (
          <DockSplitter
            key={edge}
            edge={edge}
            label={t(edge === 'l' ? 'dock.resizeWidth' : 'dock.resizeHeight')}
            bounds={box.bounds(edge)}
            className={SPLITTER_CLASS[layout][edge]}
            onStart={box.beginGesture}
            onResize={(dx, dy) => box.resizeBy(edge, dx, dy)}
            onEnd={box.endGesture}
            onStep={(amount) => box.stepBy(edge, amount)}
          />
        ))}
        {layout === 'sheet' ? (
          <div
            {...sheetDrag}
            className="flex shrink-0 touch-none justify-center py-2"
          >
            <span
              role="img"
              aria-label={t('dock.sheetHandle')}
              className="h-1 w-10 rounded-full bg-border"
            />
          </div>
        ) : null}
        <DockHeader
          tabs={tabs}
          activeId={active.id}
          unreadTabId={state.unread ? 'ai-chat' : null}
          box={box}
          mode={state.mode}
          maximized={state.maximized}
          push={state.push}
          onSelect={setDockTab}
          onMinimize={() => setDockView('minimized')}
          onClose={() => setDockView('closed')}
          onToggleMaximize={toggleDockMaximized}
          onMode={setDockMode}
          onTogglePush={() => setDockPush(!state.push)}
        />
        <div className="relative min-h-0 flex-1 overflow-hidden">
          {tabs.map((x) => {
            const Tab = x.component
            const selected = x.id === active.id
            // Closing ends the console session; the chat keeps streaming.
            const mounted =
              visited.includes(x.id) &&
              !(x.id === 'console' && state.view === 'closed')
            return (
              <div
                key={x.id}
                role="tabpanel"
                id={`dock-panel-${x.id}`}
                aria-labelledby={`dock-tab-${x.id}`}
                hidden={!selected}
                className="h-full min-h-0"
              >
                {mounted ? <Tab /> : null}
              </div>
            )
          })}
        </div>
      </section>
      {state.view === 'minimized' ? <DockLauncher /> : null}
    </>
  )
}
