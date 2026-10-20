import { useEffect, useState } from 'react'
import { useReducedMotion } from '../kit/useReducedMotion'

export const REVEAL_STEP_MS = 160

/** useStaggeredReveal counts up to total one item at a time once ready is true; with reduced motion it shows everything at once. */
export function useStaggeredReveal(total: number, ready: boolean): number {
  const reduced = useReducedMotion()
  const [count, setCount] = useState(0)

  useEffect(() => {
    if (!ready || reduced) return
    const id = window.setInterval(() => {
      setCount((c) => Math.min(c + 1, total))
    }, REVEAL_STEP_MS)
    return () => {
      window.clearInterval(id)
      setCount(0)
    }
  }, [ready, total, reduced])

  if (!ready) return 0
  return reduced ? total : count
}
