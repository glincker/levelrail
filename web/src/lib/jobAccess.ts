import { ApiError } from './apiError'

// jobErrorMessage swaps a 403 for the plain access rule, so a refused run or
// job save says which access it needs instead of a raw ability name.
export function jobErrorMessage(error: Error, accessRule: string): string {
  return error instanceof ApiError && error.status === 403
    ? accessRule
    : error.message
}
