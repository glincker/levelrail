import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { UserGlassIcon } from '@thesvg/ui-react'
import { sessionQueryOptions } from '../../queries/security'
import { twoFactorStatusQueryOptions } from '../../queries/twoFactor'
import { passkeysQueryOptions } from '../../queries/passkeys'
import { ChangePasswordCard } from '../../components/ChangePasswordCard'
import { TwoFactorCard } from '../../components/security/TwoFactorCard'
import { PasskeysCard } from '../../components/PasskeysCard'
import { ProfileCard } from '../../components/account/ProfileCard'
import { AccountSessionsCard } from '../../components/account/AccountSessionsCard'
import { LinkedProvidersCard } from '../../components/account/AccountConnectionsCards'
import { MyTokensCard } from '../../components/account/MyTokensCard'
import { MyAccessCard } from '../../components/account/MyAccessCard'
import { MyActivityCard } from '../../components/account/MyActivityCard'
import { PreferencesCard } from '../../components/account/PreferencesCard'
import { EmailChangeCard } from '../../components/account/EmailChangeCard'
import { DataExportCard } from '../../components/account/DataExportCard'
import { DangerZoneCard } from '../../components/account/DangerZoneCard'
import {
  ACCOUNT_PASSKEYS_ANCHOR,
  ACCOUNT_PASSWORD_ANCHOR,
} from '../../components/account/UnlinkProviderButton'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { PageTemplate } from '@/components/shell/PageTemplate'

// ChangePasswordCard is its own file so autoCodeSplitting keeps zod out of
// the always-loaded main bundle.
export const Route = createFileRoute('/settings/account')({
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(sessionQueryOptions()),
      queryClient.ensureQueryData(twoFactorStatusQueryOptions()),
      queryClient.ensureQueryData(passkeysQueryOptions()),
    ]),
  component: AccountSettingsPage,
  pendingComponent: AccountSettingsSkeleton,
})

function AccountSettingsSkeleton() {
  return (
    <div className="space-y-4" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={2} />
      <SettingsCardSkeleton rows={3} />
      <SettingsCardSkeleton rows={1} rowVariant="line" />
      <SettingsCardSkeleton rows={2} rowVariant="status" />
      <SettingsCardSkeleton rows={3} rowVariant="list" />
      <SettingsCardSkeleton rows={2} rowVariant="list" />
      <SettingsCardSkeleton rows={3} rowVariant="list" />
      <SettingsCardSkeleton rows={4} rowVariant="list" />
    </div>
  )
}

function AccountSettingsPage() {
  const { t } = useTranslation('settings')
  return (
    <PageTemplate
      icon={<UserGlassIcon size="1.4em" />}
      title={t('accountPage.title')}
      description={t('accountPage.description')}
    >
      <div className="grid items-start gap-4 [--glinr-card-pad:--spacing(4)] xl:grid-cols-2">
        <ProfileCard />
        <div className="xl:col-start-2 xl:row-span-3 xl:row-start-1">
          <PreferencesCard />
        </div>
        <EmailChangeCard />
        <div id={ACCOUNT_PASSWORD_ANCHOR}>
          <ChangePasswordCard />
        </div>
        <div id="two-factor">
          <TwoFactorCard />
        </div>
        <div id={ACCOUNT_PASSKEYS_ANCHOR}>
          <PasskeysCard />
        </div>
        <div id="sessions" className="xl:col-span-2">
          <AccountSessionsCard />
        </div>
        <div className="xl:col-span-2">
          <MyTokensCard />
        </div>
        <div className="xl:col-span-2">
          <MyAccessCard />
        </div>
        <div className="xl:col-span-2">
          <MyActivityCard />
        </div>
        <LinkedProvidersCard />
        <DataExportCard />
        <div className="xl:col-span-2">
          <DangerZoneCard />
        </div>
      </div>
    </PageTemplate>
  )
}
