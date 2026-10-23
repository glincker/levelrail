export interface PrefetchDeps {
  namespaces: readonly string[]
  isLoaded: (ns: string) => boolean
  load: (batch: string[]) => Promise<unknown>
  schedule: (run: () => void) => void
  batchSize: number
}

// Loads unloaded namespaces a few at a time while the browser is idle, so
// first visits to a screen do not suspend on a catalogue fetch.
export function prefetchNamespaces(deps: PrefetchDeps): void {
  const pending = deps.namespaces.filter((ns) => !deps.isLoaded(ns))
  const step = () => {
    const batch = pending.splice(0, Math.max(1, deps.batchSize))
    if (batch.length === 0) return
    void deps
      .load(batch)
      .catch(() => undefined)
      .then(() => {
        deps.schedule(step)
      })
  }
  deps.schedule(step)
}

export function scheduleIdle(run: () => void): void {
  if (typeof window.requestIdleCallback === 'function') {
    window.requestIdleCallback(run)
    return
  }
  window.setTimeout(run, 200)
}
