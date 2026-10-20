import type { ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CloudArrowUpIcon,
  PlugsConnectedIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { EmptyState } from '@/components/ui/empty-state'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import { DeleteBackupTargetDialog } from './DeleteBackupTargetDialog'
import { PROVIDER_LABEL } from './backupTargetProvider'
import { BrandTile } from './BrandLogoBadge'
import { backupTargetBrand } from '../lib/providerBrands'
import {
  DumpEncryptionBadge,
  EnableDumpEncryptionButton,
} from './BackupTargetEncryption'
import { useTestBackupTarget } from '../queries/backupTargets'
import type { BackupTarget } from '../types/backupTarget'

function formatDate(iso: string): string {
  return new Date(iso).toLocaleString()
}

// Mirrors RegistryCredentialTable.tsx's own TestButton exactly: same
// loading/success/failure states, same toast pattern, for the
// identical "verify a stored credential by actually using it, once, on
// demand" action against a different resource.
function TestButton({ target }: { target: BackupTarget }) {
  const { t } = useTranslation('settings')
  const testTarget = useTestBackupTarget()
  return (
    <Button
      type="button"
      variant="outline"
      size="sm"
      disabled={testTarget.isPending}
      onClick={() => {
        testTarget.mutate(target.id, {
          onSuccess: () => {
            toast.add({
              title: t('backupTargetsPage.table.testOk', {
                name: target.name,
              }),
              type: 'success',
            })
          },
          onError: (error) => {
            toast.add({
              title: t('backupTargetsPage.table.testFailed', {
                name: target.name,
              }),
              description: error.message,
              type: 'error',
            })
          },
        })
      }}
    >
      <PlugsConnectedIcon className="size-3.5" aria-hidden="true" />
      {testTarget.isPending
        ? t('backupTargetsPage.table.testing')
        : t('backupTargetsPage.table.test')}
    </Button>
  )
}

// Only the destination itself: bucket, provider, region/endpoint,
// connected date. No column for backup runs or last-backup status,
// since GET /api/v1/backup-targets carries none of that (nothing writes
// backup history yet, see queries/backupTargets.ts's own header
// comment), rather than a column that would just be permanently empty.
export function BackupTargetTable({
  targets,
  action,
}: {
  targets: BackupTarget[]
  action?: ReactNode
}) {
  const { t } = useTranslation('settings')
  if (targets.length === 0) {
    return (
      <EmptyState
        className="gap-2 border-0 bg-transparent py-4"
        icon={<CloudArrowUpIcon className="size-5" />}
        title={t('backupTargetsPage.empty.title')}
        description={t('backupTargetsPage.empty.body')}
        action={action}
      />
    )
  }

  const columns: ResponsiveColumn<BackupTarget>[] = [
    {
      id: 'name',
      primary: true,
      header: t('backupTargetsPage.table.name'),
      cell: (target) => (
        <span className="font-medium text-foreground">{target.name}</span>
      ),
    },
    {
      id: 'provider',
      header: t('backupTargetsPage.table.provider'),
      priority: 2,
      cell: (target) => (
        <Badge variant="outline" className="gap-1.5">
          <BrandTile
            name={backupTargetBrand(target.provider, target.endpoint)}
            label={PROVIDER_LABEL[target.provider]}
            size="xs"
          />
          {PROVIDER_LABEL[target.provider]}
        </Badge>
      ),
    },
    {
      id: 'bucket',
      header: t('backupTargetsPage.table.bucket'),
      cell: (target) => (
        <span className="text-muted-foreground">{target.bucket}</span>
      ),
    },
    {
      id: 'encryption',
      header: t('backupTargetsPage.table.encryption'),
      priority: 2,
      cell: (target) => <DumpEncryptionBadge target={target} />,
    },
    {
      id: 'location',
      header: t('backupTargetsPage.table.region'),
      priority: 3,
      cell: (target) =>
        target.region || target.endpoint ? (
          <div className="flex flex-col gap-0.5 text-muted-foreground">
            {target.region ? <span>{target.region}</span> : null}
            {target.endpoint ? (
              <span className="text-xs break-all">{target.endpoint}</span>
            ) : null}
          </div>
        ) : (
          <span className="text-muted-foreground">-</span>
        ),
    },
    {
      id: 'connected',
      header: t('backupTargetsPage.table.connected'),
      priority: 3,
      cell: (target) => (
        <span className="text-muted-foreground">
          {formatDate(target.created_at)}
        </span>
      ),
    },
  ]

  return (
    <ResponsiveTable
      rows={targets}
      columns={columns}
      getRowId={(target) => target.id}
      ariaLabel={t('backupTargetsPage.table.label')}
      renderActions={(target) => (
        <div className="flex flex-wrap items-center justify-end gap-2">
          <EnableDumpEncryptionButton target={target} />
          <TestButton target={target} />
          <DeleteBackupTargetDialog target={target} />
        </div>
      )}
    />
  )
}
