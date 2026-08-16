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
import { RemoveUserDialog } from './RemoveUserDialog'
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
            <TableHead>Last login</TableHead>
            <TableHead />
          </TableRow>
        </TableHeader>
        <TableBody>
          {users.map((user) => (
            <TableRow key={user.id}>
              <TableCell className="font-medium text-foreground">
                {user.email}
                {user.is_first_user ? (
                  <Badge variant="outline" className="ml-2">
                    First user
                  </Badge>
                ) : null}
              </TableCell>
              <TableCell>{user.display_name}</TableCell>
              <TableCell>{authMethods(user)}</TableCell>
              <TableCell>{formatDate(user.last_login_at, 'Never')}</TableCell>
              <TableCell className="text-right">
                <RemoveUserDialog user={user} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
