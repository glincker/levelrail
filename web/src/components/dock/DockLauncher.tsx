import * as React from 'react'
import { useTranslation } from 'react-i18next'
import { ChatCircleTextIcon } from '@phosphor-icons/react/dist/ssr'
import {
  LAUNCHER_SIZE,
  clamp,
  cornerPosition,
  nearestCorner,
} from './dockGeometry'
import { setDockCorner, setDockView, useDockState } from './dockStore'
import { useBottomAvoid, useViewport } from './useDockViewport'
import { useDragMove } from './useDragMove'

function place(el: HTMLElement | null, x: number, y: number) {
  if (el) el.style.transform = `translate3d(${String(x)}px,${String(y)}px,0)`
}

// Launcher bubble: draggable, snaps to the nearest corner on release.
export function DockLauncher() {
  const { t } = useTranslation()
  const { corner, unread } = useDockState()
  const vp = useViewport()
  const avoid = useBottomAvoid()
  const ref = React.useRef<HTMLButtonElement>(null)
  const origin = React.useRef({ x: 0, y: 0 })
  const dragged = React.useRef(false)
  const pos = cornerPosition(corner, vp, avoid)

  React.useLayoutEffect(() => {
    place(ref.current, pos.x, pos.y)
  }, [pos.x, pos.y])

  React.useEffect(() => {
    const active = document.activeElement
    if (!active || active === document.body) ref.current?.focus()
  }, [])

  const drag = useDragMove({
    enabled: true,
    allowInteractive: true,
    onStart: () => {
      origin.current = pos
      dragged.current = false
      ref.current?.setAttribute('data-dragging', 'true')
    },
    onDrag: (dx, dy) => {
      dragged.current = true
      place(
        ref.current,
        clamp(origin.current.x + dx, 0, vp.w - LAUNCHER_SIZE),
        clamp(origin.current.y + dy, 0, vp.h - LAUNCHER_SIZE),
      )
    },
    onEnd: (dx, dy, moved) => {
      ref.current?.removeAttribute('data-dragging')
      if (!moved) return
      const cx = origin.current.x + dx + LAUNCHER_SIZE / 2
      const cy = origin.current.y + dy + LAUNCHER_SIZE / 2
      const next = nearestCorner(cx, cy, vp)
      const target = cornerPosition(next, vp, avoid)
      place(ref.current, target.x, target.y)
      setDockCorner(next)
    },
  })

  return (
    <button
      ref={ref}
      type="button"
      aria-label={unread ? t('dock.launcherUnread') : t('dock.launcher')}
      aria-keyshortcuts="Control+J"
      title={t('dock.toggleHint')}
      onClick={() => {
        if (dragged.current) {
          dragged.current = false
          return
        }
        setDockView('open')
      }}
      {...drag}
      className="fixed top-0 left-0 z-40 flex max-md:in-[body:has([role=menu])]:invisible size-14 touch-none animate-in items-center justify-center rounded-full bg-primary text-primary-foreground shadow-floating transition-[transform,opacity] duration-200 outline-none fade-in-0 hover:opacity-90 focus-visible:ring-[3px] focus-visible:ring-ring/50 data-[dragging=true]:transition-none motion-reduce:animate-none motion-reduce:transition-none"
    >
      <ChatCircleTextIcon className="size-6" weight="fill" />
      {unread ? (
        <span
          aria-hidden="true"
          className="absolute top-1 right-1 size-3 rounded-full border-2 border-background bg-destructive"
        />
      ) : null}
    </button>
  )
}
