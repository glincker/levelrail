import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PackageIcon, PlugsConnectedIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import { BrowseRegistryCredentialDialog } from './BrowseRegistryCredentialDialog'
import { DeleteRegistryCredentialDialog } from './DeleteRegistryCredentialDialog'
import { useTestRegistryCredential } from '../queries/registryCredentials'
import type {
  RegistryCredential,
  RegistryCredentialExpiryStatus,
} from '../types/registryCredential'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Mirrors settings/general.tsx's certStatusMeta for the same three-state
// signal (alerting.CertExpiryStatus), reused server-side for this
// resource's own expiry_status.
const EXPIRY_VARIANT: Record<
  RegistryCredentialExpiryStatus,
  'success' | 'warning' | 'destructive'
> = {
  healthy: 'success',
  expiring_soon: 'warning',
  expired: 'destructive',
}

function ExpiryCell({ credential }: { credential: RegistryCredential }) {
  const { t } = useTranslation('settings')
  if (!credential.expires_at || !credential.expiry_status) {
    return (
      <span className="text-muted-foreground">
        {t('registryPage.table.noExpiry')}
      </span>
    )
  }
  const date = new Date(credential.expires_at).toLocaleDateString()
  return (
    <div className="flex flex-col gap-1">
      <Badge
        variant={EXPIRY_VARIANT[credential.expiry_status]}
        className="w-fit"
      >
        {t(`registryPage.table.expiry.${credential.expiry_status}`)}
      </Badge>
      <span className="text-xs text-muted-foreground">
        {credential.expiry_status === 'expired'
          ? t('registryPage.table.expiredOn', { date })
          : t('registryPage.table.expiresOn', { date })}
      </span>
    </div>
  )
}

function TestButton({ credential }: { credential: RegistryCredential }) {
  const { t } = useTranslation('settings')
  const testCredential = useTestRegistryCredential()
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={testCredential.isPending}
      onClick={() => {
        testCredential.mutate(credential.id, {
          onSuccess: () => {
            toast.add({
              title: t('registryPage.table.testOk', { name: credential.name }),
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: t('registryPage.table.testFailed', {
                name: credential.name,
              }),
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <PlugsConnectedIcon className="size-3.5" aria-hidden="true" />
      {testCredential.isPending
        ? t('registryPage.table.testing')
        : t('registryPage.table.test')}
    </Button>
  )
}

// The credential's identity plus an operator-set expiry, if any: registry
// host, username, connected date, expiry status. No column for which
// services reference it (GET /api/v1/registry-credentials carries none
// of that; a service's own build.registryCredential field is where that
// link lives).
export function RegistryCredentialTable({
  credentials,
  action,
}: {
  credentials: RegistryCredential[]
  action?: ReactNode
}) {
  const { t } = useTranslation('settings')

  if (credentials.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<PackageIcon className="size-5" />}
        title={t('registryPage.empty.title')}
        description={t('registryPage.empty.body')}
        action={action}
      />
    )
  }

  const columns: ResponsiveColumn<RegistryCredential>[] = [
    {
      id: 'name',
      header: t('registryPage.table.name'),
      primary: true,
      cell: (cred) => (
        <span className="font-medium text-foreground">{cred.name}</span>
      ),
    },
    {
      id: 'host',
      header: t('registryPage.table.host'),
      cell: (cred) => (
        <span className="font-mono text-muted-foreground">
          {cred.registry_host}
        </span>
      ),
    },
    {
      id: 'username',
      header: t('registryPage.table.username'),
      priority: 2,
      cell: (cred) => (
        <span className="text-muted-foreground">{cred.username}</span>
      ),
    },
    {
      id: 'added',
      header: t('registryPage.table.added'),
      priority: 3,
      cell: (cred) => (
        <span className="text-muted-foreground">
          {formatDate(cred.created_at)}
        </span>
      ),
    },
    {
      id: 'expiry',
      header: t('registryPage.table.expiryHeader'),
      cell: (cred) => <ExpiryCell credential={cred} />,
    },
  ]

  return (
    <ResponsiveTable
      rows={credentials}
      columns={columns}
      getRowId={(cred) => cred.id}
      ariaLabel={t('registryPage.table.label')}
      renderActions={(cred) => (
        <div className="flex flex-wrap justify-end gap-2">
          <BrowseRegistryCredentialDialog credential={cred} />
          <TestButton credential={cred} />
          <DeleteRegistryCredentialDialog credential={cred} />
        </div>
      )}
    />
  )
}
