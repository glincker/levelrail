import { useEffect, useState } from 'react'

// Generic debounce for a value that changes rapidly (e.g. a controlled
// search input) but should only trigger a fetch/query after typing
// settles. No debounce utility existed anywhere in web/src as of this
// pass (checked); kept generic here rather than baked into
// LogSearchPanel so a future debounced input elsewhere (e.g. an app name
// filter on the list route) can reuse it instead of re-implementing the
// same setTimeout/clearTimeout dance.
export function useDebouncedValue<T>(value: T, delayMs: number): T {
  const [debounced, setDebounced] = useState(value)

  useEffect(() => {
    const timeout = window.setTimeout(() => {
      setDebounced(value)
    }, delayMs)
    return () => {
      window.clearTimeout(timeout)
    }
  }, [value, delayMs])

  return debounced
}
