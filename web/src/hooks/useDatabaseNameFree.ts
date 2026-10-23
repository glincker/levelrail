import { useQuery } from '@tanstack/react-query'
import { useDebouncedValue } from './useDebouncedValue'

const NAME_CHECK_DEBOUNCE_MS = 400

/** true when the database name is free, false when taken, undefined while unknown. */
export function useDatabaseNameFree(name: string, enabled: boolean) {
  const debounced = useDebouncedValue(name.trim(), NAME_CHECK_DEBOUNCE_MS)
  const active = enabled && debounced !== '' && debounced === name.trim()
  const query = useQuery({
    queryKey: ['databases', 'name-check', debounced],
    queryFn: async () => {
      const res = await fetch(
        `/api/v1/databases/${encodeURIComponent(debounced)}`,
      )
      return !res.ok
    },
    enabled: active,
    retry: false,
    staleTime: 0,
  })
  return active && query.isSuccess ? query.data : undefined
}
