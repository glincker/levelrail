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

// Icon, tone and the word all carry the state, so colour is never the only cue.
function StateChip({ state }: Readonly<{ state: ProviderCapabilityState }>) {
  const { t } = useTranslation('cloudProvider')
  const { Icon, tone } = STATE[state]
  return (
    <span
      className={cn(
        'inline-flex items-center gap-1.5 text-[13px] font-medium',
        tone,
      )}
    >
      <Icon className="size-4" aria-hidden="true" />
      {t(`capabilities.${state}`)}
    </span>
  )
}

function CapabilityRow({ cap }: Readonly<{ cap: ProviderCapability }>) {
  const { t } = useTranslation('cloudProvider')
  const label = t(`cap.${cap.id}` as 'cap.instances.read', {
    defaultValue: cap.id,
  })
  const used = cap.used_by?.length
    ? t('capabilities.usedBy', { features: cap.used_by.join(', ') })
    : ''
  return (
    <li className="flex min-h-(--glinr-row-h) flex-wrap items-center justify-between gap-x-3 gap-y-1 py-2">
      <div className="min-w-0">
        <p className="text-sm">{label}</p>
        <p className="text-xs text-muted-foreground">
          {cap.state === 'unknown' ? t('capabilities.unknownHint') : used}
        </p>
      </div>
      <div className="flex flex-wrap items-center justify-end gap-x-3 gap-y-1">
        {cap.needs ? (
          <code className="rounded bg-muted px-1.5 py-0.5 text-xs">
            {cap.state === 'missing'
              ? t('capabilities.needs', { scope: cap.needs })
              : cap.needs}
          </code>
        ) : null}
        <span className="sr-only">
          {t(
            `capabilities.evidence.${cap.evidence}` as 'capabilities.evidence.probe_ok',
            {
              defaultValue: cap.evidence,
            },
          )}
        </span>
        <StateChip state={cap.state} />
      </div>
    </li>
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
        <ul className="divide-y divide-border">
          {data.capabilities.map((c) => (
            <CapabilityRow key={c.id} cap={c} />
          ))}
        </ul>
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
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
        <h2 id="cp-capabilities-title" className="text-sm font-semibold">
          {t('capabilities.title')}
        </h2>
        {summary ? (
          <span className="text-xs text-muted-foreground">
            {t('capabilities.summary', summary)}
          </span>
        ) : null}
        {data && connected ? (
          <span className="text-xs text-muted-foreground">
            {t('capabilities.checkedAt')} <RelativeTime at={data.checked_at} />
            {data.cached ? ` (${t('capabilities.cachedNote')})` : ''}
          </span>
        ) : null}
        {connected && hasProbe ? (
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="ms-auto"
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
      <p className="text-xs text-muted-foreground">
        {t('capabilities.description', { name: labels.name })}
      </p>
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
