import { useState } from 'react'
import { CheckGlassIcon, CopyGlassIcon, MailGlassIcon } from '@thesvg/ui-react'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { useTranslation } from 'react-i18next'
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
      {copied ? (
        <CheckGlassIcon size="1.4em" />
      ) : (
        <CopyGlassIcon size="1.4em" />
      )}
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
  const { t } = useTranslation('settings')
  if (invites.length === 0) {
    return (
      <div className="flex items-center gap-3 py-4">
        <div className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-muted-foreground">
          <MailGlassIcon className="size-5" />
        </div>
        <div className="space-y-0.5">
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

  const columns: ResponsiveColumn<InviteResource>[] = [
    {
      id: 'email',
      primary: true,
      header: 'Email',
      cell: (invite) => (
        <span className="font-medium break-all text-foreground">
          {invite.email}
        </span>
      ),
    },
    {
      id: 'role',
      header: 'Role',
      priority: 2,
      cell: (invite) => invite.role ?? 'custom',
    },
    {
      id: 'sent',
      header: 'Sent',
      priority: 3,
      cell: (invite) => (
        <span className="text-muted-foreground">
          {new Date(invite.created_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      id: 'expires',
      header: 'Expires',
      priority: 3,
      cell: (invite) => (
        <span className="text-muted-foreground">
          {new Date(invite.expires_at).toLocaleDateString()}
        </span>
      ),
    },
    {
      id: 'status',
      header: 'Status',
      cell: (invite) =>
        invite.expired ? (
          <Badge variant="destructive">Expired</Badge>
        ) : (
          <Badge variant="muted">Pending</Badge>
        ),
    },
  ]

  return (
    <ResponsiveTable
      rows={invites}
      columns={columns}
      getRowId={(invite) => invite.id}
      ariaLabel={t('tables.invites')}
      getRowProps={(invite) => ({
        className: invite.expired ? 'opacity-60' : undefined,
      })}
      renderActions={(invite) => {
        const link = links[invite.id]
        return (
          <div className="flex flex-wrap items-center justify-end gap-2">
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
                <CopyGlassIcon size="1.4em" />
                Copy link
              </Button>
            )}
            {isRoot || invite.created_by === ownUserID ? (
              <RevokeInviteDialog invite={invite} />
            ) : null}
          </div>
        )
      }}
    />
  )
}
