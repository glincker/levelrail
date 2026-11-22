import { useTranslation } from 'react-i18next'
import { useRestorePreview } from '../queries/restorePreview'
import { formatAge, formatBytes } from '../lib/format'

// Shown inside the restore confirm dialog so the operator sees what gets
// replaced, and what the fallback is, before typing the database name.
export function RestorePreviewPanel({
  databaseName,
  backupId,
}: {
  databaseName: string
  backupId: string
}) {
  const { t } = useTranslation('restorePreview')
  const preview = useRestorePreview(databaseName, backupId, true)

  if (preview.isPending) {
    return <p className="text-sm text-muted-foreground">{t('loading')}</p>
  }
  if (preview.isError) {
    return (
      <p className="text-sm text-destructive">
        {t('loadFailed', { error: preview.error.message })}
      </p>
    )
  }
  const p = preview.data
  const rows: Array<[string, string]> = [
    [t('size'), formatBytes(p.size_bytes)],
    [t('age'), formatAge(p.taken_at)],
    [t('target'), p.target_name ?? p.target_id],
    [t('overwrites'), p.overwrites],
    [
      t('safety'),
      p.safety_backup_id
        ? t('safetyNamed', {
            id: p.safety_backup_id,
            age: formatAge(p.safety_taken_at),
          })
        : t('safetyNone'),
    ],
  ]
  return (
    <div className="rounded-md border border-border p-3 text-sm">
      <p className="mb-2 font-medium">{t('title')}</p>
      <dl className="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
        {rows.map(([label, value]) => (
          <div key={label} className="contents">
            <dt className="text-muted-foreground">{label}</dt>
            <dd className="break-words">{value}</dd>
          </div>
        ))}
      </dl>
      {p.encrypted ? (
        <p className="mt-2 text-muted-foreground">{t('encrypted')}</p>
      ) : null}
      {p.busy ? <p className="mt-2 text-tone-warning">{t('busy')}</p> : null}
    </div>
  )
}
