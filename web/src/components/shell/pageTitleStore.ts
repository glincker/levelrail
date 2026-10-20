import * as React from 'react'

let current = ''
const listeners = new Set<() => void>()

function subscribe(l: () => void): () => void {
  listeners.add(l)
  return () => {
    listeners.delete(l)
  }
}

export function setPageTitle(title: string): void {
  if (title === current) return
  current = title
  listeners.forEach((l) => {
    l()
  })
}

export function usePageTitle(): string {
  return React.useSyncExternalStore(
    subscribe,
    () => current,
    () => '',
  )
}

// Registers a plain-string page title for the condensed top bar; clears on unmount.
export function useRegisterPageTitle(title: React.ReactNode): void {
  const text = typeof title === 'string' ? title : ''
  React.useEffect(() => {
    setPageTitle(text)
    return () => {
      setPageTitle('')
    }
  }, [text])
}
