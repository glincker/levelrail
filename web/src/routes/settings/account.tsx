import { createFileRoute } from '@tanstack/react-router'
import { UserIcon } from '@phosphor-icons/react/dist/ssr'
import { sessionQueryOptions, useSession } from '../../queries/security'
import { ChangePasswordCard } from '../../components/ChangePasswordCard'
import { Field, FieldDescription, FieldLabel } from '../../components/ui/field'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../components/ui/card'

// Two independent cards: profile (read-only) and change-password
// (ChangePasswordCard.tsx, split out so TanStack Router's
// autoCodeSplitting can keep zod out of the always-loaded main bundle).
export const Route = createFileRoute('/settings/account')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(sessionQueryOptions()),
  component: AccountSettingsPage,
})

function AccountSettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-lg font-semibold text-foreground">Account</h1>
        <p className="mt-1 text-sm text-muted-foreground">
          Profile and password.
        </p>
      </div>

      <ProfileCard />
      <ChangePasswordCard />
    </div>
  )
}

function ProfileCard() {
  const { data: session } = useSession()

  return (
    <Card>
      <CardHeader>
        <CardTitle>Profile</CardTitle>
        <CardDescription>
          Every signed-in account has identical access on this platform: no
          teams or roles yet.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <Field>
          <FieldLabel htmlFor="account-email">Email</FieldLabel>
          <div
            id="account-email"
            className="flex h-9 items-center gap-2 rounded-md border border-border bg-muted/50 px-3 text-sm text-foreground"
          >
            <UserIcon className="size-4 text-muted-foreground" />
            {session.username}
          </div>
        </Field>
        <Field>
          <FieldLabel>Display name</FieldLabel>
          <div className="flex h-9 items-center rounded-md border border-border bg-muted/50 px-3 text-sm text-foreground">
            {session.display_name}
          </div>
          <FieldDescription>
            {session.has_password
              ? 'This account signs in with a password.'
              : 'This account has no password set, only OAuth sign-in.'}
          </FieldDescription>
        </Field>
      </CardContent>
    </Card>
  )
}
