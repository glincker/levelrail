import { PackageIcon } from '@phosphor-icons/react/dist/ssr'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import { EmptyState } from '@/components/ui/empty-state'
import { DeleteRegistryCredentialDialog } from './DeleteRegistryCredentialDialog'
import type { RegistryCredential } from '../types/registryCredential'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Only the credential's identity: registry host, username, connected
// date. No column for which services reference it (GET /api/v1/
// registry-credentials carries none of that; a service's own
// build.registryCredential field is where that link lives).
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
              <TableCell className="text-right">
                <DeleteRegistryCredentialDialog credential={cred} />
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
    </div>
  )
}
