import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { CopyIcon, FileTextIcon } from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Field, FieldLabel } from '@/components/ui/field'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Switch } from '@/components/ui/switch'
import {
  ResponsiveTable,
  type ResponsiveColumn,
} from '@/components/ui/responsive-table'
import { toast } from '@/components/ui/toast'
import {
  backupCopyListQueryOptions,
  backupCopySettingsQueryOptions,
  useSetBackupCopySettings,
} from '../queries/backupCopies'
import type { BackupCopy, BackupCopyStatus } from '../types/backupCopy'
import type { BackupTarget } from '../types/backupTarget'

const STATUS_VARIANT: Record<
  BackupCopyStatus,
  'success' | 'warning' | 'destructive' | 'info'
> = {
  succeeded: 'success',
  copying: 'info',
  pending: 'warning',
  failed: 'destructive',
}

// Compact "1h 5m" style lag from seconds.
function formatLag(seconds: number): string {
  if (seconds < 60) return `${seconds}s`
  const m = Math.floor(seconds / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  return m % 60 === 0 ? `${h}h` : `${h}h ${m % 60}m`
}

export function BackupCopiesSection({ targets }: { targets: BackupTarget[] }) {
  const { t } = useTranslation('settings')
  const settings = useQuery(backupCopySettingsQueryOptions())
  const copies = useQuery(backupCopyListQueryOptions())
  const save = useSetBackupCopySettings()
  const [draft, setDraft] = useState<string | null>(null)

  const saved = settings.data
  const target = draft ?? saved?.target_id ?? ''
  const enabled = saved?.enabled ?? false

  const apply = (nextEnabled: boolean, targetId: string) => {
    save.mutate(
      { enabled: nextEnabled, target_id: targetId },
      {
        onSuccess: () => {
          setDraft(null)
          toast.add({ title: t('backupCopies.saved'), type: 'success' })
        },
        onError: (error) => {
          toast.add({
            title: t('backupCopies.saveFailed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  const columns: ResponsiveColumn<BackupCopy>[] = [
    {
      id: 'resource',
      primary: true,
      header: t('backupCopies.table.resource'),
      cell: (row) => (
        <span className="font-medium text-foreground">{row.resource}</span>
      ),
    },
    {
      id: 'status',
      header: t('backupCopies.table.status'),
      cell: (row) => (
        <span className="flex flex-wrap items-center gap-1.5">
          <Badge variant={STATUS_VARIANT[row.status]}>
            {t(`backupCopies.status.${row.status}`)}
          </Badge>
          {row.status === 'succeeded' ? (
            <span
              className="inline-flex items-center gap-1 text-xs text-muted-foreground"
              title={t('backupCopies.manifestTip')}
              data-testid="copy-manifest"
            >
              <FileTextIcon className="size-3.5" aria-hidden="true" />
              {t('backupCopies.manifest')}
            </span>
          ) : null}
        </span>
      ),
    },
    {
      id: 'lag',
      header: t('backupCopies.table.lag'),
      priority: 2,
      cell: (row) => (
        <span className="text-muted-foreground">
          {formatLag(row.lag_seconds)}
        </span>
      ),
    },
    {
      id: 'finished',
      header: t('backupCopies.table.finished'),
      priority: 3,
      cell: (row) => (
        <span className="text-muted-foreground">
          {new Date(row.backup_finished_at).toLocaleString()}
        </span>
      ),
    },
    {
      id: 'error',
      header: t('backupCopies.table.error'),
      priority: 3,
      cell: (row) => (
        <span className="text-xs break-all text-muted-foreground">
          {row.error ?? '-'}
        </span>
      ),
    },
  ]

  return (
    <section
      className="mt-8 space-y-4"
      aria-label={t('backupCopies.title')}
      data-testid="backup-copies"
    >
      <div className="flex items-center gap-2">
        <CopyIcon className="size-4 text-muted-foreground" aria-hidden="true" />
        <h2 className="text-base font-semibold">{t('backupCopies.title')}</h2>
      </div>
      <p className="text-sm text-muted-foreground">
        {t('backupCopies.description')}
      </p>
      <div className="flex flex-wrap items-end gap-4">
        <Field>
          <FieldLabel htmlFor="backup-copy-target">
            {t('backupCopies.destination')}
          </FieldLabel>
          <Select
            value={target}
            onValueChange={(v) => {
              setDraft(v)
            }}
          >
            <SelectTrigger id="backup-copy-target" className="min-w-56">
              <SelectValue placeholder={t('backupCopies.choose')} />
            </SelectTrigger>
            <SelectContent>
              {targets.map((tg) => (
                <SelectItem key={tg.id} value={tg.id}>
                  {tg.name}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </Field>
        <Button
          type="button"
          size="sm"
          disabled={save.isPending || target === ''}
          onClick={() => {
            apply(true, target)
          }}
        >
          {enabled && draft === null
            ? t('backupCopies.update')
            : t('backupCopies.enable')}
        </Button>
        {enabled ? (
          <label className="flex items-center gap-2 text-sm">
            <Switch
              checked={enabled}
              disabled={save.isPending}
              onCheckedChange={(on) => {
                apply(on, target)
              }}
              aria-label={t('backupCopies.toggle')}
            />
            {t('backupCopies.on', { name: saved?.target_name ?? '' })}
          </label>
        ) : null}
      </div>
      {(copies.data ?? []).length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('backupCopies.empty')}
        </p>
      ) : (
        <ResponsiveTable
          rows={copies.data ?? []}
          columns={columns}
          getRowId={(row) => row.backup_id}
          ariaLabel={t('backupCopies.table.label')}
        />
      )}
    </section>
  )
}
