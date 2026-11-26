import * as React from 'react'

// Only shown while the disk is full, so it stays out of the first load.
const CleanUpDockerDialog = React.lazy(() =>
  import('./CleanUpDockerDialog').then((m) => ({
    default: m.CleanUpDockerDialog,
  })),
)

export function LazyCleanUpDockerDialog() {
  return (
    <React.Suspense fallback={null}>
      <CleanUpDockerDialog />
    </React.Suspense>
  )
}
