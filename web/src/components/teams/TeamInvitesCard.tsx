import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import {
  teamInvitesQueryOptions,
  useCreateTeamInvite,
  useRevokeTeamInvite,
  type TeamInviteResource,
  type TeamResource,
  type TeamRole,
} from '../../queries/teams'

function inviteRoles(myRole: TeamResource['role'], root: boolean): TeamRole[] {
  return root || myRole === 'owner'
    ? ['admin', 'developer', 'viewer']
    : ['developer', 'viewer']
}

function absoluteLink(link: string): string {
  return link.startsWith('/') ? `${window.location.origin}${link}` : link
}

export function TeamInvitesCard({
  team,
  root,
}: {
  team: TeamResource
  root: boolean
}) {
  const { t } = useTranslation('teams')
  const { data: invites = [] } = useQuery(
    teamInvitesQueryOptions(team.id, true),
  )
  const create = useCreateTeamInvite(team.id)
  const revoke = useRevokeTeamInvite(team.id)
  const roles = inviteRoles(team.role, root)
  const [email, setEmail] = useState('')
  const [role, setRole] = useState<TeamRole>('developer')
  const [hours, setHours] = useState('')
  const [link, setLink] = useState('')

  const fail = (e: Error) =>
    toast.add({
      title: t('errors.generic', { message: e.message }),
      type: 'error',
    })

  const columns: ResponsiveColumn<TeamInviteResource>[] = [
    {
      id: 'email',
      header: t('invites.email'),
      primary: true,
      cell: (inv) => (
        <span className="font-mono text-xs break-all">
          {inv.email || t('invites.link')}
        </span>
      ),
    },
    {
      id: 'role',
      header: t('invites.role'),
      priority: 2,
      cell: (inv) => t(`roles.${inv.role}`),
    },
    {
      id: 'status',
      header: t('invites.statusHeader'),
      cell: (inv) => (
        <span className="text-xs text-muted-foreground">
          {t(`invites.status.${inv.status}`)}
        </span>
      ),
    },
  ]

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('invites.title')}</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <form
          className="grid gap-3 sm:grid-cols-[1fr_10rem_10rem_auto] sm:items-end"
          onSubmit={(e) => {
            e.preventDefault()
            const n = Number.parseInt(hours, 10)
            create.mutate(
              {
                email: email.trim(),
                role,
                expires_in_hours: Number.isNaN(n) ? undefined : n,
              },
              {
                onSuccess: (inv) => {
                  setLink(absoluteLink(inv.link ?? ''))
                  setEmail('')
                  toast.add({ title: t('invites.created'), type: 'success' })
                },
                onError: fail,
              },
            )
          }}
        >
          <Field>
            <FieldLabel htmlFor="invite-email">{t('invites.email')}</FieldLabel>
            <Input
              id="invite-email"
              type="email"
              value={email}
              onChange={(e) => {
                setEmail(e.target.value)
              }}
            />
            <FieldDescription className="text-xs">
              {t('invites.emailHint')}
            </FieldDescription>
          </Field>
          <Field>
            <FieldLabel htmlFor="invite-role">{t('invites.role')}</FieldLabel>
            <Select
              value={role}
              onValueChange={(next) => {
                if (next) setRole(next)
              }}
            >
              <SelectTrigger id="invite-role" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {roles.map((r) => (
                  <SelectItem key={r} value={r}>
                    {t(`roles.${r}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </Field>
          <Field>
            <FieldLabel htmlFor="invite-hours">
              {t('invites.expires')}
            </FieldLabel>
            <Input
              id="invite-hours"
              inputMode="numeric"
              value={hours}
              onChange={(e) => {
                setHours(e.target.value)
              }}
            />
          </Field>
          <Button type="submit" disabled={create.isPending}>
            {t('invites.submit')}
          </Button>
        </form>
        {link !== '' && (
          <div className="flex items-center gap-2 rounded-md bg-muted/50 px-3 py-2">
            <code className="flex-1 truncate text-xs">{link}</code>
            <Button
              type="button"
              size="sm"
              variant="outline"
              onClick={() => {
                void navigator.clipboard
                  .writeText(link)
                  .then(() =>
                    toast.add({ title: t('invites.copied'), type: 'success' }),
                  )
              }}
            >
              <CopyGlassIcon size="1.2em" aria-hidden="true" />
              {t('invites.copy')}
            </Button>
          </div>
        )}
        {invites.length === 0 ? (
          <p className="text-sm text-muted-foreground">{t('invites.empty')}</p>
        ) : (
          <ResponsiveTable
            rows={invites}
            columns={columns}
            getRowId={(inv) => inv.id}
            ariaLabel={t('invites.tableLabel')}
            renderActions={(inv) =>
              inv.status === 'pending' ? (
                <Button
                  type="button"
                  variant="ghost"
                  size="sm"
                  onClick={() => {
                    revoke.mutate(inv.id, {
                      onSuccess: () =>
                        toast.add({
                          title: t('invites.revoked'),
                          type: 'success',
                        }),
                      onError: fail,
                    })
                  }}
                >
                  {t('invites.revoke')}
                </Button>
              ) : null
            }
          />
        )}
      </CardContent>
    </Card>
  )
}
