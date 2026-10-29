import { useState } from 'react'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  GlobeGlassIcon,
  PlugGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Link } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import type { TFunction } from 'i18next'
import {
  useAppExposure,
  useCheckExposure,
  usePreviewExposure,
  useSetExposure,
  type ExposureMode,
  type ExposurePlan,
  type ExposureRequest,
  type PortHolder,
  type ProbeTarget,
} from '../queries/appExposure'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button, buttonVariants } from '@/components/ui/button'
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
import { cn } from '@/lib/utils'

function parsePort(raw: string): number | null | undefined {
  const trimmed = raw.trim()
  if (trimmed === '') return undefined
  const n = Number(trimmed)
  return Number.isInteger(n) && n >= 1 && n <= 65535 ? n : null
}

function holderLabel(h: PortHolder, t: TFunction<'appExposure'>) {
  if (h.kind === 'control plane') return t('inUse.control', { name: h.holder })
  if (h.kind === 'stream') return t('inUse.stream', { name: h.holder })
  return t('inUse.app', { name: h.holder })
}

function urlKindLabel(kind: string, t: TFunction<'appExposure'>) {
  if (kind === 'domain') return t('urls.domain')
  if (kind === 'fallback') return t('urls.fallback')
  return t('urls.direct')
}

function failureLabel(code: string | undefined, t: TFunction<'appExposure'>) {
  switch (code) {
    case 'container_not_running':
      return t('check.failures.container_not_running')
    case 'container_not_listening':
      return t('check.failures.container_not_listening')
    case 'firewall_closed':
      return t('check.failures.firewall_closed')
    case 'unreachable_from_host':
      return t('check.failures.unreachable_from_host')
    case 'tls_not_issued':
      return t('check.failures.tls_not_issued')
    case 'proxy_route_missing':
      return t('check.failures.proxy_route_missing')
    case 'upstream_failing':
      return t('check.failures.upstream_failing')
    case 'app_error':
      return t('check.failures.app_error')
    default:
      return t('check.failures.request_failed')
  }
}

function changeKindLabel(kind: string, t: TFunction<'appExposure'>) {
  switch (kind) {
    case 'container_port':
      return t('change.container_port')
    case 'host_port':
      return t('change.host_port')
    case 'bind_address':
      return t('change.bind_address')
    default:
      return t('change.firewall')
  }
}

function valueLabel(v: string | undefined, t: TFunction<'appExposure'>) {
  if (v === 'automatic') return t('values.automatic')
  if (v === 'open') return t('values.open')
  if (v === 'closed') return t('values.closed')
  return v ?? ''
}

