import { useQuery } from '@tanstack/react-query'
import { userListQueryOptions } from '../queries/users'
import { hasAbility } from '../types/token'
import { useAuthUsername } from './useAuthUsername'

// UX-only gate like useCanWriteSensitive, but true while unknown: the server
// checks every app on its own, so only a known read-only user is disabled.
export function useCanDeploy(): boolean {
  const ownEmail = useAuthUsername()
  const { data: users } = useQuery(userListQueryOptions())
  if (!Array.isArray(users)) return true
  const own = users.find((u) => u.email === ownEmail)
  return own ? hasAbility(own.abilities, 'deploy') : true
}
