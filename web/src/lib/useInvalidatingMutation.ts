import {
  useMutation,
  useQueryClient,
  type MutationMeta,
  type QueryKey,
  type UseMutationResult,
} from '@tanstack/react-query'

interface InvalidatingMutationOptions<TData, TVariables> {
  mutationFn: (variables: TVariables) => Promise<TData>
  invalidate: (data: TData, variables: TVariables) => QueryKey[]
  meta?: MutationMeta
}

// Invalidates fire-and-forget after success, so mutateAsync resolves without
// waiting on refetches (the dominant pattern across query modules).
export function useInvalidatingMutation<TData, TVariables = void>({
  mutationFn,
  invalidate,
  meta,
}: InvalidatingMutationOptions<TData, TVariables>): UseMutationResult<
  TData,
  Error,
  TVariables
> {
  const queryClient = useQueryClient()
  return useMutation<TData, Error, TVariables>({
    mutationFn,
    meta,
    onSuccess: (data, variables) => {
      for (const queryKey of invalidate(data, variables)) {
        void queryClient.invalidateQueries({ queryKey })
      }
    },
  })
}
