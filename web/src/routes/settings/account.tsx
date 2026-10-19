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
import { LinkedProvidersCard } from '../../components/account/AccountConnectionsCards'
import { MyTokensCard } from '../../components/account/MyTokensCard'
import { MyAccessCard } from '../../components/account/MyAccessCard'
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
      icon={<UserIcon />}
      title={t('accountPage.title')}
      description={t('accountPage.description')}
    >
      <div className="space-y-6">
        <ProfileCard />
        <ChangePasswordCard />
        <div id="two-factor">
          <TwoFactorCard />
        </div>
        <div id="passkeys">
          <PasskeysCard />
        </div>
        <div id="sessions">
          <AccountSessionsCard />
        </div>
        <LinkedProvidersCard />
        <MyTokensCard />
        <MyAccessCard />
      </div>
    </PageTemplate>
  )
}
