import { useTranslation } from 'react-i18next'
import { useDeployDetail } from '../queries/deployDetail'
import type { ContainerExit } from '../types/containerExit'
import { ContainerExitBanner } from './ContainerExitBanner'

export function DeployExitBody({ exit }: { exit: ContainerExit }) {
  const { t } = useTranslation('observability')
  return (
    <section
      aria-label={t('containerExit.deployTitle')}
      className="space-y-3 rounded-xl border border-tone-danger-border bg-card p-4"
    >
      <div>
        <h2 className="text-(length:--glinr-type-size-section) font-(weight:--glinr-type-heading-weight) text-foreground">
          {t('containerExit.deployTitle')}
        </h2>
        <p className="mt-1 text-sm text-muted-foreground">
          {t('containerExit.deploySummary', { container: exit.container })}
        </p>
      </div>
      <ContainerExitBanner
        exitCode={exit.exit_code}
        oomKilled={exit.oom_killed}
        finishedAt={exit.finished_at ?? exit.captured_at}
        message={exit.message}
        reason={exit.reason}
        showHint
      />
      {exit.lines.length === 0 ? (
        <p className="text-sm text-muted-foreground">
          {t('containerExit.noLines')}
        </p>
      ) : (
        <>
          <p className="text-xs text-muted-foreground">
            {t('containerExit.showing', { count: exit.lines.length })}
          </p>
          <pre
            tabIndex={0}
            aria-label={t('containerExit.linesLabel')}
            className="max-h-72 overflow-auto rounded-lg border border-neutral-800 bg-neutral-950 p-3 font-mono text-xs leading-5 text-neutral-200"
          >
            {exit.lines.map((l, i) => (
              <span
                key={`${l.timestamp}-${i}`}
                className={`block whitespace-pre-wrap ${
                  l.stream === 'stderr' ? 'text-tone-danger' : ''
                }`}
              >
                {l.message}
              </span>
            ))}
          </pre>
        </>
      )}
    </section>
  )
}

// The captured exit of a failed deploy's container; renders nothing for a
// deploy that did not fail that way.
export function DeployExitCard({
  appName,
  deployId,
  failed,
}: {
  appName: string
  deployId: string
  failed: boolean
}) {
  const { data } = useDeployDetail(appName, deployId, failed)
  if (!failed || !data?.exit) {
    return null
  }
  return <DeployExitBody exit={data.exit} />
}
