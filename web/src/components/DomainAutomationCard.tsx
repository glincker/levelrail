import { useState } from 'react'
import { Link } from '@tanstack/react-router'
import { useQuery } from '@tanstack/react-query'
import { useTranslation } from 'react-i18next'
import { RotateCcwGlassIcon, ZapGlassIcon } from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
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
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { appListQueryOptions } from '../queries/apps'
import { usePlugins } from '../queries/plugins'
import { useSetDnsProviderPlugin } from '../queries/pluginDns'
import {
  useAutomationRuns,
  useDomainAutomation,
  useGoLivePlan,
  useUndoAutomationRun,
  useUpdateDomainAutomation,
  type GoLivePolicy,
  type GoLiveResult,
} from '../queries/goLive'

type BoolKey =
  | 'auto_dns'
  | 'auto_proxy_route'
  | 'force_https'
  | 'attach_www_counterpart'
  | 'wildcard_for_base_domain'
  | 'verify_after'

const TOGGLES: BoolKey[] = [
  'auto_dns',
  'auto_proxy_route',
  'force_https',
  'attach_www_counterpart',
  'wildcard_for_base_domain',
  'verify_after',
]

const WWW_POLICIES: GoLivePolicy['www_policy'][] = [
  'off',
  'redirect_to_apex',
  'redirect_to_www',
]

function PlanPreview({ plan }: { plan: GoLiveResult }) {
  const { t } = useTranslation('domains')
  return (
    <ol className="space-y-1 text-xs" aria-label={t('automation.previewList')}>
      {plan.steps.map((s) => (
        <li key={s.id} className="flex gap-2">
          <span className="w-24 shrink-0 font-medium">
            {t(`goLive.steps.${s.id}`)}
          </span>
          <span className="text-muted-foreground">
            {s.detail ?? t(`goLive.state.${s.state}`)}
          </span>
        </li>
      ))}
    </ol>
  )
}

function PolicyPreview({ policy }: { policy: GoLivePolicy }) {
  const { t } = useTranslation('domains')
  const { data: apps = [] } = useQuery(appListQueryOptions())
  const [example, setExample] = useState('example.com')
  const app = apps[0]?.name ?? ''
  const plan = useGoLivePlan(app)
  const domain = example.trim().toLowerCase()
  return (
    <div className="space-y-2">
      <Label htmlFor="automation-preview-domain">
        {t('automation.previewLabel')}
      </Label>
      <div className="flex gap-2">
        <Input
          id="automation-preview-domain"
          className="font-mono"
          value={example}
          onChange={(e) => {
            setExample(e.target.value)
          }}
        />
        <Button
          type="button"
          variant="outline"
          disabled={app === '' || domain === '' || plan.isPending}
          onClick={() => {
            plan.mutate({ domain, automation: policy })
          }}
        >
          {t('automation.previewRun')}
        </Button>
      </div>
      {app === '' ? (
        <p className="text-xs text-muted-foreground">
          {t('automation.previewNeedsApp')}
        </p>
      ) : null}
      {plan.isError ? (
        <p className="text-xs text-destructive">{plan.error.message}</p>
      ) : null}
      {plan.data?.plans[0] ? (
        <>
          <p className="text-xs font-medium">
            {t('automation.previewTitle', { domain })}
          </p>
          <PlanPreview plan={plan.data.plans[0]} />
        </>
      ) : null}
    </div>
  )
}

function History() {
  const { t } = useTranslation('domains')
  const runs = useAutomationRuns(10)
  const undo = useUndoAutomationRun()
  return (
    <div className="space-y-2">
      <div className="flex items-center justify-between">
        <h4 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
          {t('automation.historyTitle')}
        </h4>
        <Link
          to="/settings/audit-log"
          className="text-xs text-muted-foreground underline"
        >
          {t('automation.auditLink')}
        </Link>
      </div>
      {runs.data?.length === 0 ? (
        <p className="text-xs text-muted-foreground">
          {t('automation.historyEmpty')}
        </p>
      ) : null}
      <ul className="divide-y divide-border">
        {runs.data?.map((run) => (
          <li key={run.id} className="flex items-center gap-2 py-1.5 text-xs">
            <span className="min-w-0 flex-1 truncate font-mono">
              {run.domain}
            </span>
            <span className="hidden text-muted-foreground sm:inline">
              {run.steps
                .filter((s) => s.state === 'done')
                .map((s) => t(`goLive.steps.${s.id}`))
                .join(', ')}
            </span>
            <Badge
              variant={
                run.undone_at
                  ? 'muted'
                  : run.result === 'failed'
                    ? 'destructive'
                    : 'success'
              }
            >
              {run.undone_at ? t('automation.undone') : run.result}
            </Badge>
            {run.undoable ? (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={undo.isPending}
                onClick={() => {
                  undo.mutate(run.id, {
                    onSuccess: () => {
                      toast.add({
                        title: t('automation.undoDone'),
                        type: 'success',
                      })
                    },
                  })
                }}
              >
                <RotateCcwGlassIcon size="1.2em" />
                {t('automation.undo')}
              </Button>
            ) : null}
          </li>
        ))}
      </ul>
    </div>
  )
}

