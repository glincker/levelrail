import { createFileRoute } from '@tanstack/react-router'
import { UserIcon } from '@phosphor-icons/react/dist/ssr'
import { useAuthUsername } from '../../hooks/useAuthUsername'
import { ChangePasswordCard } from '../../components/ChangePasswordCard'
import {
  Field,
  FieldDescription,
  FieldLabel,
} from '../../components/ui/field'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../components/ui/card'

// Real account page, following on from the placeholder shell this route
// used to be (see git history / the previous version of this file for
// why it was split out on its own). Two independent cards: profile
// (read-only, Phase 1 has exactly one editable-nothing admin account:
// single admin user, session auth, no teams or RBAC yet) and
// change-password (a real form against internal/api/account.go's PUT
// /api/v1/auth/password, see ChangePasswordCard.tsx for the form and its
// zod schema, kept in their own file rather than inline here so
// TanStack Router's autoCodeSplitting can cleanly move zod out of the
// always-loaded main bundle, the same convention LoginForm.tsx and
// RegisterForm.tsx already establish for /login).
export const Route = createFileRoute('/settings/account')({
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

// store.AdminUser (internal/store/admin.go) has exactly two fields:
// Username and PasswordHash. No email, display name, or avatar exist on
// the backend to show or edit here, so this card is deliberately just a
// read-only username, not a stub for fields that don't exist yet.
function ProfileCard() {
  const username = useAuthUsername()

  return (
    <Card>
      <CardHeader>
        <CardTitle>Profile</CardTitle>
        <CardDescription>
          Levelrail is single-admin in this phase: one account, no teams, no
          roles yet.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Field>
          <FieldLabel htmlFor="account-username">Username</FieldLabel>
          <div
            id="account-username"
            className="flex h-9 items-center gap-2 rounded-md border border-border bg-muted/50 px-3 text-sm text-foreground"
          >
            <UserIcon className="size-4 text-muted-foreground" />
            {username ?? 'Unknown'}
          </div>
          <FieldDescription>
            There is no other editable profile field yet in this phase, no
            email, display name, or avatar.
          </FieldDescription>
        </Field>
      </CardContent>
    </Card>
  )
}
