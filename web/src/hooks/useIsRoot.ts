import { useQuery } from '@tanstack/react-query'
import { userListQueryOptions } from '../queries/users'
import { useAuthUsername } from './useAuthUsername'

// Client-side "is the signed-in account root" heuristic, the same check
// routes/settings/users.tsx's own isRoot inlines for its create-user/
// invite triggers, factored out here so a second caller
// (CreateComposeFields' bind-mount helper) doesn't have to duplicate it.
// Real enforcement always happens server-side (an AbilityRoot-gated
// route rejects a non-root caller with 403 regardless of what this
// returns); this only decides whether to show a root-only control at
// all. Returns false, not undefined, while GET /api/v1/users
// (AbilityRead-gated, so any signed-in operator can load it) is still
// loading, so a root-only control never flashes on before settling off.
export function useIsRoot(): boolean {
  const ownEmail = useAuthUsername()
  const { data: users } = useQuery(userListQueryOptions())
  return (users ?? []).some(
    (u) => u.email === ownEmail && u.abilities.includes('root'),
  )
}
