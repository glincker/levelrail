import { useEffect, useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  CloudIcon,
  GlobeHemisphereWestIcon,
} from '@phosphor-icons/react/dist/ssr'
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
import {
  dnsZoneQueryOptions,
  useBackfillBaseDomain,
  useDomainAutomation,
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

export function ZoneHint({ domain }: { domain: string }) {
  const { t } = useTranslation('domains')
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
        {t('baseDomain.zoneFound', {
          zone: zone.data.zone,
          provider: zone.data.provider,
        })}
      </p>
    )
  }
  return (
    <p className="text-xs text-destructive">{t('baseDomain.zoneNotFound')}</p>
  )
}

// "No DNS provider yet" empty state with the one next action.
export function NoProviderEmptyState() {
  const { t } = useTranslation('domains')
  return (
    <Alert>
      <CloudIcon className="size-4" aria-hidden="true" />
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
  const update = useUpdateIngressSettings()
  const backfill = useBackfillBaseDomain()
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
          <GlobeHemisphereWestIcon className="size-4" />
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
          {dirty ? <ZoneHint domain={debounced} /> : null}
          {update.isError ? (
            <p className="text-xs text-destructive">{update.error.message}</p>
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
      </CardContent>
    </Card>
  )
}
