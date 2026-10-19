import { createFileRoute } from '@tanstack/react-router'
import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SettingRow, SettingRows } from '@/components/ui/setting-row'
import {
  ShieldCheckIcon,
  SignOutIcon,
  WarningIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  sessionQueryOptions,
  useRevokeOtherSessions,
  useSession,
} from '../../queries/security'
import { twoFactorStatusQueryOptions } from '../../queries/twoFactor'
import { passkeysQueryOptions } from '../../queries/passkeys'
import { CheckFix } from '../../components/fixit/FixAction'
import { PasskeysCard } from '../../components/PasskeysCard'
import { AuthEngineCard } from '../../components/AuthEngineCard'
import { DockerGuardCard } from '../../components/DockerGuardCard'
import { SignInSecurityCards } from '../../components/SignInSecurityCards'
import { TwoFactorCard } from '../../components/security/TwoFactorCard'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Button } from '@/components/ui/button'
import { Banner } from '@/components/ui/banner'
import { Alert, AlertDescription } from '@/components/ui/alert'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'
import { SecurityPageShell } from '../../components/settings/SecurityPageShell'

// Loader-primed the same way routes/settings/tokens.tsx primes
// tokenListQueryOptions: the component below only ever reads that warm
// cache via useSuspenseQuery, never fetches data in its own body (no
// data fetching in component bodies is a project-wide rule).
export const Route = createFileRoute('/settings/security')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(sessionQueryOptions()),
      queryClient.ensureQueryData(twoFactorStatusQueryOptions()),
      queryClient.ensureQueryData(passkeysQueryOptions()),
    ]),
  component: SecuritySettingsPage,
  pendingComponent: SecuritySettingsSkeleton,
})

// Mirrors this page's own five cards (current session, other sessions,
// two-factor, passkeys, login protection): each renders mostly text and
// a button rather than form fields, so every card uses the 'line' row
// variant.
function SecuritySettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={2} rowVariant="line" />
      <SettingsCardSkeleton rows={1} rowVariant="line" />
      <SettingsCardSkeleton rows={1} rowVariant="line" />
      <SettingsCardSkeleton rows={1} rowVariant="line" />
      <SettingsCardSkeleton description={false} rows={1} rowVariant="line" />
    </div>
  )
}

// Matches TokenTable.tsx's own formatDate convention: toLocaleString(),
// no separate date-formatting library, same as every other date already
// rendered in this app.
function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

function SecuritySettingsPage() {
  const { t } = useTranslation('settings')
  const { data: session } = useSession()

  return (
    <SecurityPageShell>
      <div className="space-y-6">
        <CheckFix checkId="dashboard_url_https" />

        <Card id="sessions">
          <CardHeader>
            <CardTitle>{t('securityPage.session.title')}</CardTitle>
            <CardDescription>
              {t('securityPage.session.description')}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <SettingRows>
              <SettingRow
                title={t('securityPage.session.account')}
                description={t('securityPage.session.accountHelp')}
              >
                <span className="text-sm font-medium text-foreground">
                  {session.username}
                </span>
              </SettingRow>
              <SettingRow
                title={t('securityPage.session.until')}
                description={t('securityPage.session.untilHelp')}
              >
                <span className="text-sm font-medium text-foreground">
                  {formatDate(session.expires_at)}
                </span>
              </SettingRow>
            </SettingRows>
          </CardContent>
        </Card>

        <OtherSessionsCard />

        <SignInSecurityCards />

        <div id="two-factor">
          <TwoFactorCard />
        </div>

        <div id="passkeys">
          <PasskeysCard />
        </div>

        <AuthEngineCard />

        <DockerGuardCard />

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheckIcon className="size-4 text-muted-foreground" />
              Login protection
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              Failed sign-in attempts are rate limited per IP address and
              username, with each additional failure making the next allowed
              attempt wait longer than the last. A handful of mistyped passwords
              will never lock you out, but repeated automated guessing gets
              slower with every try.
            </p>
          </CardContent>
        </Card>
      </div>
    </SecurityPageShell>
  )
}

// Own card, own dialog: sits between the read-only "current session"
// card and the static login-protection panel, matching
// RevokeTokenDialog.tsx's confirm-then-mutate shape for the same
// reason, this is a real, meaningful, if not strictly irreversible,
// security action (every other live session for this admin account is
// ended), not a one-click toggle.
function OtherSessionsCard() {
  const [open, setOpen] = useState(false)
  const [justRevoked, setJustRevoked] = useState(false)
  const revokeOtherSessions = useRevokeOtherSessions()

  function handleOpenChange(next: boolean) {
    setOpen(next)
    if (!next) {
      revokeOtherSessions.reset()
    }
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Other sessions</CardTitle>
        <CardDescription>
          Sign out every other browser or device currently signed in to this
          account, without changing your password. This browser stays signed in.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {justRevoked ? (
          <Banner variant="success" title="Other sessions signed out">
            Every other session for this account has been ended. This browser
            was not affected.
          </Banner>
        ) : null}
        <Dialog open={open} onOpenChange={handleOpenChange}>
          <DialogTrigger render={<Button variant="outline" />}>
            <SignOutIcon />
            Sign out other sessions
          </DialogTrigger>
          <DialogContent className="sm:max-w-sm">
            <DialogHeader>
              <DialogTitle className="flex items-center gap-2">
                <WarningIcon className="size-4 text-destructive" />
                Sign out other sessions?
              </DialogTitle>
              <DialogDescription>
                Any other browser or device currently signed in to this account
                will be signed out immediately. This browser is not affected.
              </DialogDescription>
            </DialogHeader>
            {revokeOtherSessions.isError ? (
              <Alert variant="destructive">
                <WarningIcon />
                <AlertDescription>
                  {revokeOtherSessions.error.message}
                </AlertDescription>
              </Alert>
            ) : null}
            <DialogFooter>
              <Button
                type="button"
                variant="outline"
                onClick={() => {
                  handleOpenChange(false)
                }}
              >
                Cancel
              </Button>
              <Button
                type="button"
                disabled={revokeOtherSessions.isPending}
                onClick={() => {
                  revokeOtherSessions.mutate(undefined, {
                    onSuccess: () => {
                      setOpen(false)
                      setJustRevoked(true)
                    },
                  })
                }}
              >
                {revokeOtherSessions.isPending
                  ? 'Signing out...'
                  : 'Sign out other sessions'}
              </Button>
            </DialogFooter>
          </DialogContent>
        </Dialog>
      </CardContent>
    </Card>
  )
}
