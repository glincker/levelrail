// POST /api/v1/mail/demo/reset (internal/api/mail_demo.go): only exists in
// local demo mode and resets the fake zone, ports and mailboxes.

import { ApiError, readErrorMessage } from '../lib/apiError'
import { useApiMutation } from './useApiMutation'

async function resetMailDemo(): Promise<void> {
  const res = await fetch('/api/v1/mail/demo/reset', { method: 'POST' })
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, `demo reset failed: ${res.status}`),
    )
  }
}

export function useResetMailDemo() {
  return useApiMutation({
    fn: resetMailDemo,
    invalidate: [['mail']],
  })
}
