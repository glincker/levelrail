import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { ArrowLeftRightGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { useEnvDiff } from '../queries/envDiff'
import { useAppListOptional } from '../queries/apps'
import type { AppDetail } from '../types/appDetail'

const PREVIEW = 'preview'

type RowStatus = 'added' | 'removed' | 'changed'

const STATUS_CLASS: Record<RowStatus, string> = {
  added: 'text-emerald-600 dark:text-emerald-400',
  removed: 'text-red-600 dark:text-red-400',
  changed: 'text-amber-600 dark:text-amber-400',
}

/** AppEnvCompareCard lists which env keys differ from another app or this app's preview overrides. The server returns keys only, never a value. */
export function AppEnvCompareCard({ app }: { app: AppDetail }) {
  const { t } = useTranslation('deploys')
  const [other, setOther] = useState('')
  const apps = useAppListOptional()
  const diff = useEnvDiff(app.name, other)
  const rows: { key: string; status: RowStatus }[] = diff.data
    ? [
        ...diff.data.added.map((key) => ({ key, status: 'added' as const })),
        ...diff.data.removed.map((key) => ({
          key,
          status: 'removed' as const,
        })),
        ...diff.data.changed.map((key) => ({
          key,
          status: 'changed' as const,
        })),
      ].sort((x, y) => x.key.localeCompare(y.key))
    : []

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ArrowLeftRightGlassIcon className="size-5 text-muted-foreground" />
          {t('envCompare.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <label className="flex items-center gap-2 text-xs text-muted-foreground">
          {t('envCompare.pick')}
          <select
            className="rounded-md border border-input bg-background px-2 py-1.5 text-sm text-foreground"
            value={other}
            onChange={(e) => setOther(e.target.value)}
          >
            <option value="">{t('envCompare.none')}</option>
            <option value={PREVIEW}>{t('envCompare.preview')}</option>
            {(apps.data ?? [])
              .filter((a) => a.name !== app.name)
              .map((a) => (
                <option key={a.name} value={a.name}>
                  {a.name}
                </option>
              ))}
          </select>
        </label>
        {diff.data ? (
          <div role="status">
            <table
              className="w-full table-fixed border-collapse text-left font-mono text-xs"
              aria-label={t('envCompare.tableLabel')}
            >
              <thead className="text-muted-foreground">
                <tr>
                  <th className="w-2/3 py-1 pr-2 font-medium">
                    {t('envCompare.colKey')}
                  </th>
                  <th className="w-1/3 py-1 font-medium">
                    {t('envCompare.colStatus')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {rows.map((r) => (
                  <tr key={r.key} className="border-t border-border align-top">
                    <th scope="row" className="break-all py-1 pr-2 font-normal">
                      {r.key}
                    </th>
                    <td className="py-1">
                      <span className={STATUS_CLASS[r.status]}>
                        {t(`envCompare.${r.status}`)}
                      </span>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            <p className="pt-2 text-xs text-muted-foreground">
              {t('envCompare.summary', { count: diff.data.same })}
            </p>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
