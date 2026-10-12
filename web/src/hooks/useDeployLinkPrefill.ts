import { useMemo } from 'react'
import { parseDeployLink, type DeployLinkPrefill } from '../lib/deployLink'

/** useDeployLinkPrefill validates the route's search params once per change. */
export function useDeployLinkPrefill(
  search: Record<string, string>,
): DeployLinkPrefill {
  const key = JSON.stringify(search)
  // eslint-disable-next-line react-hooks/exhaustive-deps
  return useMemo(() => parseDeployLink(search), [key])
}
