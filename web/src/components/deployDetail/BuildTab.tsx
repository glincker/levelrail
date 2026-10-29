import {
  CountdownGlassIcon,
  DownloadGlassIcon,
  PackageGlassIcon,
} from '@thesvg/ui-react'
import { useCallback, useMemo, useState } from 'react'
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
import { parseBuildCacheStats } from '../../lib/buildCacheStats'
import { analyzeBuildLog, type BuildStep } from '../../lib/buildLogPhases'
import { BuildPhaseChips, type PhaseFilter } from './BuildPhaseChips'
import { BuildStepList } from './BuildStepList'
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
  const cacheStats = useMemo(() => parseBuildCacheStats(lines), [lines])
  const analysis = useMemo(() => analyzeBuildLog(lines), [lines])
  const [phase, setPhase] = useState<PhaseFilter>('all')
  const [scrollTarget, setScrollTarget] = useState<{ id: number } | null>(null)
  const shownLines = useMemo(
    () =>
      phase === 'all'
        ? lines
        : lines.filter((_, i) => analysis.linePhases[i] === phase),
    [lines, analysis, phase],
  )
  const jumpToStep = useCallback(
    (step: BuildStep) => {
      const target = lines[step.firstLine]
      if (!target) return
      if (phase !== 'all' && analysis.linePhases[step.firstLine] !== phase) {
        setPhase('all')
      }
      setScrollTarget({ id: target.id })
    },
    [lines, analysis, phase],
  )
  const detected = [attempt.detected_framework, builder]
    .filter(Boolean)
    .join(', ')
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
          {detected ? (
            <p className="mb-3 text-sm text-foreground">
              {t('build.detected', { value: detected })}
              {builder ? (
                <span className="text-muted-foreground">
                  {`. ${t('build.detectedBuilder')}`}
                </span>
              ) : null}
            </p>
          ) : null}
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
          <div className="flex flex-wrap items-center justify-between gap-2">
            <p
              className="text-sm text-muted-foreground"
              data-testid="build-cache"
            >
              {cacheStats
                ? t('cache.summary', {
                    cached: cacheStats.cached,
                    total: cacheStats.total,
                  })
                : t('cache.unknown')}
            </p>
            <a
              href={deployLogDownloadURL(appName, deployId)}
              download
              className={buttonVariants({ variant: 'outline', size: 'sm' })}
            >
              <DownloadGlassIcon className="size-4" aria-hidden="true" />
              {t('build.download')}
            </a>
          </div>
          <BuildLogHints lines={lines} />
          <BuildPhaseChips
            analysis={analysis}
            active={phase}
            onSelect={setPhase}
          />
          <BuildStepList analysis={analysis} onJump={jumpToStep} />
          <LogTerminal
            lines={shownLines}
            scrollTarget={scrollTarget}
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
            <CountdownGlassIcon
              className="mt-0.5 size-5.5 shrink-0 text-muted-foreground"
              aria-hidden="true"
            />
          ) : (
            <PackageGlassIcon
              className="mt-0.5 size-5.5 shrink-0 text-muted-foreground"
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
