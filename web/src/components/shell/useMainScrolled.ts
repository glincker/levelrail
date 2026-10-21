import { useEffect, useState } from 'react'
import { MAIN_CONTENT_ID } from './SkipLink'

const THRESHOLD_PX = 8

export function useMainScrolled(): boolean {
  const [scrolled, setScrolled] = useState(false)
  useEffect(() => {
    const el = document.getElementById(MAIN_CONTENT_ID)
    if (!el) return
    let frame = 0
    const read = () => {
      frame = 0
      setScrolled(el.scrollTop > THRESHOLD_PX)
    }
    const onScroll = () => {
      if (frame === 0) frame = requestAnimationFrame(read)
    }
    read()
    el.addEventListener('scroll', onScroll, { passive: true })
    return () => {
      el.removeEventListener('scroll', onScroll)
      if (frame !== 0) cancelAnimationFrame(frame)
    }
  }, [])
  return scrolled
}
