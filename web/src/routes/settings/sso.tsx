import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { IdCardGlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Banner } from '@/components/ui/banner'
import { Badge } from '@/components/ui/badge'
import { FormSection } from '@/components/ui/form-section'
import { CopyField } from '@/components/traffic/CopyField'
import {
  SettingsCardSkeleton,
  SettingsHeaderSkeleton,
} from '@/components/settings/SettingsSkeletons'
import { SAMLConnectionList } from '@/components/settings/sso/SAMLConnectionList'
import { SSOEnforcementCard } from '@/components/settings/sso/SSOEnforcementCard'
import { SSOGroupMappingsCard } from '@/components/settings/sso/SSOGroupMappingsCard'
import { OIDCPolicyList } from '@/components/settings/sso/OIDCPolicyList'
import { SCIMCard } from '@/components/settings/sso/SCIMCard'
import {
  samlConnectionsQueryOptions,
  scimTokensQueryOptions,
  ssoMappingsQueryOptions,
  ssoSettingsQueryOptions,
} from '../../queries/sso'
import { oauthSSOPoliciesQueryOptions } from '../../queries/ssoOidc'
import { roleListQueryOptions } from '../../queries/roles'
import { requireExperimental } from '../../lib/experimental'

// Root only server-side: every /api/v1/sso route requires AbilityRoot.
export const Route = createFileRoute('/settings/sso')({
  beforeLoad: ({ context: { queryClient }, location }) =>
    requireExperimental(queryClient, 'saml-scim', location.href),
  loader: ({ context: { queryClient } }) =>
    Promise.all([
      queryClient.ensureQueryData(samlConnectionsQueryOptions()),
      queryClient.ensureQueryData(oauthSSOPoliciesQueryOptions()),
      queryClient.ensureQueryData(ssoMappingsQueryOptions()),
      queryClient.ensureQueryData(ssoSettingsQueryOptions()),
      queryClient.ensureQueryData(roleListQueryOptions()),
      queryClient.ensureQueryData(scimTokensQueryOptions()),
    ]),
  component: SSOSettingsPage,
  pendingComponent: SSOSettingsSkeleton,
})

function SSOSettingsSkeleton() {
  return (
    <div className="space-y-6" aria-hidden="true">
      <SettingsHeaderSkeleton icon />
      <SettingsCardSkeleton description={false} rows={3} />
      <SettingsCardSkeleton description={false} rows={2} />
    </div>
  )
}

function SSOSettingsPage() {
  const { t } = useTranslation('sso')
  const { data: connections } = useSuspenseQuery(samlConnectionsQueryOptions())
  const { data: oidcPolicies } = useSuspenseQuery(
    oauthSSOPoliciesQueryOptions(),
  )
  const { data: mappings } = useSuspenseQuery(ssoMappingsQueryOptions())
  const { data: settings } = useSuspenseQuery(ssoSettingsQueryOptions())
  const { data: roles } = useSuspenseQuery(roleListQueryOptions())
  const { data: scimTokens } = useSuspenseQuery(scimTokensQueryOptions())
  const on = connections.filter((c) => c.enabled && c.configured).length
  const blocked =
    settings.master_key_missing || Boolean(settings.base_url_error)

  return (
    <PageTemplate
      icon={<IdCardGlassIcon size="1.4em" />}
      eyebrow={t('page.eyebrow')}
      title={t('page.title')}
      description={t('page.description')}
      helpPath="/sso-oidc"
      status={
        <Badge variant={on > 0 ? 'success' : 'muted'}>
          {t('page.connectedCount', { count: on })}
        </Badge>
      }
      banners={
        blocked ? (
          <Banner variant="warning" title={t('page.setupBanner.title')}>
            {settings.master_key_missing
              ? t('page.setupBanner.masterKey')
              : t('page.setupBanner.baseUrl')}
          </Banner>
        ) : undefined
      }
    >
      <SAMLConnectionList connections={connections} roles={roles} />
      <OIDCPolicyList policies={oidcPolicies} roles={roles} />
      <SSOGroupMappingsCard
        key={mappings.mappings.map((m) => m.group).join('|')}
        mappings={mappings.mappings}
        roles={roles}
      />
      <SSOEnforcementCard settings={settings} />
      <SCIMCard settings={settings} tokens={scimTokens} roles={roles} />
      <FormSection
        title={t('spCert.title')}
        description={t('spCert.description')}
      >
        {settings.sp_certificate ? (
          <CopyField
            value={settings.sp_certificate}
            label={t('spCert.title')}
            multiline
          />
        ) : (
          <p className="text-sm text-muted-foreground">{t('spCert.none')}</p>
        )}
      </FormSection>
    </PageTemplate>
  )
}
