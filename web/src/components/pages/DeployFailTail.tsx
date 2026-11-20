import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { pagesDeployLogQueryOptions } from '../../queries/pagesDeployLog'

/** Last log lines of a failed deploy with the phase that failed. Loaded lazily by RecentDeploys. */
export default function DeployFailTail({
  name,
  deployId,
  lines: count,
}: Readonly<{ name: string; deployId: string; lines: number }>) {
  const { t } = useTranslation('pagesSites')
  const { data } = useQuery(pagesDeployLogQueryOptions(name, deployId, count))
  const lines = data?.lines ?? []
  if (lines.length === 0) return null
  const step = lines[lines.length - 1]?.step
  return (
    <div className="mb-1.5 rounded-md bg-muted/40 p-2 font-mono text-xs">
      {step ? (
        <p className="mb-1 font-sans font-medium text-destructive">
          {t('logs.failedAt', {
            step: t(`logs.step.${step}`, { defaultValue: step }),
          })}
        </p>
      ) : null}
      {lines.map((l, i) => (
        <div
          key={`${l.at ?? ''}-${i}`}
          className="break-all whitespace-pre-wrap"
        >
          {l.line}
        </div>
      ))}
    </div>
  )
}
