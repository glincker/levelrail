import { useCallback, useState } from 'react'
import { maxNavPins, readPins, togglePin, writePins } from '@/lib/navPins'

export function useNavPins() {
  const [pins, setPins] = useState<string[]>(() => readPins())
  const max = maxNavPins()
  const toggle = useCallback((id: string) => {
    setPins((prev) => {
      const next = togglePin(prev, id)
      writePins(next)
      return next
    })
  }, [])
  return { pins, toggle, full: pins.length >= max, max }
}
