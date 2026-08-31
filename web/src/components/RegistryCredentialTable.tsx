import {
  PackageIcon,
  PlugsConnectedIcon,
} from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import { toast } from '@/components/ui/toast'
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
}: {
  credentials: RegistryCredential[]
}) {
  if (credentials.length === 0) {
    return (
      <EmptyState
        className="py-12"
        icon={<PackageIcon className="size-5" />}
        title="No registry credentials"
        description="Add a username and password (or access token) to pull private images from a container registry, referenced by name from app.yaml's build.registryCredential field."
      />
    )
  }

  return (
    <div className="rounded-lg border border-border">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>Name</TableHead>
            <TableHead>Registry host</TableHead>
            <TableHead>Username</TableHead>
            <TableHead>Added</TableHead>
            <TableHead>Expiry</TableHead>
            <TableHead className="text-right">Actions</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {credentials.map((cred) => (
            <TableRow key={cred.id}>
              <TableCell className="font-medium text-foreground">
                {cred.name}
              </TableCell>
              <TableCell className="font-mono text-muted-foreground">
                {cred.registry_host}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {cred.username}
              </TableCell>
              <TableCell className="text-muted-foreground">
                {formatDate(cred.created_at)}
              </TableCell>
              <TableCell>
                <ExpiryCell credential={cred} />
              </TableCell>
              <TableCell className="text-right">
                <div className="flex justify-end gap-2">
                  <TestButton credential={cred} />
                  <DeleteRegistryCredentialDialog credential={cred} />
                </div>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
