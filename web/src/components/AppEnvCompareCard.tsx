import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import { ArrowsLeftRightIcon } from '@phosphor-icons/react/dist/ssr'
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card'
import { diffAppEnv } from '../lib/appEnvDiff'
import { appDetailQueryOptions, useAppListOptional } from '../queries/apps'
import type { AppDetail } from '../types/appDetail'
import { useSecretKeys } from '../queries/secrets'

/** AppEnvCompareCard diffs this app's env against another app's, comparing secrets by key only. */
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

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ArrowsLeftRightIcon className="size-4 text-muted-foreground" />
          {t('envCompare.title')}
        </CardTitle>
      </CardHeader>
      <CardContent className="space-y-3">
        <label className="block space-y-1 text-xs text-muted-foreground">
          {t('envCompare.pick')}
          <select
            className="block w-full rounded-md border border-input bg-background px-2 py-1.5 text-sm text-foreground"
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
          <div className="space-y-2 font-mono text-xs" role="status">
            {diff.onlyA.map((e) => (
              <p key={`a-${e.key}`}>
                {t('envCompare.onlyIn', { app: app.name })} {e.key}
                {e.secret ? ` ${t('envCompare.secret')}` : `=${e.a ?? ''}`}
              </p>
            ))}
            {diff.onlyB.map((e) => (
              <p key={`b-${e.key}`}>
                {t('envCompare.onlyIn', { app: other })} {e.key}
                {e.secret ? ` ${t('envCompare.secret')}` : `=${e.b ?? ''}`}
              </p>
            ))}
            {diff.differ.map((e) => (
              <p key={`d-${e.key}`}>
                {t('envCompare.differs')} {e.key}
                {e.secret
                  ? ` ${t('envCompare.secretOneSide')}`
                  : `: ${e.a ?? ''} -> ${e.b ?? ''}`}
              </p>
            ))}
            <p className="text-muted-foreground">
              {t('envCompare.summary', { count: diff.same })}
            </p>
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}
