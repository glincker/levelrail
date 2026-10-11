import { useCallback, useState } from 'react'
import { isStepUpRequired, type StepUpScope } from '../queries/envProtection'

export interface StepUpRequest {
  scope: StepUpScope
  subject?: string
  run: () => Promise<void>
}

// Runs an action and, when the server answers step_up_required, holds it
// until the confirm dialog succeeds and then runs it once more.
export function useStepUpGate(onError: (error: unknown) => void) {
  const [pending, setPending] = useState<StepUpRequest | null>(null)

  const guard = useCallback(
    async (scope: StepUpScope, run: () => Promise<void>, subject?: string) => {
      try {
        await run()
      } catch (error) {
        if (isStepUpRequired(error)) {
          setPending({ scope, run, subject })
          return
        }
        onError(error)
      }
    },
    [onError],
  )

  const onVerified = useCallback(async () => {
    const request = pending
    setPending(null)
    if (!request) return
    try {
      await request.run()
    } catch (error) {
      onError(error)
    }
  }, [pending, onError])

  const cancel = useCallback(() => {
    setPending(null)
  }, [])

  return { guard, pending, onVerified, cancel }
}
