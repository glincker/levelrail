import { useEffect, useState } from 'react'

export function useMinuteNow(): number {
  const [now, setNow] = useState(Date.now)
  useEffect(() => {
    const id = window.setInterval(() => {
      setNow(Date.now())
    }, 60_000)
    return () => {
      window.clearInterval(id)
    }
  }, [])
  return now
}
