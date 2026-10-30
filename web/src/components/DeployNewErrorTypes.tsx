import { useMemo } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { SparklesGlassIcon } from '@thesvg/ui-react'
import { useLogSignatures } from '../queries/logExplorer'

// "3 new error types since this deploy": error signatures that appeared after
// the deploy started and were absent in the window just before it.
export function DeployNewErrorTypes({
  appName,
  startedAt,
}: {
  appName: string
  startedAt: string
}) {
  const { t } = useTranslation('observability')
  const range = useMemo(
    () => ({ from: new Date(startedAt), to: new Date() }),
    [startedAt],
  )
  const { data } = useLogSignatures(appName, {
    ...range,
    baseline: true,
    limit: 1,
  })
  if (!data?.has_baseline || data.new_count === 0) {
    return null
  }
  return (
    <p className="mt-3 flex flex-wrap items-center gap-2 rounded-lg border border-border px-3 py-2 text-sm">
      <SparklesGlassIcon
        className="size-5 text-tone-warning"
        aria-hidden="true"
      />
      <span>{t('explorer.newErrorTypes', { count: data.new_count })}</span>
      <Link
        to="/apps/$name/logs"
        params={{ name: appName }}
        search={{ tab: 'errors' }}
        className="underline"
      >
        {t('explorer.viewLines')}
      </Link>
    </p>
  )
}
