import { UserIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from './ui/table'
import { Badge } from './ui/badge'
import { Button } from './ui/button'
import { RemoveUserDialog } from './RemoveUserDialog'
import { EditUserAbilitiesDialog } from './EditUserAbilitiesDialog'
import { useAuthUsername } from '../hooks/useAuthUsername'
import { ABILITY_BADGE_VARIANT } from '../types/token'
import type { UserResource } from '../queries/users'

function formatDate(iso: string | undefined, fallback: string): string {
  return iso ? new Date(iso).toLocaleString() : fallback
}

function authMethods(user: UserResource): string {
  const methods = [...user.providers]
  if (user.has_password) {
    methods.push('password')
  }
  return methods.length > 0 ? methods.join(', ') : 'none'
}

export function UserTable({ users }: { users: UserResource[] }) {
  // The client-side mirror of "who am I signed in as" (authStore's own
  // doc comment): good enough to decide whether to show this row's edit
  // control, since PUT .../abilities enforces the real self-lockout rule
  // server-side regardless of what this heuristic gets wrong.
  const ownEmail = useAuthUsername()

  if (users.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border px-6 py-12 text-center">
        <div className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <UserIcon className="size-5" />
        </div>
        <p className="text-sm text-muted-foreground">No users found.</p>
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Email</TableHead>
            <TableHead>Display name</TableHead>
            <TableHead>Signs in with</TableHead>
            <TableHead>Abilities</TableHead>
            <TableHead>Last login</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map((user) => {
            const isSelf = ownEmail !== null && ownEmail === user.email
            return (
              <TableRow key={user.id}>
                <TableCell className="font-medium text-foreground">
                  {user.email}
                  {user.is_first_user ? (
                    <Badge variant="outline" className="ml-2">
                      First user
                    </Badge>
                  ) : null}
                  {isSelf ? (
                    <Badge variant="muted" className="ml-2">
                      You
                    </Badge>
                  ) : null}
                </TableCell>
                <TableCell>{user.display_name}</TableCell>
                <TableCell>{authMethods(user)}</TableCell>
                <TableCell>
                  <div className="flex flex-wrap gap-1">
                    {user.abilities.map((ability) => (
                      <Badge key={ability} variant={ABILITY_BADGE_VARIANT[ability]}>
                        {ability}
                      </Badge>
                    ))}
                  </div>
                </TableCell>
                <TableCell>{formatDate(user.last_login_at, 'Never')}</TableCell>
                <TableCell className="text-right">
                  <div className="flex items-center justify-end gap-2">
                    {/* isSelf disables editing your own abilities in the
                        UI itself, not just relying on the server's own
                        400 (handleUpdateUserAbilities's self-lockout
                        guard) after a wasted round trip. */}
                    {isSelf ? (
                      <Button variant="outline" size="sm" disabled title="You cannot edit your own abilities">
                        Edit abilities
                      </Button>
                    ) : (
                      <EditUserAbilitiesDialog user={user} />
                    )}
                    <RemoveUserDialog user={user} />
                  </div>
                </TableCell>
              </TableRow>
            )
          })}
        </TableBody>
      </Table>
    </div>
  )
}
