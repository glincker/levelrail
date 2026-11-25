import { useEffect, useSyncExternalStore } from 'react'
import type { NAV_ICONS } from './navIconSet'

export type NavIconName = keyof typeof NAV_ICONS

type IconSet = typeof NAV_ICONS

let loaded: IconSet | null = null
let pending: Promise<void> | null = null
const listeners = new Set<() => void>()

// Icons arrive in a deferred chunk; the placeholder keeps layout identical meanwhile.
function load(): Promise<void> {
  pending ??= import('./navIconSet').then((m) => {
    loaded = m.NAV_ICONS
    listeners.forEach((l) => l())
  })
  return pending
}

function subscribe(listener: () => void): () => void {
  listeners.add(listener)
  return () => listeners.delete(listener)
}

const getSnapshot = (): IconSet | null => loaded

export function NavGlassIcon({ name }: { name: NavIconName }) {
  const set = useSyncExternalStore(subscribe, getSnapshot, getSnapshot)
  useEffect(() => {
    void load()
  }, [])
  const Icon = set?.[name]
  return Icon ? (
    <Icon size="1.4em" />
  ) : (
    <span aria-hidden="true" className="inline-block size-[1.4em]" />
  )
}
