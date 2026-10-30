// POST /api/v1/mail/demo/reset (internal/api/mail_demo.go): only exists in
// local demo mode and resets the fake zone, ports and mailboxes.

import { useMutation, useQueryClient } from '@tanstack/react-query'
import { ApiError, readErrorMessage } from '../lib/apiError'

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
  const qc = useQueryClient()
  return useMutation({
    mutationFn: resetMailDemo,
    onSuccess: () => qc.invalidateQueries({ queryKey: ['mail'] }),
  })
}
