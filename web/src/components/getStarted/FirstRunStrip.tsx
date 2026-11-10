import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowRightGlassIcon,
  CircleAlertGlassIcon,
  CircleCheckGlassIcon,
  CircleDashedGlassIcon,
  ExternalLinkGlassIcon,
  LoaderCircleGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Button, buttonVariants } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import type {
  FirstRunModel,
  FirstRunStep,
  FirstRunStepState,
} from '../../lib/firstRunSteps'

const LINK =
  'inline-flex items-center gap-1 pointer-coarse:min-h-10 text-xs font-medium text-foreground underline-offset-2 hover:underline'

function StateIcon({ state }: { state: FirstRunStepState }) {
  const base = 'size-5 shrink-0'
  if (state === 'done') {
    return (
      <CircleCheckGlassIcon
        aria-hidden
        className={cn(base, 'text-tone-success')}
      />
    )
  }
  if (state === 'working') {
    return (
      <LoaderCircleGlassIcon
        aria-hidden
        className={cn(base, 'text-primary motion-safe:animate-spin')}
      />
    )
  }
  if (state === 'failed') {
    return (
      <CircleAlertGlassIcon
        aria-hidden
        className={cn(base, 'text-destructive')}
      />
    )
  }
  if (state === 'attention') {
    return (
      <TriangleAlertGlassIcon
        aria-hidden
        className={cn(base, 'text-tone-warning')}
      />
    )
  }
  return (
    <CircleDashedGlassIcon
      aria-hidden
      className={cn(base, 'text-muted-foreground')}
    />
  )
}

function StepItem({
  index,
  step,
  children,
}: {
  index: number
  step: FirstRunStep
  children?: ReactNode
}) {
  const { t } = useTranslation('getStarted')
  return (
    <li
      data-step={step.id}
      data-state={step.state}
      className="flex min-w-0 flex-1 items-start gap-2 rounded-lg border border-border bg-card p-3"
    >
      <StateIcon state={step.state} />
      <div className="min-w-0 flex-1 space-y-0.5">
        <p className="text-sm font-medium text-foreground">
          {index}. {t(`strip.steps.${step.id}.title`)}
        </p>
        <p className="line-clamp-2 text-xs text-muted-foreground">
          {step.detail || t(`strip.steps.${step.id}.${step.state}`)}
        </p>
        {children}
      </div>
    </li>
  )
}

/** FirstRunStrip is the one install-to-live path: three steps until an app runs, then one line with its link. */
export function FirstRunStrip({
  model,
  onDeploy,
  deploying,
  error,
  onContinue,
}: {
  model: FirstRunModel
  onDeploy: () => void
  deploying: boolean
  error?: string
  onContinue?: () => void
}) {
  const { t } = useTranslation('getStarted')

  if (model.live) {
    return (
      <section
        aria-label={t('strip.title')}
        className="flex flex-wrap items-center gap-x-3 gap-y-2 rounded-lg border border-border bg-card p-3"
      >
        <CircleCheckGlassIcon
          aria-hidden
          className="size-5 text-tone-success"
        />
        <p className="text-sm font-medium text-foreground">{t('strip.live')}</p>
        {model.address ? (
          <a
            data-primary
            href={model.address}
            target="_blank"
            rel="noreferrer"
            className={buttonVariants({ size: 'sm' })}
          >
            {t('strip.open')}
            <ExternalLinkGlassIcon aria-hidden className="size-4.5" />
            <span className="sr-only">{model.address}</span>
          </a>
        ) : null}
        {model.address ? (
          <span className="min-w-0 truncate text-xs text-muted-foreground">
            {model.address}
          </span>
        ) : null}
        {model.hasDomain || !model.appName ? null : (
          <Link
            to="/apps/$name/domains"
            params={{ name: model.appName }}
            className={cn(LINK, 'ml-auto')}
          >
            {t('strip.addDomain')}
          </Link>
        )}
        {onContinue ? (
          <Button size="sm" variant="ghost" onClick={onContinue}>
            {t('strip.continue')}
          </Button>
        ) : null}
      </section>
    )
  }

  const deployBusy = deploying || model.steps[2]?.state === 'working'
  const primaryLabel =
    model.primary === 'server'
      ? t('strip.review')
      : deploying
        ? t('strip.deploying')
        : t('strip.deploy')

  return (
    <section aria-label={t('strip.title')} className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <h2 className="text-base font-semibold text-foreground">
          {t('strip.title')}
        </h2>
        {model.primary === 'server' ? (
          <Link
            data-primary
            to="/inbox"
            className={buttonVariants({ size: 'sm' })}
          >
            {primaryLabel}
          </Link>
        ) : (
          <Button
            data-primary
            size="sm"
            onClick={onDeploy}
            disabled={deployBusy}
          >
            {primaryLabel}
          </Button>
        )}
      </div>
      <ol className="flex flex-col gap-2 md:flex-row">
        {model.steps.map((step, i) => (
          <StepItem key={step.id} index={i + 1} step={step}>
            {step.id === 'server' &&
            step.state !== 'done' &&
            step.state !== 'todo' ? (
              <Link to="/inbox" className={LINK}>
                {t('strip.review')}
                <ArrowRightGlassIcon aria-hidden className="size-4" />
              </Link>
            ) : null}
            {step.id === 'deploy' && step.state === 'todo' ? (
              <Link to="/apps/new" className={LINK}>
                {t('strip.gitUrl')}
                <ArrowRightGlassIcon aria-hidden className="size-4" />
              </Link>
            ) : null}
            {step.id === 'deploy' &&
            step.state === 'failed' &&
            model.appName ? (
              <Link
                to="/apps/$name"
                params={{ name: model.appName }}
                className={LINK}
              >
                {t('strip.viewApp')}
                <ArrowRightGlassIcon aria-hidden className="size-4" />
              </Link>
            ) : null}
          </StepItem>
        ))}
      </ol>
      {error ? (
        <p role="alert" className="text-xs text-destructive">
          {error}
        </p>
      ) : null}
    </section>
  )
}
