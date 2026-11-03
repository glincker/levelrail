import { useId, useState, type ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  ChevronDownGlassIcon,
  CopyGlassIcon,
  OctagonAlertGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button, buttonVariants } from '@/components/ui/button'
import { failureLinesToText } from '../../lib/observabilityFormat'
import { useFailureContext } from '../../queries/investigate'
import { useStuck, type StuckAction } from '../../queries/stuck'
import type { DeployAttempt } from '../../types/deployAttempt'
import { ActionButton, CauseCard } from '../StuckPanel'
import { ContainerExitBanner } from '../ContainerExitBanner'

function Section({ title, children }: { title: string; children: ReactNode }) {
  const [open, setOpen] = useState(false)
  const id = useId()
  return (
    <div className="mt-1">
      <Button
        type="button"
        variant="ghost"
        size="xs"
        aria-expanded={open}
        aria-controls={id}
        onClick={() => {
          setOpen(!open)
        }}
      >
        <ChevronDownGlassIcon
          className={`size-4 ${open ? 'rotate-180' : ''}`}
          aria-hidden="true"
        />
        {title}
      </Button>
      {open ? <div id={id}>{children}</div> : null}
    </div>
  )
}

/** One card for every "why is this not running" signal: top cause and fix first, detail folded away. */
export function DiagnosisCard({
  appName,
  latest,
}: {
  appName: string
  latest?: DeployAttempt
}) {
  const { t } = useTranslation('fixit')
  const stuckQuery = useStuck('app', appName)
  const stuck = stuckQuery.data
  const failure = useFailureContext(appName).data
  const [copied, setCopied] = useState(false)

  const failing = failure !== undefined && failure.state !== 'healthy'
  const stuckShown = stuck !== undefined && !stuck.ready
  if (!failing && !stuckShown) return null

  const crash = failure?.state === 'crashlooping'
  const causes = stuck?.causes ?? []
  const top = causes[0]
  const others = causes.slice(1)
  const lines = failure?.lines ?? []
  const lastLine = [...lines].reverse().find((l) => l.message.trim() !== '')
  const deployId =
    failure?.deploy?.id ?? (latest?.status === 'failed' ? latest.id : undefined)
  const topActions = (top?.actions ?? []).filter((a) => a.type !== 'recheck')
  const hasRetry = topActions.some((a) => a.type === 'retry_deploy')
  const retryAction: StuckAction = {
    type: 'retry_deploy',
    label: t('stuck.retryDeploy'),
  }
  const invalidateKey = ['stuck', 'app', appName] as const
  const minutes = Math.max(1, Math.round((failure?.window_seconds ?? 60) / 60))
  const headline = top
    ? top.title
    : crash
      ? t('diagnosis.crashloop')
      : failing
        ? t('diagnosis.deployFailed')
        : t('stuck.notReadyYet')

  return (
    <section
      aria-label={t('diagnosis.title')}
      data-testid="diagnosis-card"
      className="rounded-xl border border-tone-danger-border bg-card px-4 py-3"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-xs font-medium text-muted-foreground">
            {t('diagnosis.title')}
          </p>
          <h2 className="flex items-center gap-1.5 text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
            <OctagonAlertGlassIcon
              className="size-5 shrink-0 text-tone-danger"
              aria-hidden="true"
            />
            {headline}
            {top ? (
              <Badge
                variant={top.confidence === 'high' ? 'destructive' : 'warning'}
              >
                {top.confidence === 'high'
                  ? t('stuck.confidenceHigh')
                  : t('stuck.confidenceMedium')}
              </Badge>
            ) : null}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {top
              ? top.explanation
              : crash && failure
                ? t('diagnosis.crashSummary', {
                    count: failure.restarts_in_window,
                    minutes,
                  })
                : failing && failure?.deploy?.error
                  ? failure.deploy.error
                  : t('diagnosis.noCause')}
          </p>
          {top && top.evidence.length > 0 ? (
            <ul className="mt-1 space-y-0.5" aria-label={t('stuck.evidence')}>
              {top.evidence.map((e) => (
                <li
                  key={e}
                  className="truncate font-mono text-xs text-muted-foreground"
                  title={e}
                >
                  {e}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
        <div className="flex flex-wrap items-center gap-2">
          {topActions.map((a) => (
            <ActionButton
              key={`${a.type}-${a.fix_id ?? a.href ?? ''}`}
              action={a}
              appName={appName}
              onRecheck={() => void stuckQuery.refetch()}
              rechecking={stuckQuery.isFetching}
              invalidateKey={invalidateKey}
            />
          ))}
          {!hasRetry && failing && !crash ? (
            <ActionButton
              action={retryAction}
              appName={appName}
              onRecheck={() => void stuckQuery.refetch()}
              rechecking={stuckQuery.isFetching}
              invalidateKey={invalidateKey}
            />
          ) : null}
          <Link
            to="/apps/$name/logs"
            params={{ name: appName }}
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            {t('diagnosis.openLogs')}
          </Link>
          {deployId ? (
            <Link
              to="/apps/$name/deploys/$deployId/logs"
              params={{ name: appName, deployId }}
              className={buttonVariants({ variant: 'outline', size: 'sm' })}
            >
              {t('diagnosis.openDeployLogs')}
            </Link>
          ) : null}
          {stuck ? (
            <Button
              type="button"
              size="sm"
              variant="outline"
              disabled={stuckQuery.isFetching}
              onClick={() => void stuckQuery.refetch()}
            >
              {t('stuck.recheck')}
            </Button>
          ) : null}
        </div>
      </div>
      {lastLine ? (
        <p className="mt-2 truncate font-mono text-xs text-foreground">
          <span className="mr-2 font-sans font-medium text-tone-danger">
            {t('diagnosis.lastLine')}
          </span>
          {lastLine.message}
        </p>
      ) : null}
      {failing && failure ? (
        <Section title={t('diagnosis.sectionLog')}>
          <div className="mt-2">
            {failure.exit ? (
              <div className="mb-3">
                <ContainerExitBanner
                  exitCode={failure.exit.exit_code}
                  oomKilled={failure.exit.oom_killed}
                  finishedAt={
                    failure.exit.finished_at ?? failure.exit.captured_at
                  }
                  message={failure.exit.message}
                  reason={failure.exit.reason}
                  showHint
                />
              </div>
            ) : null}
            {lines.length === 0 ? (
              <p className="text-sm text-muted-foreground">
                {t('diagnosis.noLines')}
              </p>
            ) : (
              <>
                <div className="flex flex-wrap items-center justify-between gap-2">
                  <p className="text-xs text-muted-foreground">
                    {t('diagnosis.showing', {
                      shown: lines.length,
                      total: failure.total_lines,
                    })}
                  </p>
                  <Button
                    type="button"
                    variant="outline"
                    size="xs"
                    onClick={() => {
                      void navigator.clipboard
                        .writeText(failureLinesToText(lines))
                        .then(() => {
                          setCopied(true)
                        })
                    }}
                  >
                    {copied ? (
                      <CheckGlassIcon className="size-4" aria-hidden="true" />
                    ) : (
                      <CopyGlassIcon className="size-4" aria-hidden="true" />
                    )}
                    {copied ? t('diagnosis.copied') : t('diagnosis.copy')}
                  </Button>
                </div>
                <pre
                  tabIndex={0}
                  aria-label={t('diagnosis.linesLabel')}
                  className="mt-1 max-h-72 overflow-auto rounded-lg border border-neutral-800 bg-neutral-950 p-3 font-mono text-xs leading-5 text-neutral-200"
                >
                  {lines.map((l, i) => (
                    <span
                      key={`${l.timestamp}-${i}`}
                      className={`block whitespace-pre-wrap ${
                        l.stream === 'stderr' ? 'text-red-400' : ''
                      }`}
                    >
                      {l.message}
                    </span>
                  ))}
                </pre>
              </>
            )}
          </div>
        </Section>
      ) : null}
      {stuck && stuck.timeline.length > 0 ? (
        <Section title={t('diagnosis.sectionTimeline')}>
          <ul className="mt-1 space-y-1">
            {stuck.timeline.map((c) => (
              <li
                key={`${c.type}-${c.since}`}
                className="font-mono text-xs text-muted-foreground"
              >
                {new Date(c.since).toLocaleString()} {c.type}={c.status} (
                {c.reason})
              </li>
            ))}
          </ul>
        </Section>
      ) : null}
      {others.length > 0 ? (
        <Section title={t('diagnosis.sectionOthers', { count: others.length })}>
          <div className="mt-2 space-y-2">
            {others.map((cause) => (
              <CauseCard
                key={cause.code}
                cause={cause}
                appName={appName}
                onRecheck={() => void stuckQuery.refetch()}
                rechecking={stuckQuery.isFetching}
                invalidateKey={invalidateKey}
              />
            ))}
          </div>
        </Section>
      ) : null}
    </section>
  )
}
