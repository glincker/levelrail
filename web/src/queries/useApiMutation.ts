import {
  useMutation,
  useQueryClient,
  type MutationMeta,
  type QueryKey,
  type UseMutationResult,
} from '@tanstack/react-query'

type InvalidateSpec<TData, TVars> =
  QueryKey[] | ((vars: TVars, data: TData) => QueryKey[])

export interface ApiMutationOptions<TData, TVars> {
  fn: (vars: TVars) => Promise<TData>
  invalidate: InvalidateSpec<TData, TVars>
  onSuccess?: (data: TData, vars: TVars) => void | Promise<unknown>
  onError?: (error: Error, vars: TVars) => void
  meta?: MutationMeta
}

// Awaits the refetches so mutateAsync resolves with fresh data, which is the
// behavior of the hand-written onSuccess handlers this replaces.
export function useApiMutation<TData, TVars = void>({
  fn,
  invalidate,
  onSuccess,
  onError,
  meta,
}: ApiMutationOptions<TData, TVars>): UseMutationResult<TData, Error, TVars> {
  const queryClient = useQueryClient()
  return useMutation<TData, Error, TVars>({
    mutationFn: fn,
    meta,
    onSuccess: async (data, vars) => {
      const keys =
        typeof invalidate === 'function' ? invalidate(vars, data) : invalidate
      await Promise.all(
        keys.map((queryKey) => queryClient.invalidateQueries({ queryKey })),
      )
      await onSuccess?.(data, vars)
    },
    onError: (error, vars) => onError?.(error, vars),
  })
}
