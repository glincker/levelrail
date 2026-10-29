import * as React from 'react'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  EllipsisGlassIcon,
  Maximize2GlassIcon,
  MinusGlassIcon,
  ShrinkGlassIcon,
  XGlassIcon,
} from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { KEY_STEP, KEY_STEP_LARGE, type DockMode } from './dockGeometry'
import type { DockTab } from './dockTabs'
import type { useDockBox } from './useDockBox'
import { useDragMove } from './useDragMove'

const MODES: readonly DockMode[] = ['float', 'bottom', 'side']

interface Props {
  tabs: readonly DockTab[]
  activeId: string
  unreadTabId: string | null
  box: ReturnType<typeof useDockBox>
  mode: DockMode
  maximized: boolean
  push: boolean
  onSelect: (id: string) => void
  onMinimize: () => void
  onClose: () => void
  onToggleMaximize: () => void
  onMode: (mode: DockMode) => void
  onTogglePush: () => void
}

export function DockHeader({
  tabs,
  activeId,
  unreadTabId,
  box,
  mode,
  maximized,
  push,
  onSelect,
  onMinimize,
  onClose,
  onToggleMaximize,
  onMode,
  onTogglePush,
}: Props) {
  const { t } = useTranslation()
  const movable = box.layout === 'float' && !maximized
  const tablistRef = React.useRef<HTMLDivElement>(null)
  const drag = useDragMove({
    enabled: movable,
    onStart: box.beginGesture,
    onDrag: box.moveBy,
    onEnd: (_dx, _dy, moved) => {
      if (moved) box.endGesture()
      else box.cancelGesture()
    },
  })

  function onHeaderKeyDown(e: React.KeyboardEvent<HTMLDivElement>) {
    if (e.target !== e.currentTarget || !movable) return
    const step = e.shiftKey ? KEY_STEP_LARGE : KEY_STEP
    const move: Record<string, [number, number]> = {
      ArrowLeft: [-step, 0],
      ArrowRight: [step, 0],
      ArrowUp: [0, -step],
      ArrowDown: [0, step],
    }
    const delta = move[e.key]
    if (delta) {
      e.preventDefault()
      box.nudge(delta[0], delta[1])
    } else if (e.key === 'Home') {
      e.preventDefault()
      box.resetPosition()
    }
  }

  function onTabsKeyDown(e: React.KeyboardEvent) {
    if (e.key !== 'ArrowRight' && e.key !== 'ArrowLeft') return
    const index = tabs.findIndex((x) => x.id === activeId)
    const dir = e.key === 'ArrowRight' ? 1 : -1
    const next = tabs[(index + dir + tabs.length) % tabs.length]
    if (!next) return
    e.preventDefault()
    onSelect(next.id)
    tablistRef.current
      ?.querySelector<HTMLElement>(`[data-tab-id="${next.id}"]`)
      ?.focus()
  }

  function onDoubleClick(e: React.MouseEvent) {
    if ((e.target as HTMLElement).closest('button,[role="tab"]')) return
    if (box.layout !== 'sheet') onToggleMaximize()
  }

  const MaxIcon = maximized ? ShrinkGlassIcon : Maximize2GlassIcon

  return (
    <div
      role="group"
      aria-label={t('dock.header')}
      tabIndex={movable ? 0 : undefined}
      aria-description={movable ? t('dock.moveHint') : undefined}
      onKeyDown={onHeaderKeyDown}
      onDoubleClick={onDoubleClick}
      {...drag}
      className={cn(
        'flex shrink-0 items-center gap-1 border-b border-border px-2 py-1 outline-none focus-visible:ring-2 focus-visible:ring-ring/60 focus-visible:ring-inset',
        movable && 'cursor-grab touch-none active:cursor-grabbing',
      )}
    >
      <div
        ref={tablistRef}
        role="tablist"
        aria-label={t('dock.tabsLabel')}
        onKeyDown={onTabsKeyDown}
        className="flex min-w-0 flex-1 items-center gap-1 overflow-x-auto [scrollbar-width:none] [&::-webkit-scrollbar]:hidden"
      >
        {tabs.map((x) => {
          const Icon = x.icon
          const selected = x.id === activeId
          return (
            <Button
              key={x.id}
              type="button"
              role="tab"
              id={`dock-tab-${x.id}`}
              data-tab-id={x.id}
              aria-selected={selected}
              aria-controls={`dock-panel-${x.id}`}
              tabIndex={selected ? 0 : -1}
              variant={selected ? 'secondary' : 'ghost'}
              size="sm"
              className="relative shrink-0"
              onClick={() => onSelect(x.id)}
            >
              <Icon />
              {t(x.labelKey)}
              {unreadTabId === x.id ? (
                <span
                  role="img"
                  aria-label={t('dock.unread')}
                  className="absolute top-1 right-1 size-2 rounded-full bg-primary"
                />
              ) : null}
            </Button>
          )
        })}
      </div>
      <DropdownMenu>
        <DropdownMenuTrigger
          render={
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              aria-label={t('dock.menu')}
            />
          }
        >
          <EllipsisGlassIcon size="1.4em" />
        </DropdownMenuTrigger>
        <DropdownMenuContent align="end">
          {box.layout === 'sheet'
            ? null
            : MODES.map((m) => (
                <DropdownMenuItem key={m} onClick={() => onMode(m)}>
                  <CheckGlassIcon
                    size="1.4em"
                    className={m === mode ? '' : 'invisible'}
                  />
                  {t(`dock.mode.${m}`)}
                </DropdownMenuItem>
              ))}
          {box.layout === 'sheet' ? null : <DropdownMenuSeparator />}
          {box.layout === 'sheet' ? null : (
            <DropdownMenuItem
              onClick={onTogglePush}
              disabled={mode === 'float'}
            >
              <CheckGlassIcon
                size="1.4em"
                className={push ? '' : 'invisible'}
              />
              {t('dock.push')}
            </DropdownMenuItem>
          )}
          <DropdownMenuItem onClick={box.resetPosition}>
            <span className="size-4" aria-hidden="true" />
            {t('dock.reset')}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      {box.layout === 'sheet' ? null : (
        <Button
          type="button"
          variant="ghost"
          size="icon-sm"
          aria-label={maximized ? t('dock.restore') : t('dock.maximize')}
          aria-pressed={maximized}
          onClick={onToggleMaximize}
        >
          <MaxIcon />
        </Button>
      )}
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={t('dock.minimize')}
        onClick={onMinimize}
      >
        <MinusGlassIcon size="1.4em" />
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="icon-sm"
        aria-label={t('dock.close')}
        onClick={onClose}
      >
        <XGlassIcon size="1.4em" />
      </Button>
    </div>
  )
}
