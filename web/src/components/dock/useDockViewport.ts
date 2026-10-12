import { useEffect, useState } from 'react'
import type { Viewport } from './dockGeometry'

export const DOCK_AVOID_ATTR = 'data-dock-avoid'

function read(): Viewport {
  return { w: window.innerWidth, h: window.innerHeight }
}

export function useViewport(): Viewport {
  const [vp, setVp] = useState<Viewport>(read)
  useEffect(() => {
    let frame = 0
    const update = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(() => {
        const next = read()
        setVp((prev) => (prev.w === next.w && prev.h === next.h ? prev : next))
      })
    }
    window.addEventListener('resize', update)
    window.addEventListener('orientationchange', update)
    return () => {
      cancelAnimationFrame(frame)
      window.removeEventListener('resize', update)
      window.removeEventListener('orientationchange', update)
    }
  }, [])
  return vp
}

// Height of the tallest fixed bottom bar (marked with data-dock-avoid) on screen.
export function useBottomAvoid(): number {
  const [avoid, setAvoid] = useState(0)
  useEffect(() => {
    let frame = 0
    const measure = () => {
      let max = 0
      document
        .querySelectorAll<HTMLElement>(`[${DOCK_AVOID_ATTR}]`)
        .forEach((el) => {
          const rect = el.getBoundingClientRect()
          if (rect.height === 0) return
          max = Math.max(max, window.innerHeight - rect.top)
        })
      const next = max > 0 ? Math.ceil(max) + 8 : 0
      setAvoid((prev) => (prev === next ? prev : next))
    }
    const schedule = () => {
      cancelAnimationFrame(frame)
      frame = requestAnimationFrame(measure)
    }
    const observer = new MutationObserver(schedule)
    observer.observe(document.body, { childList: true, subtree: true })
    window.addEventListener('resize', schedule)
    schedule()
    return () => {
      cancelAnimationFrame(frame)
      observer.disconnect()
      window.removeEventListener('resize', schedule)
    }
  }, [])
  return avoid
}
