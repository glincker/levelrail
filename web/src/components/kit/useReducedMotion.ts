import { useEffect, useState } from 'react'
import { prefersReducedMotion } from '@/lib/reducedMotion'

const QUERY = '(prefers-reduced-motion: reduce)'

export { prefersReducedMotion }

export function useReducedMotion(): boolean {
  const [reduced, setReduced] = useState(prefersReducedMotion)
  useEffect(() => {
    if (typeof window.matchMedia !== 'function') return
    const mq = window.matchMedia(QUERY)
    const onChange = () => setReduced(mq.matches)
    onChange()
    mq.addEventListener?.('change', onChange)
    return () => mq.removeEventListener?.('change', onChange)
  }, [])
  return reduced
}
