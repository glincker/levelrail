import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { KeyGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { Badge } from '@/components/ui/badge'
import { oauthSettingsQueryOptions } from '../../queries/oauth'
import { OAuthProviderRow } from '../../components/OAuthProviderRow'
import { OAuthRail } from '../../components/settings/OAuthRail'
import { connectionState } from '../../lib/oauthProviders'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'

// AbilityRoot-gated server-side (PUT /api/v1/settings/oauth/{provider}),
// same precedent as /settings/ingress: configures Google/GitHub sign-in.
export const Route = createFileRoute('/settings/oauth')({
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(oauthSettingsQueryOptions()),
  component: OAuthSettingsPage,
  pendingComponent: OAuthSettingsSkeleton,
})

function OAuthSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton description={false} rows={3} />
      <SettingsCardSkeleton description={false} rows={3} />
    </div>
  )
}

function OAuthSettingsPage() {
  const { t } = useTranslation('settings')
  const { data: settings } = useSuspenseQuery(oauthSettingsQueryOptions())
  const connected = settings.filter(
    (s) => connectionState(s) === 'connected',
  ).length

  return (
    <PageTemplate
      icon={<KeyGlassIcon size="1.2em" />}
      eyebrow={t('oauthPage.eyebrow')}
      title={t('oauthPage.title')}
      description={t('oauthPage.description')}
      aside={<OAuthRail settings={settings} />}
      asideLabel={t('oauthPage.rail.label')}
      status={
        <Badge variant={connected > 0 ? 'success' : 'muted'}>
          {t('oauthPage.connectedCount', { count: connected })}
        </Badge>
      }
      banners={
        <Banner
          variant="info"
          dismissId="settings.oauth.callback-urls"
          title={t('oauthPage.banner.title')}
        >
          {t('oauthPage.banner.body')}
        </Banner>
      }
    >
      <ul className="divide-y divide-border rounded-lg border border-border bg-card px-4">
        {settings.map((s) => (
          <OAuthProviderRow key={s.provider} settings={s} />
        ))}
      </ul>
    </PageTemplate>
  )
}
