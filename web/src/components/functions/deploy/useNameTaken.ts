import { useQuery } from '@tanstack/react-query'
import { useDebouncedValue } from '../../../hooks/useDebouncedValue'

const NAME_CHECK_DEBOUNCE_MS = 400

export type NameState = 'idle' | 'checking' | 'free' | 'taken'

/** An app with this name already exists when GET /apps/{name} answers 200. */
export function useNameState(name: string, valid: boolean): NameState {
  const debounced = useDebouncedValue(name.trim(), NAME_CHECK_DEBOUNCE_MS)
  const enabled = valid && debounced !== '' && debounced === name.trim()
  const query = useQuery({
    queryKey: ['functions', 'name-check', debounced],
    queryFn: async () => {
      const res = await fetch(`/api/v1/apps/${encodeURIComponent(debounced)}`)
      return res.ok
    },
    enabled,
    retry: false,
    staleTime: 0,
  })
  if (!valid || name.trim() === '') return 'idle'
  if (!enabled || query.isPending) return 'checking'
  return query.data ? 'taken' : 'free'
}
