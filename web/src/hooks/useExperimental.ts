import { useQuery } from '@tanstack/react-query'
import { devModeQueryOptions } from '../queries/devMode'

const NONE: readonly string[] = []

// Experimental features the control plane has switched on. Empty until the
// boot query resolves, so gated navigation never flashes in.
export function useExperimentalFeatures(): readonly string[] {
  const { data } = useQuery(devModeQueryOptions())
  return data?.experimental ?? NONE
}
