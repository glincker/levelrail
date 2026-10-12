import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { UserIcon } from '@phosphor-icons/react/dist/ssr'
import { sessionQueryOptions, useSession } from '../../queries/security'
import { ChangePasswordCard } from '../../components/ChangePasswordCard'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '../../components/ui/card'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '../../components/settings/SettingsSkeletons'
import { PageTemplate } from '@/components/shell/PageTemplate'

// Two independent cards: profile (read-only) and change-password
// (ChangePasswordCard.tsx, split out so TanStack Router's
// autoCodeSplitting can keep zod out of the always-loaded main bundle).
// No separate recovery-email card: the account's own email (User.Email)
// already covers that, see queries/security.ts's SessionInfo.
export const Route = createFileRoute('/settings/account')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(sessionQueryOptions()),
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
        <ChangePasswordCard />
      </div>
    </PageTemplate>
  )
}

function ProfileCard() {
  const { t } = useTranslation('settings')
  const { data: session } = useSession()

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t('accountPage.profile.title')}</CardTitle>
        <CardDescription>
          {t('accountPage.profile.description')}
        </CardDescription>
      </CardHeader>
      <CardContent>
        <dl className="space-y-4 text-sm">
          <div className="space-y-1">
            <dt className="text-xs font-medium text-muted-foreground">
              {t('accountPage.profile.email')}
            </dt>
            <dd className="text-foreground">{session.username}</dd>
          </div>
          <div className="space-y-1">
            <dt className="text-xs font-medium text-muted-foreground">
              {t('accountPage.profile.displayName')}
            </dt>
            <dd className="text-foreground">{session.display_name}</dd>
            <dd className="text-xs text-muted-foreground">
              {session.has_password
                ? t('accountPage.profile.hasPassword')
                : t('accountPage.profile.noPassword')}
            </dd>
          </div>
        </dl>
      </CardContent>
    </Card>
  )
}
