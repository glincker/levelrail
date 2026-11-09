import { Suspense, lazy } from 'react'

// Lazy so the trigger pattern card stays out of the first-load bundle.
const GitTriggerPatternsCard = lazy(() =>
  import('./GitTriggerPatternsCard').then((m) => ({
    default: m.GitTriggerPatternsCard,
  })),
)

export function GitTriggerPatternsSlot({ appName }: { appName: string }) {
  return (
    <Suspense fallback={null}>
      <GitTriggerPatternsCard appName={appName} />
    </Suspense>
  )
}
