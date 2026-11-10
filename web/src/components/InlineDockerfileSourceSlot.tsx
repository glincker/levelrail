import { Suspense, lazy } from 'react'

// Lazy so the rarely used card stays out of the first-load bundle.
const InlineDockerfileCard = lazy(() =>
  import('./InlineDockerfileCard').then((m) => ({
    default: m.InlineDockerfileCard,
  })),
)

export function InlineDockerfileSourceSlot({ appName }: { appName: string }) {
  return (
    <Suspense fallback={null}>
      <InlineDockerfileCard appName={appName} />
    </Suspense>
  )
}
