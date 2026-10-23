import { lazy, Suspense } from 'react'
import { createFileRoute } from '@tanstack/react-router'
import { useSuspenseQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { PackageIcon } from '@phosphor-icons/react/dist/ssr'
import { registryCredentialListQueryOptions } from '../../queries/registryCredentials'
import { RegistryCredentialTable } from '../../components/RegistryCredentialTable'
import { CreateRegistryCredentialDialog } from '../../components/CreateRegistryCredentialDialog'
import { TableSkeleton } from '@/components/ui/table-skeleton'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { Badge } from '@/components/ui/badge'
import { Banner } from '@/components/ui/banner'
import { pickStringSearch } from '../../lib/urlDialog'
import { RegistryVendorGrid } from '../../components/registry/RegistryVendorGrid'
import { BuiltInRegistryCard } from '../../components/registry/BuiltInRegistryCard'
import { PackageRegistriesCard } from '../../components/registry/PackageRegistriesCard'
import { SettingsPageTitle } from '../../components/settings/SettingsPageTitle'

const CloudRegistryCards = lazy(
  () => import('../../components/registry/CloudRegistryCards'),
)

// Account-level, same reasoning settings/backup-targets.tsx's own doc
// comment gives: a registry credential isn't scoped to one app, it's
// referenced by name from any app.yaml's build.registryCredential
// field, so it lives here rather than under routes/apps/.
export const Route = createFileRoute('/settings/registry-credentials')({
  validateSearch: (
    search: Record<string, unknown>,
  ): { connect?: string; ecosystem?: string } =>
    pickStringSearch(search, ['connect', 'ecosystem']),
  loader: ({ context: { queryClient } }) =>
    queryClient.ensureQueryData(registryCredentialListQueryOptions()),
  component: RegistryCredentialsPage,
  pendingComponent: RegistryCredentialsPending,
})

function RegistryCredentialsPage() {
  const { t } = useTranslation('settings')
  const { data: credentials } = useSuspenseQuery(
    registryCredentialListQueryOptions(),
  )
  const expired = credentials.filter((c) => c.expiry_status === 'expired')
  const expiring = credentials.filter(
    (c) => c.expiry_status === 'expiring_soon',
  )

  let banner = null
  if (expired.length > 0) {
    banner = (
      <Banner
        variant="critical"
        title={t('registryPage.banner.expired', { count: expired.length })}
      >
        {t('registryPage.banner.expiredBody')}
      </Banner>
    )
  } else if (expiring.length > 0) {
    banner = (
      <Banner
        variant="warning"
        title={t('registryPage.banner.expiring', { count: expiring.length })}
      >
        {t('registryPage.banner.expiringBody')}
      </Banner>
    )
  }

  return (
    <PageTemplate
      icon={<PackageIcon />}
      eyebrow={t('registryPage.eyebrow')}
      title={t('registryPage.title')}
      description={t('registryPage.description')}
      helpPath="/templates-and-registry"
      helpLabel={t('registryPage.help')}
      status={
        <Badge variant={credentials.length > 0 ? 'success' : 'muted'}>
          {t('registryPage.count', { count: credentials.length })}
        </Badge>
      }
      actions={<CreateRegistryCredentialDialog />}
      banners={banner}
    >
      <div className="space-y-8">
        <BuiltInRegistryCard />
        <Suspense fallback={null}>
          <CloudRegistryCards credentials={credentials} />
        </Suspense>
        <RegistryVendorGrid />
        <RegistryCredentialTable
          credentials={credentials}
          action={<CreateRegistryCredentialDialog />}
        />
        <PackageRegistriesCard />
      </div>
    </PageTemplate>
  )
}

// Route-level fallback for the loader's pending phase, matching
// RegistryCredentialTable's own 6-column shape so the skeleton doesn't
// jump when real rows swap in.
function RegistryCredentialsPending() {
  return (
    <div className="space-y-6">
      <SettingsPageTitle id="registry-credentials" />
      <TableSkeleton columnCount={6} />
    </div>
  )
}
