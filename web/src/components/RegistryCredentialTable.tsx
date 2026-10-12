import { useMemo, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { PackageIcon, PlugsConnectedIcon } from '@phosphor-icons/react/dist/ssr'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
import { BrandTile } from './BrandLogoBadge'
import { registryBrandForHost } from '../lib/providerBrands'
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
const expiryStatusMeta: Record<
  RegistryCredentialExpiryStatus,
  { label: string; variant: 'success' | 'warning' | 'destructive' }
> = {
  healthy: { label: 'Healthy', variant: 'success' },
  expiring_soon: { label: 'Expiring soon', variant: 'warning' },
  expired: { label: 'Expired', variant: 'destructive' },
}

function ExpiryCell({ credential }: { credential: RegistryCredential }) {
  if (!credential.expires_at || !credential.expiry_status) {
    return <span className="text-muted-foreground">No expiry set</span>
  }
  const meta = expiryStatusMeta[credential.expiry_status]
  return (
    <div className="flex flex-col gap-1">
      <Badge variant={meta.variant} className="w-fit">
        {meta.label}
      </Badge>
      <span className="text-xs text-muted-foreground">
        {credential.expiry_status === 'expired' ? 'Expired' : 'Expires'}{' '}
        {new Date(credential.expires_at).toLocaleDateString()}
      </span>
    </div>
  )
}

function TestButton({ credential }: { credential: RegistryCredential }) {
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
              title: `"${credential.name}" authenticated successfully.`,
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: `"${credential.name}" failed to authenticate.`,
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <PlugsConnectedIcon className="size-3.5" aria-hidden="true" />
      {testCredential.isPending ? 'Testing...' : 'Test connection'}
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
  const { t } = useTranslation('common')
  const columns = useMemo<ResponsiveColumn<RegistryCredential>[]>(
    () => [
      {
        id: 'name',
        header: 'Name',
        primary: true,
        cell: (c) => (
          <span className="font-medium text-foreground">{c.name}</span>
        ),
      },
      {
        id: 'host',
        header: 'Registry host',
        cell: (c) => (
          <span className="inline-flex items-center gap-2 font-mono break-all text-muted-foreground">
            <BrandTile
              name={registryBrandForHost(c.registry_host)}
              label={c.registry_host}
              size="xs"
            />
            {c.registry_host}
          </span>
        ),
      },
      {
        id: 'username',
        header: 'Username',
        priority: 2,
        cell: (c) => (
          <span className="text-muted-foreground">{c.username}</span>
        ),
      },
      {
        id: 'added',
        header: 'Added',
        priority: 3,
        cell: (c) => (
          <span className="text-muted-foreground">
            {formatDate(c.created_at)}
          </span>
        ),
      },
      {
        id: 'expiry',
        header: 'Expiry',
        priority: 2,
        cell: (c) => <ExpiryCell credential={c} />,
      },
    ],
    [],
  )

  if (credentials.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<PackageIcon className="size-5" />}
        title="No registry credentials"
        description="Add a username and password (or access token) to pull private images from a container registry, referenced by name from app.yaml's build.registryCredential field."
        action={action}
      />
    )
  }

  return (
    <ResponsiveTable
      rows={credentials}
      columns={columns}
      getRowId={(c) => c.id}
      ariaLabel={t('tableLabels.registryCredentials')}
      renderActions={(c) => (
        <span className="flex flex-wrap justify-end gap-2">
          <BrowseRegistryCredentialDialog credential={c} />
          <TestButton credential={c} />
          <DeleteRegistryCredentialDialog credential={c} />
        </span>
      )}
    />
  )
}
