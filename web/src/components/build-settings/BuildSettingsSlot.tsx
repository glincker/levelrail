import { Suspense, lazy } from 'react'

// Lazy so the build settings card stays out of the first-load bundle.
const BuildSettingsCard = lazy(() =>
  import('./BuildSettingsCard').then((m) => ({ default: m.BuildSettingsCard })),
)

export function BuildSettingsSlot({ appName }: { appName: string }) {
  return (
    <Suspense fallback={null}>
      <BuildSettingsCard appName={appName} />
    </Suspense>
  )
}
