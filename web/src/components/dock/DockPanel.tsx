import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { XIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { DOCK_TABS } from './dockTabs'
import {
  DOCK_MIN_HEIGHT,
  persistDockHeight,
  setDockHeight,
  setDockOpen,
  useDockHeight,
  useDockOpen,
} from './dockStore'

const KEY_STEP = 24

export default function DockPanel() {
  const { t } = useTranslation()
  const open = useDockOpen()
  const height = useDockHeight()
  const [activeId, setActiveId] = React.useState(DOCK_TABS[0]?.id ?? '')
  const regionRef = React.useRef<HTMLElement>(null)
  const dragRef = React.useRef<{ startY: number; startHeight: number } | null>(
    null,
  )
  const tab = DOCK_TABS.find((x) => x.id === activeId) ?? DOCK_TABS[0]

  React.useEffect(() => {
    if (open) regionRef.current?.focus()
  }, [open])

  // Height is dynamic user state; set on the node directly to keep JSX style-free.
  React.useLayoutEffect(() => {
    if (regionRef.current) regionRef.current.style.height = `${height}px`
  }, [height, open])

  if (!open || !tab) return null
  const ActiveTab = tab.component

  function onPointerDown(e: React.PointerEvent<HTMLDivElement>) {
    dragRef.current = { startY: e.clientY, startHeight: height }
    e.currentTarget.setPointerCapture(e.pointerId)
  }
  function onPointerMove(e: React.PointerEvent<HTMLDivElement>) {
    const drag = dragRef.current
    if (!drag) return
    setDockHeight(drag.startHeight + (drag.startY - e.clientY), false)
  }
  function onPointerUp() {
    if (!dragRef.current) return
    dragRef.current = null
    persistDockHeight()
  }
  function onHandleKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'ArrowUp') {
      e.preventDefault()
      setDockHeight(height + KEY_STEP)
    } else if (e.key === 'ArrowDown') {
      e.preventDefault()
      setDockHeight(height - KEY_STEP)
    }
  }
  function onKeyDown(e: React.KeyboardEvent) {
    if (e.key === 'Escape') {
      e.stopPropagation()
      setDockOpen(false)
    }
  }

  return (
    <section
      ref={regionRef}
      role="region"
      aria-label={t('dock.region')}
      tabIndex={-1}
      onKeyDown={onKeyDown}
      className="flex shrink-0 flex-col border-t border-border bg-background outline-none"
    >
      <div
        role="separator"
        aria-orientation="horizontal"
        aria-label={t('dock.resize')}
        aria-valuemin={DOCK_MIN_HEIGHT}
        aria-valuenow={height}
        tabIndex={0}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onKeyDown={onHandleKeyDown}
        className="h-1.5 shrink-0 cursor-row-resize touch-none bg-border/40 hover:bg-border focus-visible:bg-ring"
      />
      <div className="flex shrink-0 items-center gap-1 border-b border-border px-2 py-1">
        <div role="tablist" className="flex items-center gap-1">
          {DOCK_TABS.map((x) => {
            const Icon = x.icon
            const selected = x.id === tab.id
            return (
              <Button
                key={x.id}
                type="button"
                role="tab"
                aria-selected={selected}
                variant={selected ? 'secondary' : 'ghost'}
                size="sm"
                onClick={() => setActiveId(x.id)}
              >
                <Icon />
                {t(x.labelKey)}
              </Button>
            )
          })}
        </div>
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          className="ml-auto"
          aria-label={t('dock.close')}
          onClick={() => setDockOpen(false)}
        >
          <XIcon />
        </Button>
      </div>
      <div role="tabpanel" className="min-h-0 flex-1 overflow-hidden">
        <ActiveTab />
      </div>
    </section>
  )
}
