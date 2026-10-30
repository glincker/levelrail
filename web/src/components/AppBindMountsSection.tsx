import { useTranslation } from 'react-i18next'
import { FolderGlassIcon, TriangleAlertGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import type { AppBindMount } from '../types/appDetail'

const COLUMNS: ResponsiveColumn<AppBindMount>[] = [
  {
    id: 'host',
    header: 'Host path',
    primary: true,
    cell: (mount) => (
      <span className="inline-flex items-center gap-1.5 font-mono text-xs text-foreground">
        <FolderGlassIcon
          className="size-5 shrink-0 text-muted-foreground"
          aria-hidden="true"
        />
        <span className="min-w-0 break-all">{mount.host_path}</span>
      </span>
    ),
  },
  {
    id: 'container',
    header: 'Container path',
    cell: (mount) => (
      <span className="font-mono text-xs break-all text-muted-foreground">
        {mount.container_path}
      </span>
    ),
  },
  {
    id: 'access',
    header: 'Access',
    priority: 2,
    cell: (mount) => (
      <Badge variant={mount.read_only ? 'outline' : 'muted'}>
        {mount.read_only ? 'Read-only' : 'Read/write'}
      </Badge>
    ),
  },
]

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
  const { t } = useTranslation('common')
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
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>
            Each row below gives this app&rsquo;s container direct read
            {bindMounts.some((m) => !m.read_only) ? '/write ' : ' '}
            access to a real directory on the host machine it runs on, outside
            Docker&rsquo;s own volume management. Change which directories are
            mounted by redeploying this app&rsquo;s compose file.
          </AlertDescription>
        </Alert>
        <ResponsiveTable
          rows={bindMounts}
          columns={COLUMNS}
          getRowId={(m) => m.host_path + m.container_path}
          ariaLabel={t('tableLabels.bindMounts')}
        />
      </CardContent>
    </Card>
  )
}
