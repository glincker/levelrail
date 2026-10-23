import { useEffect, type ReactElement } from 'react'
import { useRouter } from '@tanstack/react-router'
import { UnsavedGuardDialog } from '@/components/UnsavedGuardDialog'

// useUnsavedGuard(isDirty) blocks in-app nav and tab close while dirty; render the returned node.
// Pass plain `isDirty` and reset() the form after a save so it re-baselines. A clean form is never blocked.
export function useUnsavedGuard(isDirty: boolean): ReactElement | null {
  const router = useRouter({ warn: false })

  useEffect(() => {
    if (!isDirty) return
    const onBeforeUnload = (event: BeforeUnloadEvent) => {
      event.preventDefault()
    }
    window.addEventListener('beforeunload', onBeforeUnload)
    return () => {
      window.removeEventListener('beforeunload', onBeforeUnload)
    }
  }, [isDirty])

  // No router (isolated component tests) means nothing to block; useBlocker needs one.
  return router ? <UnsavedGuardDialog isDirty={isDirty} /> : null
}