// Port and exposure controls for one app: draft, preview, apply, reachability.
export function AppExposurePanel({ appName }: { appName: string }) {
  const { t } = useTranslation('appExposure')
  const { data, isError } = useAppExposure(appName)
  const preview = usePreviewExposure(appName)
  const apply = useSetExposure(appName)
  const [port, setPort] = useState<string | null>(null)
  const [mode, setMode] = useState<ExposureMode | null>(null)
  const [hostPort, setHostPort] = useState<string | null>(null)
  const [plan, setPlan] = useState<ExposurePlan | null>(null)

  if (isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{t('errors.load')}</AlertDescription>
      </Alert>
    )
  }
  if (!data) return null

  const portValue = port ?? String(data.container_port)
  const modeValue = mode ?? data.mode
  const hostPortValue =
    hostPort ?? (data.host_port ? String(data.host_port) : '')
  const parsedPort = parsePort(portValue)
  const parsedHost = parsePort(hostPortValue)
  const invalid =
    parsedPort === null || parsedPort === undefined || parsedHost === null

  const request = (): ExposureRequest => {
    const req: ExposureRequest = { mode: modeValue }
    if (parsedPort && parsedPort !== data.container_port)
      req.container_port = parsedPort
    if (modeValue === 'direct' && parsedHost && parsedHost !== data.host_port) {
      req.host_port = parsedHost
    }
    return req
  }
  const reset = () => {
    setPort(null)
    setMode(null)
    setHostPort(null)
    setPlan(null)
    preview.reset()
  }
  const runPreview = (req: ExposureRequest = request()) =>
    preview.mutate(req, { onSuccess: setPlan })
  const runApply = (req: ExposureRequest) =>
    apply.mutate(req, {
      onSuccess: () => {
        reset()
        toast.add({
          title: t('applied'),
          description: t('appliedDescription'),
          type: 'success',
        })
      },
    })
  const remoteOnly = !data.local_node
  const mutationError = preview.error ?? apply.error

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <PlugGlassIcon className="size-5" />
          {t('title')}
        </CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <div className="max-w-xs space-y-1.5">
          <Label htmlFor="exposure-container-port">{t('containerPort')}</Label>
          <Input
            id="exposure-container-port"
            inputMode="numeric"
            className="font-mono"
            value={portValue}
            aria-invalid={parsedPort === null || parsedPort === undefined}
            onChange={(e) => {
              setPort(e.target.value)
              setPlan(null)
            }}
          />
          <p className="text-xs text-muted-foreground">
            {t('containerPortHelp')}
          </p>
        </div>

        <fieldset className="space-y-2">
          <legend className="text-sm font-medium">{t('mode.label')}</legend>
          {(['proxied', 'direct'] as const).map((m) => (
            <label
              key={m}
              className={cn(
                'flex cursor-pointer items-start gap-3 rounded-lg border p-3 text-sm',
                modeValue === m
                  ? 'border-primary bg-accent/40'
                  : 'border-border',
                m === 'direct' && remoteOnly && 'cursor-not-allowed opacity-60',
              )}
            >
              <input
                type="radio"
                name="exposure-mode"
                className="mt-1"
                checked={modeValue === m}
                disabled={m === 'direct' && remoteOnly}
                onChange={() => {
                  setMode(m)
                  setPlan(null)
                }}
              />
              <span className="space-y-0.5">
                <span className="block font-medium">{t(`mode.${m}`)}</span>
                <span className="block text-muted-foreground">
                  {t(`mode.${m}Help`)}
                </span>
              </span>
            </label>
          ))}
          {remoteOnly && (
            <p className="text-xs text-muted-foreground">{t('remoteNode')}</p>
          )}
        </fieldset>

        {modeValue === 'direct' && (
          <div className="max-w-xs space-y-1.5">
            <Label htmlFor="exposure-host-port">{t('hostPort')}</Label>
            <Input
              id="exposure-host-port"
              inputMode="numeric"
              className="font-mono"
              value={hostPortValue}
              aria-invalid={parsedHost === null}
              onChange={(e) => {
                setHostPort(e.target.value)
                setPlan(null)
              }}
            />
            <p className="text-xs text-muted-foreground">
              {t('hostPortHelp', {
                lo: data.port_range[0],
                hi: data.port_range[1],
              })}
            </p>
          </div>
        )}

        {invalid && (
          <p className="text-sm text-destructive">{t('errors.invalidPort')}</p>
        )}
        {mutationError && (
          <Alert variant="destructive">
            <AlertDescription>{mutationError.message}</AlertDescription>
          </Alert>
        )}

        {plan && <PlanView plan={plan} />}

        <div className="flex flex-wrap items-center gap-2">
          {plan && plan.changes.length > 0 ? (
            <Button
              disabled={apply.isPending}
              onClick={() => runApply(request())}
            >
              {apply.isPending ? t('applying') : t('apply')}
            </Button>
          ) : (
            <Button
              variant="outline"
              disabled={invalid || preview.isPending}
              onClick={() => runPreview()}
            >
              {preview.isPending ? t('previewing') : t('preview')}
            </Button>
          )}
          {(plan || port !== null || mode !== null || hostPort !== null) && (
            <Button variant="ghost" onClick={reset}>
              {t('reset')}
            </Button>
          )}
          {data.mode === 'direct' && (
            <Button
              variant="ghost"
              disabled={apply.isPending}
              onClick={() => runApply({ mode: 'proxied' })}
            >
              {t('revert')}
            </Button>
          )}
        </div>

        <UrlList urls={data.urls} port={data.container_port} />
        <ReachabilityCheck appName={appName} />
        <PortsInUse rows={data.ports_in_use} />
      </CardContent>
    </Card>
  )
}

function PlanView({ plan }: { plan: ExposurePlan }) {
  const { t } = useTranslation('appExposure')
  const value = (v?: string) => valueLabel(v, t)
  if (plan.changes.length === 0) {
    return (
      <p className="text-sm text-muted-foreground">{t('nothingToChange')}</p>
    )
  }
  return (
    <div className="space-y-3 rounded-lg border border-border p-3 text-sm">
      <p className="font-medium">{t('willChange')}</p>
      <ul className="space-y-1">
        {plan.changes.map((c) => (
          <li key={`${c.kind}-${c.to}`} className="flex flex-wrap gap-x-2">
            <span className="text-muted-foreground">
              {changeKindLabel(c.kind, t)}
            </span>
            <span className="font-mono">
              {value(c.from)} {'->'} {value(c.to)}
            </span>
          </li>
        ))}
      </ul>
      {plan.recreates && (
        <p className="text-muted-foreground">{t('recreates')}</p>
      )}
      {plan.warnings?.includes('plain_http') && (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="size-5 text-tone-warning" />
          <AlertDescription>{t('plainHttpWarning')}</AlertDescription>
        </Alert>
      )}
    </div>
  )
}

