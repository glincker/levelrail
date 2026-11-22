import * as React from 'react'
import type { ErrorComponentProps } from '@tanstack/react-router'
import type { NotFoundResource } from './NotFoundState'

// Error views are only needed after a failure, so keep them out of first load.
const RouteErrorFallback = React.lazy(() =>
  import('./RouteErrorFallback').then((m) => ({
    default: m.RouteErrorFallback,
  })),
)

export function LazyRouteErrorFallback(
  props: ErrorComponentProps & { resource?: NotFoundResource },
) {
  return (
    <React.Suspense fallback={null}>
      <RouteErrorFallback {...props} />
    </React.Suspense>
  )
}
