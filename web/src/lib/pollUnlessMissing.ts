import { ApiError } from './apiError'

const MISSING_STATUSES = [404, 501]

export function pollUnlessMissing(intervalMs: number) {
  return (query: { state: { error: Error | null } }): number | false => {
    const err = query.state.error
    if (err instanceof ApiError && MISSING_STATUSES.includes(err.status)) {
      return false
    }
    return intervalMs
  }
}
