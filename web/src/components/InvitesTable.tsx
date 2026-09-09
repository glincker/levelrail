import { useState } from 'react'
import {
  CheckIcon,
  CopyIcon,
  EnvelopeIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { RevokeInviteDialog } from './RevokeInviteDialog'
import type { InviteResource } from '../queries/invites'

function CopyLinkButton({ link }: { link: string }) {
  const [copied, setCopied] = useState(false)
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      onClick={() => {
        void navigator.clipboard.writeText(link).then(() => {
          setCopied(true)
          setTimeout(() => {
            setCopied(false)
          }, 2000)
        })
      }}
    >
      {copied ? <CheckIcon /> : <CopyIcon />}
      {copied ? 'Copied' : 'Copy link'}
    </Button>
  )
}

// InvitesTable renders the Users settings page's pending-invites list.
// links maps invite ID to its plaintext accept link, known only for
// invites created earlier in this same page session (InviteMemberDialog's
// onSuccess populates it): the server only ever stores a token's hash
// (store.Invite.TokenHash, matching api_tokens' own convention), so a row
// loaded fresh from GET /api/v1/invites has no plaintext link to offer,
// the same "shown once" constraint CreateTokenDialog's own plaintext
// token has. That row's action cell explains this instead of pretending
// a link is available.
//
// isRoot/ownUserID decide whether the Revoke button renders per row,
// mirroring handleRevokeInvite's own server-side rule (invites.go): root
// can revoke any invite, anyone else only the ones they created
// themselves. The list itself is already scoped that way by
// GET /api/v1/invites for a non-root caller, so in practice every row a
// non-root session sees is already its own, but the per-row check stays
// explicit rather than assumed.
export function InvitesTable({
  invites,
  links,
  isRoot,
  ownUserID,
}: {
  invites: InviteResource[]
  links: Record<string, string>
  isRoot: boolean
  ownUserID?: string
}) {
  if (invites.length === 0) {
    return (
      <div className="flex flex-col items-center justify-center gap-3 rounded-lg border border-dashed border-border px-6 py-12 text-center">
        <div className="flex size-10 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <EnvelopeIcon className="size-5" />
        </div>
        <div className="space-y-1">
          <p className="text-sm font-medium text-foreground">
            No pending invites
          </p>
          <p className="text-sm text-muted-foreground">
            Invite a teammate to give them their own account.
          </p>
        </div>
      </div>
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Email</TableHead>
            <TableHead>Role</TableHead>
            <TableHead>Sent</TableHead>
            <TableHead>Expires</TableHead>
            <TableHead>Status</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {invites.map((invite) => {
            const link = links[invite.id]
            return (
              <TableRow
                key={invite.id}
                className={invite.expired ? 'opacity-60' : undefined}
              >
                <TableCell className="font-medium text-foreground">
                  {invite.email}
                </TableCell>
                <TableCell>{invite.role ?? 'custom'}</TableCell>
                <TableCell className="text-muted-foreground">
                  {new Date(invite.created_at).toLocaleDateString()}
                </TableCell>
                <TableCell className="text-muted-foreground">
                  {new Date(invite.expires_at).toLocaleDateString()}
                </TableCell>
                <TableCell>
                  {invite.expired ? (
                    <Badge variant="destructive">Expired</Badge>
                  ) : (
                    <Badge variant="muted">Pending</Badge>
                  )}
                </TableCell>
                <TableCell className="text-right">
                  <div className="flex justify-end gap-2">
                    {link ? (
                      <CopyLinkButton link={link} />
                    ) : (
                      <Button
                        type="button"
                        size="sm"
                        variant="outline"
                        disabled
                        title="The invite link was only shown when this invite was created. Revoke and send a new one if it's needed again."
                      >
                        <CopyIcon />
                        Copy link
                      </Button>
                    )}
                    {isRoot || invite.created_by === ownUserID ? (
                      <RevokeInviteDialog invite={invite} />
                    ) : null}
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
