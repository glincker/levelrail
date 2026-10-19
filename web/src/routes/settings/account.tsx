import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { UserIcon } from '@phosphor-icons/react/dist/ssr'
import { sessionQueryOptions } from '../../queries/security'
import { twoFactorStatusQueryOptions } from '../../queries/twoFactor'
import { passkeysQueryOptions } from '../../queries/passkeys'
import { ChangePasswordCard } from '../../components/ChangePasswordCard'
import { TwoFactorCard } from '../../components/security/TwoFactorCard'
import { PasskeysCard } from '../../components/PasskeysCard'
import { ProfileCard } from '../../components/account/ProfileCard'
import { AccountSessionsCard } from '../../components/account/AccountSessionsCard'
import {
  LinkedProvidersCard,
  MyTokensCard,
} from '../../components/account/AccountConnectionsCards'
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
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton />
      <SettingsCardSkeleton rows={2} />
      <SettingsCardSkeleton rows={3} />
    </div>
  )
}

function AccountSettingsPage() {
  const { t } = useTranslation('settings')
  return (
    <PageTemplate
      icon={<UserIcon />}
      title={t('accountPage.title')}
      description={t('accountPage.description')}
    >
      <div className="space-y-6">
        <ProfileCard />
        <PreferencesCard />
        <EmailChangeCard />
        <div id={ACCOUNT_PASSWORD_ANCHOR}>
          <ChangePasswordCard />
        </div>
        <TwoFactorCard />
        <div id={ACCOUNT_PASSKEYS_ANCHOR}>
          <PasskeysCard />
        </div>
        <AccountSessionsCard />
        <LinkedProvidersCard />
        <MyTokensCard />
        <MyAccessCard />
        <MyActivityCard />
        <DataExportCard />
        <DangerZoneCard />
      </div>
    </PageTemplate>
  )
}
