import type { ErrorComponentProps } from '@tanstack/react-router'
import { LazyRouteErrorFallback } from './LazyRouteErrorFallback'
import type { NotFoundResource } from './NotFoundState'

// Shows `resource`'s not-found state for a 404 and the shared fallback for
// everything else. Used as a route's errorComponent, in place of a bare Alert.
export function resourceErrorComponent(resource: NotFoundResource) {
  return function ResourceErrorComponent(props: ErrorComponentProps) {
    return <LazyRouteErrorFallback {...props} resource={resource} />
  }
}
