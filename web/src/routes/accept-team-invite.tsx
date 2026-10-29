import { useState } from 'react'
import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Field, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { ApiError, readErrorMessage } from '../lib/apiError'
import { useAcceptTeamInvite } from '../queries/teams'

interface AcceptTeamInviteSearch {
  token: string
}

function validateSearch(
  search: Record<string, unknown>,
): AcceptTeamInviteSearch {
  return { token: typeof search.token === 'string' ? search.token : '' }
}

interface InvitePreview {
  team_name: string
  role: string
  email: string
}

// A 401 resolves to null instead of throwing, so the shell's global 401
// handler does not bounce a signed out invitee to the login page.
async function fetchPreview(token: string): Promise<InvitePreview | null> {
  const res = await fetch(`/api/v1/team-invites/${encodeURIComponent(token)}`)
  if (res.status === 401) {
    return null
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      await readErrorMessage(res, 'invite lookup failed'),
    )
  }
  return (await res.json()) as InvitePreview
}

// Public: a signed in visitor joins with their account; anyone else creates
// an account that belongs to the invite's team only.
export const Route = createFileRoute('/accept-team-invite')({
  validateSearch,
  component: AcceptTeamInvitePage,
})

function AcceptTeamInvitePage() {
  const { t } = useTranslation('teams')
  const { token } = Route.useSearch()
  const navigate = useNavigate()
  const accept = useAcceptTeamInvite()
  const preview = useQuery({
    queryKey: ['team-invite-preview', token],
    queryFn: () => fetchPreview(token),
    enabled: token !== '',
    retry: false,
  })
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [name, setName] = useState('')
  const invite = preview.data ?? null
  const signedIn = invite !== null
  const notSignedIn = preview.isSuccess && invite === null
  const invalid = token === '' || preview.isError

  return (
    <div className="flex min-h-screen items-center justify-center p-4">
      <Card className="w-full max-w-md">
        <CardHeader>
          <CardTitle>{t('accept.title')}</CardTitle>
          <CardDescription>
            {invite
              ? t('accept.summary', {
                  team: invite.team_name,
                  role: t(`roles.${invite.role as 'viewer'}`),
                })
              : t('accept.description')}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {invalid ? (
            <Alert variant="destructive">
              <TriangleAlertGlassIcon
                className="text-tone-warning"
                size="1.4em"
              />
              <AlertDescription>
                {token === '' ? t('accept.missing') : t('accept.invalid')}
              </AlertDescription>
            </Alert>
          ) : (
            <form
              className="space-y-3"
              onSubmit={(e) => {
                e.preventDefault()
                accept.mutate(
                  signedIn
                    ? { token }
                    : { token, email, password, display_name: name },
                  {
                    onSuccess: () => {
                      toast.add({ title: t('accept.joined'), type: 'success' })
                      void navigate({ to: '/' })
                    },
                  },
                )
              }}
            >
              <p className="text-sm text-muted-foreground">
                {signedIn
                  ? t('accept.signedInHint')
                  : t('accept.newAccountHint')}
              </p>
              {notSignedIn && (
                <>
                  <Field>
                    <FieldLabel htmlFor="tinv-email">
                      {t('accept.email')}
                    </FieldLabel>
                    <Input
                      id="tinv-email"
                      type="email"
                      autoComplete="email"
                      value={email}
                      onChange={(e) => {
                        setEmail(e.target.value)
                      }}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="tinv-name">
                      {t('accept.name')}
                    </FieldLabel>
                    <Input
                      id="tinv-name"
                      value={name}
                      onChange={(e) => {
                        setName(e.target.value)
                      }}
                    />
                  </Field>
                  <Field>
                    <FieldLabel htmlFor="tinv-password">
                      {t('accept.password')}
                    </FieldLabel>
                    <Input
                      id="tinv-password"
                      type="password"
                      autoComplete="new-password"
                      value={password}
                      onChange={(e) => {
                        setPassword(e.target.value)
                      }}
                    />
                  </Field>
                </>
              )}
              {accept.isError && (
                <Alert variant="destructive">
                  <TriangleAlertGlassIcon
                    className="text-tone-warning"
                    size="1.4em"
                  />
                  <AlertDescription>{accept.error.message}</AlertDescription>
                </Alert>
              )}
              <Button
                type="submit"
                className="w-full"
                disabled={accept.isPending || preview.isPending}
              >
                {t('accept.submit')}
              </Button>
            </form>
          )}
        </CardContent>
      </Card>
    </div>
  )
}
