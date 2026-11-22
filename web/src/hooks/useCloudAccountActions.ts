import { actionsFromAccess, type CloudAccountActions } from '../lib/cloudAccess'
import { useCloudAccess } from '../queries/cloudAccounts'
import { useIsRoot } from './useIsRoot'

// Without an account id only account level (root) actions can be true.
export function useCloudAccountActions(
  accountId?: string,
): CloudAccountActions {
  const isRoot = useIsRoot()
  const access = useCloudAccess()
  return actionsFromAccess(isRoot, access.data, accountId)
}
