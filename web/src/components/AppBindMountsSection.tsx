import { FolderOpenIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from '@/components/ui/table'
import type { AppBindMount } from '../types/appDetail'

// Read-only listing of an app's bind-mounted host directories
// (store.ServiceBindMount), the AppVolumeBackupsSection counterpart for
// bind mounts: unlike a named Docker volume, a bind mount has no backup
// mechanism through this platform (it's already a real path on the
// host, an operator's own backup tooling applies to it directly), so
// this only ever shows what's mounted, it never triggers anything.
// Bind mounts are only ever set through the compose-import path
// (CreateComposeFields' own bind-mount helper), root-ability-gated
// server-side (internal/api/apps_compose.go's handleDeployCompose), not
// editable here.
export function AppBindMountsSection({
  bindMounts,
}: {
  bindMounts: AppBindMount[] | undefined
}) {
  if (!bindMounts || bindMounts.length === 0) {
    return null
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Bind mounts</CardTitle>
      </CardHeader>
      <CardContent className="space-y-4">
        <Alert>
          <WarningIcon />
          <AlertDescription>
            Each row below gives this app&rsquo;s container direct read
            {bindMounts.some((m) => !m.read_only) ? '/write ' : ' '}
            access to a real directory on the host machine it runs on,
            outside Docker&rsquo;s own volume management. Change which
            directories are mounted by redeploying this app&rsquo;s
            compose file.
          </AlertDescription>
        </Alert>
        <div className="rounded-lg border border-border">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Host path</TableHead>
                <TableHead>Container path</TableHead>
                <TableHead>Access</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {bindMounts.map((mount) => (
                <TableRow key={mount.host_path + mount.container_path}>
                  <TableCell className="font-mono text-xs text-foreground">
                    <span className="inline-flex items-center gap-1.5">
                      <FolderOpenIcon
                        className="size-3.5 text-muted-foreground"
                        aria-hidden="true"
                      />
                      {mount.host_path}
                    </span>
                  </TableCell>
                  <TableCell className="font-mono text-xs text-muted-foreground">
                    {mount.container_path}
                  </TableCell>
                  <TableCell>
                    <Badge variant={mount.read_only ? 'outline' : 'muted'}>
                      {mount.read_only ? 'Read-only' : 'Read/write'}
                    </Badge>
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      </CardContent>
    </Card>
  )
}
