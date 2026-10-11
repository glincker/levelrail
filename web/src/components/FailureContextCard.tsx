import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckIcon,
  CopyIcon,
  WarningOctagonIcon,
} from '@phosphor-icons/react/dist/ssr'
import { useFailureContext } from '../queries/investigate'
import { failureLinesToText } from '../lib/observabilityFormat'
import type { FailureContext } from '../types/investigate'
import { Button, buttonVariants } from './ui/button'

function FailureBody({
  appName,
  ctx,
}: {
  appName: string
  ctx: FailureContext
}) {
  const { t } = useTranslation('observability')
  const [copied, setCopied] = useState(false)
  const crash = ctx.state === 'crashlooping'
  const minutes = Math.max(1, Math.round(ctx.window_seconds / 60))

  return (
    <section
      aria-label={t('failure.title')}
      className="rounded-xl border border-tone-danger-border bg-card p-4"
    >
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="flex items-center gap-1.5 text-sm font-semibold text-foreground">
            <WarningOctagonIcon
              className="size-4 text-tone-danger"
              aria-hidden="true"
            />
            {crash
              ? t('failure.crashloopTitle')
              : t('failure.deployFailedTitle')}
          </h2>
          <p className="mt-1 text-sm text-muted-foreground">
            {crash
              ? t('failure.crashloopSummary', {
                  count: ctx.restarts_in_window,
                  minutes,
                })
              : t('failure.deploySummary', {
                  image: ctx.deploy?.image ?? '',
                })}
          </p>
          {!crash && ctx.deploy?.error ? (
            <p className="mt-1 font-mono text-xs break-words text-tone-danger">
              {ctx.deploy.error}
            </p>
          ) : null}
        </div>
        <div className="flex items-center gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            onClick={() => {
              void navigator.clipboard
                .writeText(failureLinesToText(ctx.lines))
                .then(() => {
                  setCopied(true)
                })
            }}
          >
            {copied ? (
              <CheckIcon className="size-3.5" aria-hidden="true" />
            ) : (
              <CopyIcon className="size-3.5" aria-hidden="true" />
            )}
            {copied ? t('logs.copied') : t('failure.copy')}
          </Button>
          {!crash && ctx.deploy ? (
            <Link
              to="/apps/$name/deploys/$deployId/logs"
              params={{ name: appName, deployId: ctx.deploy.id }}
              className={buttonVariants({ variant: 'outline', size: 'sm' })}
            >
              {t('failure.openDeployLogs')}
            </Link>
          ) : null}
          <Link
            to="/apps/$name/logs"
            params={{ name: appName }}
            className={buttonVariants({ variant: 'outline', size: 'sm' })}
          >
            {t('failure.openLogs')}
          </Link>
        </div>
      </div>
      {ctx.lines.length === 0 ? (
        <p className="mt-3 text-sm text-muted-foreground">
          {t('failure.noLines')}
        </p>
      ) : (
        <>
          <p className="mt-3 text-xs text-muted-foreground">
            {t('failure.showing', {
              shown: ctx.lines.length,
              total: ctx.total_lines,
            })}
          </p>
          <pre
            tabIndex={0}
            aria-label={t('failure.linesLabel')}
            className="mt-1 max-h-72 overflow-auto rounded-lg border border-neutral-800 bg-neutral-950 p-3 font-mono text-xs leading-5 text-neutral-200"
          >
            {ctx.lines.map((l, i) => (
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
    </section>
  )
}

// Shows the last log lines of the failing container when the app is
// crashlooping or its latest deploy failed; renders nothing otherwise.
export function FailureContextCard({ appName }: { appName: string }) {
  const { data } = useFailureContext(appName)
  if (!data || data.state === 'healthy') {
    return null
  }
  return <FailureBody appName={appName} ctx={data} />
}
