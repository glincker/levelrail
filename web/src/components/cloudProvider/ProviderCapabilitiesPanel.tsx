import { isProviderConnected } from '../../lib/providerConnectionState'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  CircleQuestionMarkGlassIcon,
  CircleXGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import { RelativeTime } from '../kit/RelativeTime'
import { SkeletonList } from '../kit/Skeleton'
import {
  capabilitySummary,
  type ProviderCapability,
  type ProviderCapabilityState,
} from '../../lib/providerCapabilities'
import {
  RateLimitedError,
  useProviderCapabilities,
  useRecheckProviderCapabilities,
} from '../../queries/providerCapabilities'
import type { SectionProps } from './sections'

const CAPABILITIES_ANCHOR = 'cp-capabilities'

const STATE = {
  granted: { Icon: CircleCheckGlassIcon, tone: 'text-tone-success' },
  missing: { Icon: CircleXGlassIcon, tone: 'text-tone-danger' },
  unknown: { Icon: CircleQuestionMarkGlassIcon, tone: 'text-muted-foreground' },
} as const satisfies Record<ProviderCapabilityState, unknown>

// Icon shape, tone and a screen reader word all carry the state, so colour is never the only cue.
function CapabilityChip({ cap }: Readonly<{ cap: ProviderCapability }>) {
  const { t } = useTranslation('cloudProvider')
  const { Icon, tone } = STATE[cap.state]
  const label = t(`cap.${cap.id}` as 'cap.instances.read', {
    defaultValue: cap.id,
  })
  return (
    <li
      data-state={cap.state}
      className="flex min-w-0 items-start gap-1.5 rounded-md border border-border bg-muted/30 px-2 py-1 text-xs"
    >
      <Icon
        className={cn('mt-px size-3.5 shrink-0', tone)}
        aria-hidden="true"
      />
      <span className="min-w-0">
        <span className="block break-words">{label}</span>
        <span className="sr-only">{t(`capabilities.${cap.state}`)}</span>
        {cap.state === 'missing' && cap.needs ? (
          <span className="block break-all text-tone-danger">
            {t('capabilities.needs', { scope: cap.needs })}
          </span>
        ) : null}
      </span>
    </li>
  )
}

const METER_TONE = {
  granted: 'bg-tone-success',
  missing: 'bg-tone-danger',
  unknown: 'bg-muted-foreground/40',
} as const satisfies Record<ProviderCapabilityState, string>

// Segments, not a width, so the meter needs no inline style.
function GrantMeter({
  caps,
  label,
}: Readonly<{ caps: ProviderCapability[]; label: string }>) {
  const order: ProviderCapabilityState[] = ['granted', 'missing', 'unknown']
  const sorted = order.flatMap((s) => caps.filter((c) => c.state === s))
  return (
    <div
      role="meter"
      aria-label={label}
      aria-valuemin={0}
      aria-valuemax={caps.length}
      aria-valuenow={caps.filter((c) => c.state === 'granted').length}
      className="flex h-1.5 gap-0.5"
    >
      {sorted.map((c) => (
        <span
          key={c.id}
          className={cn('h-full flex-1 rounded-full', METER_TONE[c.state])}
        />
      ))}
    </div>
  )
}

function CapabilityDetails({ caps }: Readonly<{ caps: ProviderCapability[] }>) {
  const { t } = useTranslation('cloudProvider')
  return (
    <details className="group text-xs">
      <summary className="cursor-pointer rounded-sm text-muted-foreground select-none hover:text-foreground focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring">
        {t('capabilities.details')}
      </summary>
      <ul className="mt-1.5 divide-y divide-border">
        {caps.map((cap) => {
          const label = t(`cap.${cap.id}` as 'cap.instances.read', {
            defaultValue: cap.id,
          })
          const used = cap.used_by?.length
            ? t('capabilities.usedBy', { features: cap.used_by.join(', ') })
            : ''
          return (
            <li key={cap.id} className="space-y-0.5 py-1.5">
              <p className="flex flex-wrap items-center gap-x-2">
                <span>{label}</span>
                {cap.needs ? (
                  <code className="rounded bg-muted px-1 py-0.5 break-all">
                    {cap.needs}
                  </code>
                ) : null}
              </p>
              <p className="text-muted-foreground">
                {cap.state === 'unknown' ? t('capabilities.unknownHint') : used}{' '}
                {t(
                  `capabilities.evidence.${cap.evidence}` as 'capabilities.evidence.probe_ok',
                  { defaultValue: cap.evidence },
                )}
              </p>
            </li>
          )
        })}
      </ul>
    </details>
  )
}

