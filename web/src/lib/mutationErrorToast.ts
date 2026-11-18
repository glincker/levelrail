import type { Mutation } from '@tanstack/react-query'
import { toast } from '@/components/ui/toast'
import { ApiError, isUnauthorized } from './apiError'
import { getDemoInfo } from './demoStore'
import { errorText } from './errorText'
import { translateShared } from './activeLocale'

// Default failure toast for mutations that handle no error themselves.
// A caller opts out with meta: { errorToast: false } (e.g. it renders ApiErrorAlert).
export function toastMutationError(
  error: unknown,
  mutation: Mutation<unknown, unknown, unknown, unknown>,
): void {
  if (mutation.options.onError || mutation.meta?.errorToast === false) return
  // 401 redirects to login instead.
  if (isUnauthorized(error)) return
  // The demo shows its own single read-only notice for blocked writes.
  if (
    getDemoInfo().enabled &&
    error instanceof ApiError &&
    error.status === 403
  )
    return
  const { title, message, action } = errorText(error)
  toast.add({
    title: title ?? translateShared('apiError.toastTitle', 'That did not work'),
    description: action ? `${message} ${action}` : message,
    type: 'error',
  })
}
