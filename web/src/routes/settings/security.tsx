import { Link, createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { SettingRow, SettingRows } from '@/components/ui/setting-row'
import { ShieldCheckGlassIcon, UserGlassIcon } from '@thesvg/ui-react'
import { sessionQueryOptions, useSession } from '../../queries/security'
import { formatAbsolute } from '../../lib/dateFormat'
import { twoFactorStatusQueryOptions } from '../../queries/twoFactor'
import { passkeysQueryOptions } from '../../queries/passkeys'
import { CheckFix } from '../../components/fixit/FixAction'
import { AuthEngineCard } from '../../components/AuthEngineCard'
import { DockerGuardCard } from '../../components/DockerGuardCard'
import { SignInSecurityCards } from '../../components/SignInSecurityCards'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { buttonVariants } from '@/components/ui/button'
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

function SecuritySettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={2} rowVariant="line" />
      <SettingsCardSkeleton rows={1} rowVariant="line" />
      <SettingsCardSkeleton description={false} rows={1} rowVariant="line" />
    </div>
  )
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
                  {formatAbsolute(session.expires_at)}
                </span>
              </SettingRow>
            </SettingRows>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle>{t('securityPage.account.title')}</CardTitle>
            <CardDescription>
              {t('securityPage.account.description')}
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Link
              to="/settings/account"
              className={buttonVariants({ variant: 'outline', size: 'sm' })}
            >
              <UserGlassIcon size="1.4em" aria-hidden="true" />
              {t('securityPage.account.open')}
            </Link>
          </CardContent>
        </Card>

        <SignInSecurityCards />

        <AuthEngineCard />

        <DockerGuardCard />

        <Card>
          <CardHeader>
            <CardTitle className="flex items-center gap-2">
              <ShieldCheckGlassIcon className="size-5 text-muted-foreground" />
              {t('securityPage.loginProtection.title')}
            </CardTitle>
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground">
              {t('securityPage.loginProtection.body')}
            </p>
          </CardContent>
        </Card>
      </div>
    </SecurityPageShell>
  )
}
