import { useEffect } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { FieldSectionLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { repoEnvHintOptions } from '../queries/repoEnvHints'
import {
  mergeEnvHintRows,
  missingEnvRows,
  type EnvHintRow,
} from '../lib/createAppFromGit'

// Reads the repo's example env file once a repo and ref are picked and
// shows each required variable as an empty field. The parent blocks the
// deploy button while any stays empty.
export function GitRepoEnvFields({
  repoUrl,
  gitRef,
  baseDirectory,
  token,
  rows,
  onRowsChange,
  disabled,
}: {
  repoUrl: string
  gitRef: string
  baseDirectory: string
  token?: string
  rows: EnvHintRow[]
  onRowsChange: (rows: EnvHintRow[]) => void
  disabled: boolean
}) {
  const { t } = useTranslation('appSetup')
  const enabled = repoUrl !== '' && gitRef !== ''
  const query = useQuery(
    repoEnvHintOptions(
      {
        repo_url: repoUrl,
        ref: gitRef,
        base_directory: baseDirectory || undefined,
        token: token || undefined,
      },
      enabled,
    ),
  )
  const hints = query.data
  useEffect(() => {
    if (!hints) return
    const next = mergeEnvHintRows(rows, hints)
    if (next !== rows) onRowsChange(next)
    // rows is read only to keep typed values; re-running on its own change
    // would loop.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hints])

  if (!enabled) return null
  if (query.isPending) {
    return (
      <p className="text-muted-foreground text-xs" role="status">
        {t('envHints.scanning')}
      </p>
    )
  }
  if (rows.length === 0) return null
  const missing = missingEnvRows(rows)
  return (
    <div className="space-y-3" data-testid="git-env-hints">
      <FieldSectionLabel>{t('envHints.title')}</FieldSectionLabel>
      <p className="text-muted-foreground text-xs">{t('envHints.help')}</p>
      {rows.map((row, i) => (
        <div key={row.name} className="flex items-center gap-2">
          <code className="w-2/5 truncate font-mono text-xs">{row.name}</code>
          <Input
            aria-label={t('envHints.valueLabel', { name: row.name })}
            type={row.secret ? 'password' : 'text'}
            autoComplete="off"
            required
            disabled={disabled}
            value={row.value}
            onChange={(e) =>
              onRowsChange(
                rows.map((r, j) =>
                  j === i ? { ...r, value: e.target.value } : r,
                ),
              )
            }
          />
        </div>
      ))}
      {missing.length > 0 ? (
        <p className="text-destructive text-xs" role="status">
          {t('envHints.missing', { count: missing.length })}
        </p>
      ) : null}
    </div>
  )
}
