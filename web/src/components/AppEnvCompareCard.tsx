import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { ArrowLeftRightGlassIcon } from '@thesvg/ui-react'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { diffAppEnv } from '../lib/appEnvDiff'
import { toRows, type EnvRowStatus } from '../lib/appEnvRows'
import { appDetailQueryOptions, useAppListOptional } from '../queries/apps'
import type { AppDetail } from '../types/appDetail'
import { useSecretKeys } from '../queries/secrets'

const STATUS_CLASS: Record<EnvRowStatus, string> = {
  added: 'text-emerald-600 dark:text-emerald-400',
  removed: 'text-red-600 dark:text-red-400',
  changed: 'text-amber-600 dark:text-amber-400',
}

function EnvCell({
  value,
  secret,
}: Readonly<{ value: string | undefined; secret: boolean }>) {
  const { t } = useTranslation('deploys')
  if (secret) {
    return (
      <span className="text-muted-foreground">
        <span aria-hidden="true">••••••</span>
        <span className="sr-only">{t('envCompare.secretMasked')}</span>
      </span>
    )
  }
  if (value === undefined) {
    return (
      <span className="text-muted-foreground">{t('envCompare.missing')}</span>
    )
  }
  return <>{value}</>
}

/** AppEnvCompareCard diffs this app's env against another app's side by side, comparing secrets by key only. */
export function AppEnvCompareCard({ app }: { app: AppDetail }) {
  const { t } = useTranslation('deploys')
  const [other, setOther] = useState('')
  const apps = useAppListOptional()
  const mine = useSecretKeys(app.name)
  const theirApp = useQuery({
    ...appDetailQueryOptions(other),
    enabled: other !== '',
  })
  const theirs = useSecretKeys(other)

  const ready =
    other !== '' &&
    theirApp.data &&
    mine.data &&
    (theirs.data || theirs.isError)
  const diff = ready
    ? diffAppEnv(
        {
          env: app.env ?? {},
          secretKeys: [
            ...(app.secret_env ?? []),
            ...(mine.data ?? []).map((k) => k.key),
          ],
        },
        {
          env: theirApp.data.env ?? {},
          secretKeys: [
            ...(theirApp.data.secret_env ?? []),
            ...(theirs.data ?? []).map((k) => k.key),
          ],
        },
      )
    : null
  const rows = diff ? toRows(diff) : []

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
            {(apps.data ?? [])
              .filter((a) => a.name !== app.name)
              .map((a) => (
                <option key={a.name} value={a.name}>
                  {a.name}
                </option>
              ))}
          </select>
        </label>
        {diff ? (
          <div role="status">
            <table
              className="w-full table-fixed border-collapse text-left font-mono text-xs"
              aria-label={t('envCompare.tableLabel')}
            >
              <thead className="text-muted-foreground">
                <tr>
                  <th className="w-1/4 py-1 pr-2 font-medium">
                    {t('envCompare.colKey')}
                  </th>
                  <th className="w-1/4 py-1 pr-2 font-medium">{app.name}</th>
                  <th className="w-1/4 py-1 pr-2 font-medium">{other}</th>
                  <th className="w-1/4 py-1 font-medium">
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
                    <td className="break-all py-1 pr-2">
                      <EnvCell value={r.a} secret={r.secret} />
                    </td>
                    <td className="break-all py-1 pr-2">
                      <EnvCell value={r.b} secret={r.secret} />
                    </td>
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
              {t('envCompare.summary', { count: diff.same })}
            </p>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
