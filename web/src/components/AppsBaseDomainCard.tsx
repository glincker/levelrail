import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CheckGlassIcon,
  CloudGlassIcon,
  CopyGlassIcon,
  GlobeGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import {
  ingressSettingsQueryOptions,
  useUpdateIngressSettings,
} from '../queries/domains'
import { useCopyToClipboard } from '../hooks/useCopyToClipboard'
import { ApiError } from '../lib/apiError'
import { listedZoneNames, ROUTE53_IAM_ACTIONS } from '../lib/zoneSuggestions'
import {
  dnsZoneQueryOptions,
  useBackfillBaseDomain,
  useBaseDomainWildcard,
  useDomainAutomation,
  type BaseDomainWildcard,
} from '../queries/goLive'

const BASE_DOMAIN_PATTERN =
  /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$/
const ZONE_HINT_DELAY_MS = 400

function useDebounced(value: string, delay: number): string {
  const [debounced, setDebounced] = useState(value)
  useEffect(() => {
    const id = setTimeout(() => {
      setDebounced(value)
    }, delay)
    return () => {
      clearTimeout(id)
    }
  }, [value, delay])
  return debounced
}

export function ZoneHint({
  domain,
  onPick,
}: {
  domain: string
  onPick?: (zone: string) => void
}) {
  const { t } = useTranslation('domains')
  const { t: tp } = useTranslation('dnsProviders')
  const zone = useQuery({
    ...dnsZoneQueryOptions(`app.${domain}`),
    enabled: BASE_DOMAIN_PATTERN.test(domain),
  })
  if (!BASE_DOMAIN_PATTERN.test(domain)) {
    return domain === '' ? null : (
      <p className="text-xs text-destructive">{t('baseDomain.invalid')}</p>
    )
  }
  if (zone.isPending) {
    return (
      <p className="text-xs text-muted-foreground">
        {t('baseDomain.zoneChecking')}
      </p>
    )
  }
  if (zone.data?.found) {
    return (
      <p className="text-xs text-emerald-700 dark:text-emerald-400">
        {t('baseDomain.zoneMatched', {
          zone: zone.data.zone,
          provider: tp(`providers.${zone.data.provider}`, {
            defaultValue: zone.data.provider,
          }),
        })}
      </p>
    )
  }
  const message = zone.data?.message ?? zone.error?.message
  const suggestions = listedZoneNames(message, zone.data?.zones)
  return (
    <div className="space-y-1.5" role="alert">
      <p className="text-xs text-destructive">
        {message ?? t('baseDomain.zoneNotFound')}
      </p>
      {onPick ? <ZoneSuggestions names={suggestions} onPick={onPick} /> : null}
      {zone.data?.provider === 'route53' ||
      zone.data?.provider === undefined ? (
        <IamHint />
      ) : null}
    </div>
  )
}

export function ZoneSuggestions({
  names,
  onPick,
}: {
  names: string[]
  onPick: (zone: string) => void
}) {
  const { t } = useTranslation('domains')
  if (names.length === 0) return null
  return (
    <div className="flex flex-wrap items-center gap-1.5">
      <span className="text-xs text-muted-foreground">
        {t('baseDomain.suggestions')}
      </span>
      {names.map((name) => (
        <Button
          key={name}
          type="button"
          variant="outline"
          size="sm"
          className="h-6 px-2 font-mono text-xs"
          onClick={() => {
            onPick(name)
          }}
        >
          {name}
        </Button>
      ))}
    </div>
  )
}

function IamHint() {
  const { t } = useTranslation('domains')
  const { copy, copied } = useCopyToClipboard()
  const actions = ROUTE53_IAM_ACTIONS.join(', ')
  return (
    <p className="flex flex-wrap items-center gap-1.5 text-xs text-muted-foreground">
      {t('baseDomain.iamHint', { actions })}
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className="h-6 px-2"
        aria-label={t('baseDomain.copyIam')}
        onClick={() => {
          copy(ROUTE53_IAM_ACTIONS.join('\n'))
        }}
      >
        {copied ? (
          <CheckGlassIcon className="size-3.5" aria-hidden="true" />
        ) : (
          <CopyGlassIcon className="size-3.5" aria-hidden="true" />
        )}
        {copied ? t('baseDomain.copied') : t('baseDomain.copy')}
      </Button>
    </p>
  )
}

// "No DNS provider yet" empty state with the one next action.
export function NoProviderEmptyState() {
  const { t } = useTranslation('domains')
  return (
    <Alert>
      <CloudGlassIcon className="size-5" aria-hidden="true" />
      <AlertDescription>
        {t('baseDomain.noProvider')}{' '}
        <Link to="/domains" className="underline">
          {t('baseDomain.noProviderAction')}
        </Link>
      </AlertDescription>
    </Alert>
  )
}

