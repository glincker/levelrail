import { useEffect, useRef, useState } from 'react'

// Reports whether the hero loop may run: not under reduced motion, and paused while the tab
// is hidden or the element is offscreen.
export function usePlayback<T extends Element>() {
  const ref = useRef<T>(null)
  const [reduced, setReduced] = useState(
    () =>
      typeof window.matchMedia === 'function' &&
      window.matchMedia('(prefers-reduced-motion: reduce)').matches,
  )
  const [onscreen, setOnscreen] = useState(true)
  const [tabVisible, setTabVisible] = useState(
    () => document.visibilityState !== 'hidden',
  )

  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const mq = window.matchMedia('(prefers-reduced-motion: reduce)')
    const onChange = () => setReduced(mq.matches)
    mq.addEventListener('change', onChange)
    return () => mq.removeEventListener('change', onChange)
  }, [])

  useEffect(() => {
    const onVis = () => setTabVisible(document.visibilityState !== 'hidden')
    document.addEventListener('visibilitychange', onVis)
    return () => document.removeEventListener('visibilitychange', onVis)
  }, [])

  useEffect(() => {
    const el = ref.current
    if (!el || typeof IntersectionObserver === 'undefined') return
    const io = new IntersectionObserver(([entry]) =>
      setOnscreen(entry?.isIntersecting ?? true),
    )
    io.observe(el)
    return () => io.disconnect()
  }, [])

  return { ref, motion: !reduced, paused: !onscreen || !tabVisible }
}