export function ProviderCapabilitiesPanel({
  provider,
  resource,
  labels,
}: SectionProps) {
  const { t } = useTranslation('cloudProvider')
  const connected = resource.has_token
  const caps = useProviderCapabilities(provider.id, connected)
  const recheck = useRecheckProviderCapabilities(provider.id)
  const data = recheck.data ?? caps.data
  const limited =
    recheck.error instanceof RateLimitedError ? recheck.error : null
  const otherError = recheck.isError && !limited ? recheck.error : caps.error
  const hasProbe = !!data && data.capabilities.length > 0
  const missing = data?.capabilities.some((c) => c.state === 'missing') ?? false

  // Data here reads the legacy credential, so an account-only provider has nothing to show.
  if (!connected && isProviderConnected(resource)) return null
  let body
  if (!connected) {
    body = (
      <Note
        title={t('capabilities.notConnectedTitle')}
        text={t('capabilities.notConnectedBody', {
          name: labels.name,
          servers: labels.servers,
        })}
      />
    )
  } else if (caps.isPending) {
    body = <SkeletonList rows={3} />
  } else if (!data) {
    body = (
      <Alert variant="destructive">
        <AlertDescription>
          {t('capabilities.error', { message: otherError?.message ?? '' })}
        </AlertDescription>
      </Alert>
    )
  } else if (!hasProbe) {
    body = (
      <Note
        title={t('capabilities.noProbeTitle', { name: labels.name })}
        text={t('capabilities.noProbeBody')}
      />
    )
  } else {
    body = (
      <>
        <ul className="grid grid-cols-[repeat(auto-fill,minmax(9.5rem,1fr))] gap-1.5">
          {data.capabilities.map((c) => (
            <CapabilityChip key={c.id} cap={c} />
          ))}
        </ul>
        <CapabilityDetails caps={data.capabilities} />
        {data.live_check_error ? (
          <p className="text-xs text-muted-foreground">
            {t('capabilities.liveError')}
          </p>
        ) : null}
        {missing ? (
          <p className="text-xs text-muted-foreground">
            {labels.tokenHelp}{' '}
            {provider.tokenUrl ? (
              <a
                href={provider.tokenUrl}
                target="_blank"
                rel="noopener noreferrer"
                className="underline underline-offset-2 hover:text-foreground"
              >
                {t('capabilities.howToFix')}
              </a>
            ) : null}
          </p>
        ) : null}
      </>
    )
  }

  const summary = hasProbe ? capabilitySummary(data) : null

  return (
    <section
      id={CAPABILITIES_ANCHOR}
      aria-labelledby="cp-capabilities-title"
      className="space-y-2 rounded-lg border border-border p-3"
    >
      <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
        <h2 id="cp-capabilities-title" className="text-sm font-semibold">
          {t('capabilities.title')}
        </h2>
        {summary ? (
          <span className="text-xs text-muted-foreground">
            {t('capabilities.summary', summary)}
          </span>
        ) : null}
      </div>
      {hasProbe && data ? (
        <GrantMeter
          caps={data.capabilities}
          label={t('capabilities.summary', summary ?? { granted: 0, total: 0 })}
        />
      ) : null}
      {data && connected ? (
        <div className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1">
          <span className="text-xs text-muted-foreground">
            {t('capabilities.checkedAt')} <RelativeTime at={data.checked_at} />
            {data.cached ? ` (${t('capabilities.cachedNote')})` : ''}
          </span>
          {hasProbe ? (
            <Button
              type="button"
              variant="outline"
              size="sm"
              disabled={recheck.isPending}
              onClick={() => recheck.mutate()}
            >
              <RefreshCwGlassIcon size="1.4em" aria-hidden="true" />
              {recheck.isPending
                ? t('capabilities.rechecking')
                : t('capabilities.recheck')}
            </Button>
          ) : null}
        </div>
      ) : null}
      {limited ? (
        <p role="status" className="text-xs text-tone-warning">
          {limited.retryAfterSeconds > 0
            ? t('capabilities.rateLimited', {
                seconds: limited.retryAfterSeconds,
              })
            : t('capabilities.rateLimitedNoWait')}
        </p>
      ) : recheck.isError ? (
        <p role="alert" className="text-xs text-tone-danger">
          {t('capabilities.error', { message: recheck.error.message })}
        </p>
      ) : null}
      {body}
    </section>
  )
}

function Note({ title, text }: Readonly<{ title: string; text: string }>) {
  return (
    <div className="rounded-md border border-dashed border-border p-3">
      <p className="text-sm font-medium">{title}</p>
      <p className="text-xs text-muted-foreground">{text}</p>
    </div>
  )
}
