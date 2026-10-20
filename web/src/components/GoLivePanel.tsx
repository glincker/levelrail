import type { ReactNode } from 'react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ArrowSquareOutIcon,
  CheckCircleIcon,
  CloudIcon,
  HandPointingIcon,
  MinusCircleIcon,
  SpinnerGapIcon,
  WarningCircleIcon,
} from '@phosphor-icons/react/dist/ssr'
import { Badge } from '@/components/ui/badge'
import { buttonVariants } from '@/components/ui/button'
import {
  useGoLiveStatus,
  type DomainDnsResult,
  type GoLiveResult,
  type GoLiveStep,
} from '../queries/goLive'

function StepIcon({ state }: { state: GoLiveStep['state'] }) {
  const cls = 'mt-0.5 size-4 shrink-0'
  switch (state) {
    case 'done':
      return (
        <CheckCircleIcon
          className={`${cls} text-emerald-600`}
          aria-hidden="true"
        />
      )
    case 'pending':
      return (
        <SpinnerGapIcon
          className={`${cls} text-muted-foreground motion-safe:animate-spin`}
          aria-hidden="true"
        />
      )
    case 'failed':
    case 'conflict':
      return (
        <WarningCircleIcon
          className={`${cls} text-destructive`}
          aria-hidden="true"
        />
      )
    case 'manual':
      return (
        <HandPointingIcon
          className={`${cls} text-amber-600`}
          aria-hidden="true"
        />
      )
    default:
      return (
        <MinusCircleIcon
          className={`${cls} text-muted-foreground`}
          aria-hidden="true"
        />
      )
  }
}

// Plain-language line for a step, from the structured fields so it can be
// translated; the server's own detail is the fallback.
function useStepText() {
  const { t } = useTranslation('domains')
  return (step: GoLiveStep): string => {
    if (step.id === 'dns' && step.state === 'done' && step.provider) {
      return t('goLive.text.dnsDone', { provider: step.provider })
    }
    if (step.id === 'propagation' && step.state === 'pending') {
      const seen = step.resolvers?.find((r) => (r.addresses?.length ?? 0) > 0)
      return seen
        ? t('goLive.text.propagatingSeen', { resolver: seen.name })
        : t('goLive.text.propagatingWaiting')
    }
    if (step.id === 'certificate' && step.state === 'done' && step.issuer) {
      return t('goLive.text.certDone', {
        issuer: step.issuer,
        date: step.not_after
          ? new Date(step.not_after).toLocaleDateString()
          : '',
      })
    }
    if (step.state === 'skipped' && step.detail) {
      return t('goLive.text.skipped', { reason: step.detail })
    }
    return step.detail ?? ''
  }
}

function ConnectProviderPrompt() {
  const { t } = useTranslation('domains')
  return (
    <p className="text-xs text-muted-foreground">
      <CloudIcon className="mr-1 inline size-3.5" aria-hidden="true" />
      {t('goLive.connectCloudflare')}{' '}
      <Link to="/domains" className="underline">
        {t('goLive.connectCloudflareAction')}
      </Link>
    </p>
  )
}

export function GoLiveSteps({
  result,
  dns,
}: {
  result: GoLiveResult
  dns?: DomainDnsResult
}) {
  const { t } = useTranslation('domains')
  const stepText = useStepText()
  const dnsStep = result.steps.find((s) => s.id === 'dns')
  const noProvider = dnsStep?.state === 'manual' && !dnsStep.provider
  const liveUrl = result.url
  return (
    <div className="space-y-2">
      <ul className="space-y-1.5" aria-live="polite">
        {result.steps.map((step) => (
          <li key={step.id} className="flex items-start gap-2 text-sm">
            <StepIcon state={step.state} />
            <div className="min-w-0 flex-1">
              <p className="font-medium">{t(`goLive.steps.${step.id}`)}</p>
              {stepText(step) ? (
                <p className="break-words text-xs text-muted-foreground">
                  {stepText(step)}
                </p>
              ) : null}
              {step.id === 'dns' && step.record ? (
                <p className="font-mono text-xs text-muted-foreground">
                  {step.record.type} {step.record.name} {step.record.value}
                </p>
              ) : null}
            </div>
            {step.id === 'dns' && step.provider === 'cloudflare' ? (
              <Badge variant="muted">{t('goLive.cloudflareBadge')}</Badge>
            ) : null}
          </li>
        ))}
      </ul>
      {dns?.proxied ? (
        <p className="text-xs text-amber-700 dark:text-amber-400">
          {t('goLive.proxiedWarning')}
        </p>
      ) : null}
      {dns?.dns === 'conflict' ? (
        <p className="text-xs text-destructive">{t('goLive.conflictHint')}</p>
      ) : null}
      {noProvider ? <ConnectProviderPrompt /> : null}
      {result.state === 'live' && liveUrl ? (
        <a
          href={liveUrl}
          target="_blank"
          rel="noopener noreferrer"
          className={buttonVariants({ size: 'sm', variant: 'outline' })}
        >
          <ArrowSquareOutIcon />
          {t('goLive.openLive')}
        </a>
      ) : null}
    </div>
  )
}

// Live progress for one domain: polls the go-live status until it settles.
export function GoLivePanel({
  app,
  domain,
  initial,
  dns,
}: {
  app: string
  domain: string
  initial?: GoLiveResult
  dns?: DomainDnsResult
}): ReactNode {
  const { t } = useTranslation('domains')
  const status = useGoLiveStatus(app, domain)
  const result = status.data ?? initial
  if (!result || !Array.isArray(result.steps)) {
    return (
      <p className="text-xs text-muted-foreground">{t('goLive.loading')}</p>
    )
  }
  return <GoLiveSteps result={result} dns={dns} />
}
