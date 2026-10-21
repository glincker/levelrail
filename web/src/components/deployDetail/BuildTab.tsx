import {
  DownloadSimpleIcon,
  HourglassIcon,
  PackageIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useTranslation } from 'react-i18next'
import { buttonVariants } from '@/components/ui/button'
import { EffectiveConfigPanel } from '../EffectiveConfigPanel'
import { BuildLogHints } from '../BuildLogHints'
import { DeployStepFeed } from '../DeployStepFeed'
import { LogTerminal } from '../LogTerminal'
import type { LogLine } from '../../hooks/useLogStream'
import type { DeployStepEvent } from '../../hooks/useDeployStepStream'
import type { DeployAttempt } from '../../types/deployAttempt'
import type { GitSourceResource } from '../../types/gitSource'
import { deployLogDownloadURL } from '../../queries/deployLogs'
import { useDeployDetailT } from './useDeployDetailT'

function heightFor(count: number): string {
  if (count <= 12) return 'h-64'
  if (count <= 40) return 'h-96'
  return 'h-[60dvh] min-h-96 max-h-[48rem]'
}

function Fact({ label, value }: { label: string; value?: string }) {
  if (!value) return null
  return (
    <div className="min-w-0">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      <dd className="truncate font-mono text-sm text-foreground" title={value}>
        {value}
      </dd>
    </div>
  )
}

/** BuildTab pairs a source summary with the log, or an honest empty state. */
export function BuildTab({
  appName,
  deployId,
  attempt,
  gitSource,
  lines,
  steps,
  noBuildStep,
  finished,
  isPaused,
  pause,
  resume,
}: {
  appName: string
  deployId: string
  attempt: DeployAttempt
  gitSource?: GitSourceResource
  lines: LogLine[]
  steps: DeployStepEvent[]
  noBuildStep: boolean
  finished: boolean
  isPaused: boolean
  pause: () => void
  resume: () => void
}) {
  const { t } = useDeployDetailT()
  const { t: tc } = useTranslation('deployContexts')
  const builder = gitSource
    ? t(`build.builderValue.${gitSource.build_type}`)
    : undefined
  const hasSource =
    gitSource?.repo_url ||
    attempt.commit_sha ||
    attempt.branch ||
    attempt.detected_framework ||
    builder

  const emptyKind = noBuildStep
    ? 'noStep'
    : finished
      ? 'notCaptured'
      : 'waiting'

  return (
    <div className="space-y-3">
      {hasSource ? (
        <section
          aria-label={t('build.source')}
          className="rounded-lg border border-border p-4"
        >
          <dl className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
            <Fact label={t('build.repo')} value={gitSource?.repo_url} />
            <Fact
              label={t('build.branch')}
              value={attempt.branch || gitSource?.branch}
            />
            <Fact
              label={t('build.commit')}
              value={attempt.commit_sha?.slice(0, 12)}
            />
            <Fact label={t('build.builder')} value={builder} />
            <Fact
              label={t('build.framework')}
              value={attempt.detected_framework}
            />
            <Fact label={t('build.image')} value={attempt.image} />
          </dl>
        </section>
      ) : null}

      {steps.length > 0 ? (
        <details
          open={noBuildStep || !finished}
          className="rounded-lg border border-border px-3 py-2"
        >
          <summary className="cursor-pointer text-sm font-medium text-foreground">
            {t(noBuildStep ? 'build.traceSteps' : 'build.steps', {
              count: steps.length,
            })}
          </summary>
          <div className="pt-2">
            <DeployStepFeed steps={steps} />
          </div>
        </details>
      ) : null}

      {lines.length > 0 ? (
        <>
          <div className="flex justify-end">
            <a
              href={deployLogDownloadURL(appName, deployId)}
              download
              className={buttonVariants({ variant: 'outline', size: 'sm' })}
            >
              <DownloadSimpleIcon className="size-3.5" aria-hidden="true" />
              {t('build.download')}
            </a>
          </div>
          <BuildLogHints lines={lines} />
          <LogTerminal
            lines={lines}
            isPaused={isPaused}
            pause={pause}
            resume={resume}
            heightClassName={heightFor(lines.length)}
            isFinished={finished}
          />
        </>
      ) : steps.length > 0 ? null : (
        <div className="flex items-start gap-3 rounded-lg border border-dashed border-border p-4">
          {emptyKind === 'waiting' ? (
            <HourglassIcon
              className="mt-0.5 size-5 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
          ) : (
            <PackageIcon
              className="mt-0.5 size-5 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
          )}
          <div className="space-y-1">
            <p className="text-sm font-medium text-foreground">
              {t(`build.empty.${emptyKind}Title`)}
            </p>
            <p className="text-sm text-muted-foreground">
              {t(`build.empty.${emptyKind}Body`)}
            </p>
          </div>
        </div>
      )}
      <details className="rounded-lg border border-border p-3">
        <summary className="cursor-pointer text-sm font-medium text-foreground">
          {tc('effective.title')}
        </summary>
        <div className="pt-3">
          <EffectiveConfigPanel appName={appName} />
        </div>
      </details>
    </div>
  )
}
