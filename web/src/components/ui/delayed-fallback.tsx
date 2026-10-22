import { useEffect, useState } from 'react'
import { PageSpinner } from './page-spinner'

const DELAY_MS = Number(import.meta.env.VITE_ROUTE_FALLBACK_DELAY_MS ?? 150)

// Stays blank briefly so a fast chunk or namespace load swaps content without a skeleton flash.
export function DelayedFallback() {
  const [show, setShow] = useState(DELAY_MS <= 0)
  useEffect(() => {
    if (show) return
    const id = window.setTimeout(() => {
      setShow(true)
    }, DELAY_MS)
    return () => {
      window.clearTimeout(id)
    }
  }, [show])
  return show ? <PageSpinner /> : null
}