// The apps base domain: new apps without a domain get <app>.<base> and
// their DNS record is created for them.
export function AppsBaseDomainCard() {
  const { t } = useTranslation('domains')
  const { data: settings } = useQuery(ingressSettingsQueryOptions())
  const automation = useDomainAutomation()
  const update = useUpdateIngressSettings({ inlineError: true })
  const backfill = useBackfillBaseDomain()
  const wildcard = useBaseDomainWildcard()
  const [value, setValue] = useState(settings?.apps_base_domain ?? '')
  const debounced = useDebounced(value.trim().toLowerCase(), ZONE_HINT_DELAY_MS)
  if (!settings) return null

  const providerConnected = (automation.data?.dns_provider ?? 'none') !== 'none'
  const current = settings.apps_base_domain ?? ''
  const dirty = value.trim().toLowerCase() !== current

  function save() {
    if (!settings) return
    update.mutate(
      { ...settings, apps_base_domain: value.trim().toLowerCase() },
      {
        onSuccess: () => {
          toast.add({ title: t('baseDomain.saved'), type: 'success' })
        },
      },
    )
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <GlobeGlassIcon className="size-5" />
          {t('baseDomain.title')}
        </CardTitle>
        <CardDescription>{t('baseDomain.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-3">
        {!providerConnected && automation.isSuccess ? (
          <NoProviderEmptyState />
        ) : null}
        <div className="space-y-1.5">
          <Label htmlFor="apps-base-domain">{t('baseDomain.label')}</Label>
          <div className="flex gap-2">
            <Input
              id="apps-base-domain"
              className="font-mono"
              value={value}
              placeholder="apps.example.com"
              disabled={update.isPending}
              aria-busy={update.isPending}
              onChange={(e) => {
                setValue(e.target.value)
              }}
            />
            <Button
              type="button"
              disabled={!dirty || update.isPending}
              onClick={save}
            >
              {t('baseDomain.save')}
            </Button>
          </div>
          {dirty || current !== '' ? (
            <ZoneHint domain={dirty ? debounced : current} onPick={setValue} />
          ) : null}
          {update.isError ? (
            <div className="space-y-1.5" role="alert">
              <p className="text-xs text-destructive">{update.error.message}</p>
              <ZoneSuggestions
                names={listedZoneNames(
                  update.error.message,
                  update.error instanceof ApiError
                    ? update.error.zones
                    : undefined,
                )}
                onPick={setValue}
              />
            </div>
          ) : null}
        </div>
        {current !== '' ? (
          <div className="space-y-2 border-t border-border pt-3">
            <p className="text-xs text-muted-foreground">
              {t('baseDomain.backfillHelp', { base: current })}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={backfill.isPending}
                onClick={() => {
                  backfill.mutate(false)
                }}
              >
                {t('baseDomain.backfillPreview')}
              </Button>
              {backfill.data?.dry_run && backfill.data.items.length > 0 ? (
                <Button
                  type="button"
                  size="sm"
                  disabled={backfill.isPending}
                  onClick={() => {
                    backfill.mutate(true)
                  }}
                >
                  {t('baseDomain.backfillApply', {
                    count: backfill.data.items.length,
                  })}
                </Button>
              ) : null}
            </div>
            {backfill.data ? (
              <ul className="space-y-0.5 text-xs">
                {backfill.data.items.length === 0 ? (
                  <li className="text-muted-foreground">
                    {t('baseDomain.backfillNone')}
                  </li>
                ) : (
                  backfill.data.items.map((item) => (
                    <li key={item.app} className="font-mono">
                      {item.app} {'->'} {item.domain}{' '}
                      <span className="text-muted-foreground">
                        {item.status}
                      </span>
                    </li>
                  ))
                )}
              </ul>
            ) : null}
          </div>
        ) : null}
        {current !== '' && providerConnected ? (
          <div className="space-y-2 border-t border-border pt-3">
            <p className="text-xs text-muted-foreground">
              {t('baseDomain.wildcardHelp', { base: current })}
            </p>
            <div className="flex flex-wrap gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                disabled={wildcard.isPending}
                onClick={() => {
                  wildcard.mutate({ confirm: false, replace: false })
                }}
              >
                {t('baseDomain.wildcardPreview')}
              </Button>
              {wildcard.data?.dry_run &&
              wildcard.data.dns.planned !== 'unchanged' &&
              wildcard.data.dns.record ? (
                <Button
                  type="button"
                  size="sm"
                  disabled={wildcard.isPending}
                  onClick={() => {
                    wildcard.mutate({ confirm: true, replace: false })
                  }}
                >
                  {t('baseDomain.wildcardApply')}
                </Button>
              ) : null}
              {wildcard.data?.dns.planned === 'conflict' ? (
                <Button
                  type="button"
                  size="sm"
                  variant="destructive"
                  disabled={wildcard.isPending}
                  onClick={() => {
                    wildcard.mutate({ confirm: true, replace: true })
                  }}
                >
                  {t('baseDomain.wildcardReplace')}
                </Button>
              ) : null}
            </div>
            {wildcard.isError ? (
              <p className="text-xs text-destructive" role="alert">
                {wildcard.error.message}
              </p>
            ) : null}
            {wildcard.data ? <WildcardResult data={wildcard.data} /> : null}
          </div>
        ) : null}
      </CardContent>
    </Card>
  )
}

function WildcardResult({ data }: { data: BaseDomainWildcard }) {
  const { t } = useTranslation('domains')
  const { record, replaced, message, planned } = data.dns
  return (
    <div className="space-y-0.5 text-xs" aria-live="polite">
      {record ? (
        <p className="font-mono">
          {t('baseDomain.wildcardRecord', {
            type: record.type,
            name: data.dns.domain,
            value: record.value,
          })}
        </p>
      ) : null}
      {(replaced ?? []).map((old) => (
        <p key={`${old.type}-${old.value}`} className="font-mono">
          {t('baseDomain.wildcardExisting', {
            type: old.type,
            name: old.name,
            value: old.value,
          })}
        </p>
      ))}
      {planned === 'conflict' ? (
        <p className="text-destructive">{t('baseDomain.wildcardConflict')}</p>
      ) : null}
      {message ? <p className="text-muted-foreground">{message}</p> : null}
    </div>
  )
}
