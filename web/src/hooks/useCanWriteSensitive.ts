import { useQuery } from '@tanstack/react-query'
import { userListQueryOptions } from '../queries/users'
import { hasAbility } from '../types/token'
import { useAuthUsername } from './useAuthUsername'

// UX-only gate for write:sensitive controls, same approach as useIsRoot.
// The server enforces the ability; false while the user list loads.
export function useCanWriteSensitive(): boolean {
  const ownEmail = useAuthUsername()
  const { data: users } = useQuery(userListQueryOptions())
  if (!Array.isArray(users)) return false
  const own = users.find((u) => u.email === ownEmail)
  return own ? hasAbility(own.abilities, 'write:sensitive') : false
}
