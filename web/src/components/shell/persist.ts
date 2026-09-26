export function readJson<T>(
  key: string,
  fallback: T,
  guard: (v: unknown) => v is T,
  storage: Pick<Storage, 'getItem'> = window.localStorage,
): T {
  try {
    const raw = storage.getItem(key)
    if (!raw) return fallback
    const parsed: unknown = JSON.parse(raw)
    return guard(parsed) ? parsed : fallback
  } catch {
    return fallback
  }
}

export function writeJson(
  key: string,
  value: unknown,
  storage: Pick<Storage, 'setItem'> = window.localStorage,
): void {
  try {
    storage.setItem(key, JSON.stringify(value))
  } catch {
    // Storage unavailable: state just does not persist.
  }
}

export function isBoolRecord(v: unknown): v is Record<string, boolean> {
  return (
    typeof v === 'object' &&
    v !== null &&
    !Array.isArray(v) &&
    Object.values(v).every((x) => typeof x === 'boolean')
  )
}