function DnsWriter({ current }: Readonly<{ current: string }>) {
  const { t } = useTranslation('domains')
  const plugins = usePlugins()
  const choose = useSetDnsProviderPlugin()
  const options = (Array.isArray(plugins.data) ? plugins.data : []).filter(
    (p) => p.kind === 'dns_provider' && p.state === 'enabled',
  )
  if (current === '' && options.length === 0) return null
  return (
    <div className="space-y-1.5">
      <Label htmlFor="automation-dns-writer">
        {t('automation.dnsWriter.label')}
      </Label>
      <select
        id="automation-dns-writer"
        className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm"
        value={current}
        disabled={choose.isPending}
        onChange={(e) => {
          choose.mutate(e.target.value, {
            onSuccess: () => {
              toast.add({
                title: t('automation.dnsWriter.saved'),
                type: 'success',
              })
            },
          })
        }}
      >
        <option value="">{t('automation.dnsWriter.builtIn')}</option>
        {current !== '' && !options.some((p) => p.name === current) ? (
          <option value={current}>
            {t('automation.dnsWriter.plugin', { name: current })}
          </option>
        ) : null}
        {options.map((p) => (
          <option key={p.name} value={p.name}>
            {t('automation.dnsWriter.plugin', { name: p.name })}
          </option>
        ))}
      </select>
      <p className="text-xs text-muted-foreground">
        {t('automation.dnsWriter.help')}{' '}
        <Link to="/settings/plugins" className="underline">
          {t('automation.dnsWriter.managePlugins')}
        </Link>
      </p>
      {choose.isError ? (
        <p className="text-xs text-destructive">{choose.error.message}</p>
      ) : null}
    </div>
  )
}

// The go-live policy as plain toggles, a live "what will happen" preview and
// the history of recent runs.
export function DomainAutomationCard() {
  const { t } = useTranslation('domains')
  const { data } = useDomainAutomation()
  const update = useUpdateDomainAutomation()
  if (!data) return null

  const policy: GoLivePolicy = {
    auto_dns: data.auto_dns,
    auto_proxy_route: data.auto_proxy_route,
    force_https: data.force_https,
    www_policy: data.www_policy,
    attach_www_counterpart: data.attach_www_counterpart,
    wildcard_for_base_domain: data.wildcard_for_base_domain,
    verify_after: data.verify_after,
  }
  const save = (next: GoLivePolicy) => {
    update.mutate(next, {
      onSuccess: () => {
        toast.add({ title: t('automation.saved'), type: 'success' })
      },
    })
  }
  const disabled = (key: BoolKey) =>
    update.isPending ||
    (key === 'auto_dns' && data.dns_provider === 'none') ||
    (key === 'auto_proxy_route' && !data.proxy_integration_enabled)

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ZapGlassIcon className="size-4.5" />
          {t('automation.title')}
        </CardTitle>
        <CardDescription>{t('automation.description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <ul className="space-y-3">
          {TOGGLES.map((key) => (
            <li key={key} className="flex items-start gap-3">
              <Switch
                id={`automation-${key}`}
                checked={policy[key]}
                disabled={disabled(key)}
                onCheckedChange={(checked) => {
                  save({ ...policy, [key]: checked })
                }}
              />
              <div className="min-w-0">
                <Label htmlFor={`automation-${key}`}>
                  {t(`automation.toggles.${key}.label`)}
                </Label>
                <p className="text-xs text-muted-foreground">
                  {t(`automation.toggles.${key}.help`)}
                </p>
              </div>
            </li>
          ))}
        </ul>
        <div className="space-y-1.5">
          <Label htmlFor="automation-www-policy">
            {t('automation.www.label')}
          </Label>
          <select
            id="automation-www-policy"
            className="h-9 w-full rounded-md border border-input bg-background px-2 text-sm"
            value={policy.www_policy}
            disabled={update.isPending}
            onChange={(e) => {
              save({
                ...policy,
                www_policy: e.target.value as GoLivePolicy['www_policy'],
              })
            }}
          >
            {WWW_POLICIES.map((p) => (
              <option key={p} value={p}>
                {t(`automation.www.${p}`)}
              </option>
            ))}
          </select>
          <p className="text-xs text-muted-foreground">
            {t('automation.www.help')}
          </p>
        </div>
        <DnsWriter current={data.dns_provider_plugin} />
        {update.isError ? (
          <p className="text-xs text-destructive">{update.error.message}</p>
        ) : null}
        <PolicyPreview policy={policy} />
        <History />
      </CardContent>
    </Card>
  )
}