function UrlList({
  urls,
  port,
}: {
  urls: { kind: string; url: string }[]
  port: number
}) {
  const { t } = useTranslation('appExposure')
  return (
    <div className="space-y-2 border-t border-border pt-4">
      <h3 className="text-sm font-medium">{t('urls.title')}</h3>
      {urls.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('urls.none')}</p>
      ) : (
        <ul className="space-y-1 text-sm">
          {urls.map((u) => (
            <li
              key={`${u.kind}-${u.url}`}
              className="flex flex-wrap items-center gap-2"
            >
              <GlobeGlassIcon
                className="size-5 text-muted-foreground"
                aria-hidden="true"
              />
              <span className="text-muted-foreground">
                {urlKindLabel(u.kind, t)}
              </span>
              <a
                href={u.url}
                target="_blank"
                rel="noreferrer"
                className="font-mono underline-offset-2 hover:underline"
              >
                {u.url}
              </a>
            </li>
          ))}
        </ul>
      )}
      <p className="text-xs text-muted-foreground">
        {t('urls.internal', { port })}
      </p>
    </div>
  )
}

function ReachabilityCheck({ appName }: { appName: string }) {
  const { t } = useTranslation('appExposure')
  const check = useCheckExposure(appName)
  const result = check.data
  return (
    <div className="space-y-3 border-t border-border pt-4">
      <div className="flex flex-wrap items-center gap-3">
        <Button
          variant="outline"
          disabled={check.isPending}
          onClick={() => check.mutate()}
        >
          {check.isPending ? t('check.running') : t('check.button')}
        </Button>
        <span className="text-xs text-muted-foreground">
          {t('check.scope')}
        </span>
      </div>
      {check.error && (
        <Alert variant="destructive">
          <AlertDescription>{check.error.message}</AlertDescription>
        </Alert>
      )}
      {result && (
        <ul className="space-y-2 text-sm">
          {result.targets.map((tg) => (
            <ProbeRow key={`${tg.kind}-${tg.url}`} target={tg} />
          ))}
        </ul>
      )}
    </div>
  )
}

function ProbeRow({ target }: { target: ProbeTarget }) {
  const { t } = useTranslation('appExposure')
  const Icon = target.ok ? CircleCheckGlassIcon : CircleXGlassIcon
  const fix =
    target.failure === 'firewall_closed' ? (
      <Link
        to="/settings/firewall"
        className={buttonVariants({ variant: 'link', size: 'sm' })}
      >
        {t('check.fixFirewall')}
      </Link>
    ) : target.failure === 'tls_not_issued' ? (
      <Link
        to="/domains"
        className={buttonVariants({ variant: 'link', size: 'sm' })}
      >
        {t('check.fixCertificates')}
      </Link>
    ) : null
  return (
    <li className="flex items-start gap-2">
      <Icon
        className={cn(
          'mt-0.5 size-4 shrink-0',
          target.ok ? 'text-success' : 'text-destructive',
        )}
        aria-hidden="true"
      />
      <div className="space-y-0.5">
        <div className="font-mono text-xs break-all">
          {target.url || target.kind}
        </div>
        <div>
          {target.ok
            ? `${t('check.ok')}: ${t('check.status', { status: target.status, ms: target.latency_ms ?? 0 })}`
            : failureLabel(target.failure, t)}
        </div>
        {fix}
      </div>
    </li>
  )
}

function PortsInUse({ rows }: { rows: PortHolder[] }) {
  const { t } = useTranslation('appExposure')
  return (
    <div className="space-y-2 border-t border-border pt-4">
      <h3 className="text-sm font-medium">{t('inUse.title')}</h3>
      {rows.length === 0 ? (
        <p className="text-sm text-muted-foreground">{t('inUse.empty')}</p>
      ) : (
        <table className="w-full text-sm">
          <thead>
            <tr className="text-left text-muted-foreground">
              <th className="py-1 pr-4 font-normal">{t('inUse.port')}</th>
              <th className="py-1 font-normal">{t('inUse.heldBy')}</th>
            </tr>
          </thead>
          <tbody>
            {rows.map((r) => (
              <tr key={`${r.port}-${r.holder}`}>
                <td className="py-1 pr-4 font-mono">{r.port}</td>
                <td className="py-1">{holderLabel(r, t)}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
    </div>
  )
}
