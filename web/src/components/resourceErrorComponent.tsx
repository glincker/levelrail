import type { ErrorComponentProps } from '@tanstack/react-router'
import { RouteErrorFallback } from './RouteErrorFallback'
import type { NotFoundResource } from './NotFoundState'

// Shows `resource`'s not-found state for a 404 and the shared fallback for
// everything else. Used as a route's errorComponent, in place of a bare Alert.
export function resourceErrorComponent(resource: NotFoundResource) {
  return function ResourceErrorComponent(props: ErrorComponentProps) {
    return <RouteErrorFallback {...props} resource={resource} />
  }
}
