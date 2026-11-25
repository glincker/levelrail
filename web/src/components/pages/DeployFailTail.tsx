import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { pagesDeployLogQueryOptions } from '../../queries/pagesDeployLog'
import DeployFailHint from './DeployFailHint'

/** Last log lines of a failed deploy with the phase that failed and a fix hint. Loaded lazily by RecentDeploys. */
export default function DeployFailTail({
  name,
  deployId,
  lines: count,
  scan,
}: Readonly<{
  name: string
  deployId: string
  lines: number
  /** How many trailing lines the hint matcher reads; at least the shown count. */
  scan: number
}>) {
  const { t } = useTranslation('pagesSites')
  const { data } = useQuery(
    pagesDeployLogQueryOptions(name, deployId, Math.max(scan, count)),
  )
  const all = data?.lines ?? []
  if (all.length === 0) return null
  const lines = all.slice(-count)
  const step = all[all.length - 1]?.step
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
      <DeployFailHint lines={all.map((l) => l.line)} />
    </div>
  )
}
