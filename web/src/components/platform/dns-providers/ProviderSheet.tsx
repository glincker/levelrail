import { useState, type FormEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { useQuery } from '@tanstack/react-query'
import {
  CircleCheckGlassIcon,
  CircleXGlassIcon,
  SearchGlassIcon,
} from '@thesvg/ui-react'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { toast } from '@/components/ui/toast'
import { DnsHostTile } from '@/components/dns/DnsHostChip'
import {
  cloudflareDnsSettingsQueryOptions,
  useDisconnectCloudflareDns,
  useUpdateCloudflareDnsSettings,
} from '@/queries/cloudflareDns'
import {
  route53DnsSettingsQueryOptions,
  useDisconnectRoute53Dns,
  useUpdateRoute53DnsSettings,
} from '@/queries/route53Dns'
import {
  useDisconnectGoogleCloudDns,
  useGoogleCloudDnsSettings,
  useTestGoogleCloudDns,
  useUpdateGoogleCloudDns,
} from '@/queries/googleCloudDns'
import type { DnsZone } from '@/types/dns'
import type { ProviderId, ProviderState } from './providers'

interface Step {
  key: 'credential' | 'zones'
  ok: boolean
  detail?: string
}

function ProviderForm({
  id,
  onSaved,
  busy,
  setBusy,
}: {
  id: ProviderId
  onSaved: () => void
  busy: boolean
  setBusy: (b: boolean) => void
}) {
  const { t } = useTranslation('dnsProviders')
  const label = t(`providers.${id}`)
  const cfSettings = useQuery(cloudflareDnsSettingsQueryOptions())
  const r53Settings = useQuery(route53DnsSettingsQueryOptions())
  const gcpSettings = useGoogleCloudDnsSettings()
  const cf = useUpdateCloudflareDnsSettings()
  const r53 = useUpdateRoute53DnsSettings()
  const gcp = useUpdateGoogleCloudDns()
  const [token, setToken] = useState('')
  const [keyId, setKeyId] = useState('')
  const [secret, setSecret] = useState('')
  const [region, setRegion] = useState('')
  const [zoneId, setZoneId] = useState('')
  const [json, setJson] = useState('')
  const [error, setError] = useState<string | null>(null)

  const stored =
    id === 'cloudflare'
      ? !!cfSettings.data?.has_token
      : id === 'route53'
        ? !!r53Settings.data?.has_access_key_id
        : !!gcpSettings.data?.has_key

  const afterSave = () => {
    // Secrets leave component state the moment the server has them.
    setToken('')
    setKeyId('')
    setSecret('')
    setJson('')
    setError(null)
    toast.add({ title: t('sheet.saved', { provider: label }), type: 'success' })
    onSaved()
  }
  const fail = (e: Error) =>
    setError(t('sheet.saveFailed', { error: e.message }))

  const submit = (e: FormEvent) => {
    e.preventDefault()
    setBusy(true)
    const done = {
      onSuccess: afterSave,
      onError: fail,
      onSettled: () => setBusy(false),
    }
    if (id === 'cloudflare') {
      cf.mutate({ enabled: true, token: token.trim() || undefined }, done)
    } else if (id === 'route53') {
      r53.mutate(
        {
          enabled: true,
          region: region.trim() || r53Settings.data?.region,
          hosted_zone_id: zoneId.trim() || r53Settings.data?.hosted_zone_id,
          access_key_id: keyId.trim() || undefined,
          secret_access_key: secret.trim() || undefined,
        },
        done,
      )
    } else {
      gcp.mutate(
        { enabled: true, service_account_json: json.trim() || undefined },
        done,
      )
    }
  }

  const empty =
    id === 'cloudflare'
      ? !token.trim()
      : id === 'route53'
        ? !keyId.trim() && !secret.trim()
        : !json.trim()
  const needsSecret = !stored && empty

  return (
    <form onSubmit={submit} className="space-y-3" aria-label={label}>
      {id === 'cloudflare' ? (
        <Field>
          <FieldLabel htmlFor="dnsp-cf-token">
            {t('field.cloudflareToken')}
          </FieldLabel>
          <Input
            id="dnsp-cf-token"
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={token}
            onChange={(e) => setToken(e.target.value)}
            disabled={busy}
          />
        </Field>
      ) : null}
      {id === 'route53' ? (
        <>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="dnsp-r53-key">
                {t('field.accessKeyId')}
              </FieldLabel>
              <Input
                id="dnsp-r53-key"
                autoComplete="off"
                spellCheck={false}
                value={keyId}
                onChange={(e) => setKeyId(e.target.value)}
                disabled={busy}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="dnsp-r53-secret">
                {t('field.secretAccessKey')}
              </FieldLabel>
              <Input
                id="dnsp-r53-secret"
                type="password"
                autoComplete="off"
                value={secret}
                onChange={(e) => setSecret(e.target.value)}
                disabled={busy}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="dnsp-r53-region">
                {t('field.region')}
              </FieldLabel>
              <Input
                id="dnsp-r53-region"
                autoComplete="off"
                placeholder={r53Settings.data?.region ?? 'us-east-1'}
                value={region}
                onChange={(e) => setRegion(e.target.value)}
                disabled={busy}
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="dnsp-r53-zone">
                {t('field.hostedZoneId')}
              </FieldLabel>
              <Input
                id="dnsp-r53-zone"
                autoComplete="off"
                placeholder={r53Settings.data?.hosted_zone_id ?? ''}
                value={zoneId}
                onChange={(e) => setZoneId(e.target.value)}
                disabled={busy}
              />
            </Field>
          </div>
        </>
      ) : null}
      {id === 'googleclouddns' ? (
        <Field>
          <FieldLabel htmlFor="dnsp-gcp-json">
            {t('field.serviceAccount')}
          </FieldLabel>
          <Input
            id="dnsp-gcp-json"
            type="password"
            autoComplete="off"
            spellCheck={false}
            value={json}
            onChange={(e) => setJson(e.target.value)}
            disabled={busy}
          />
        </Field>
      ) : null}
      {stored ? (
        <FieldDescription>{t('sheet.keepHint')}</FieldDescription>
      ) : null}
      {error ? (
        <p role="alert" className="text-sm text-tone-danger">
          {error}
        </p>
      ) : null}
      <Button type="submit" size="sm" disabled={busy || needsSecret}>
        {busy
          ? t('sheet.saving')
          : stored
            ? t('sheet.update')
            : t('sheet.save')}
      </Button>
    </form>
  )
}

function TestPanel({
  id,
  state,
  zones,
  refetchZones,
  onResult,
}: {
  id: ProviderId
  state: ProviderState
  zones: DnsZone[]
  refetchZones: () => Promise<DnsZone[] | undefined>
  onResult: (error: string | null) => void
}) {
  const { t } = useTranslation('dnsProviders')
  const gcpTest = useTestGoogleCloudDns()
  const [steps, setSteps] = useState<Step[] | null>(null)
  const [running, setRunning] = useState(false)

  const run = async () => {
    setRunning(true)
    try {
      if (id === 'googleclouddns') {
        const r = await gcpTest.mutateAsync()
        setSteps([
          { key: 'credential', ok: r.ok, detail: r.ok ? undefined : r.error },
          {
            key: 'zones',
            ok: r.ok,
            detail: r.ok
              ? t('test.zonesCount', { count: r.zones?.length ?? 0 })
              : undefined,
          },
        ])
        onResult(r.ok ? null : (r.error ?? t('test.failed')))
      } else {
        const fresh = await refetchZones()
        const count = (fresh ?? zones).filter((z) => z.provider === id).length
        const ok = state.status !== 'notConnected'
        setSteps([
          { key: 'credential', ok },
          {
            key: 'zones',
            ok: ok && count > 0,
            detail: t('test.zonesCount', { count }),
          },
        ])
        onResult(ok && count > 0 ? null : t('test.failed'))
      }
    } catch (e) {
      const message = e instanceof Error ? e.message : t('test.failed')
      setSteps([{ key: 'credential', ok: false, detail: message }])
      onResult(message)
    } finally {
      setRunning(false)
    }
  }

  return (
    <div className="space-y-2">
      <Button
        size="sm"
        variant="outline"
        disabled={running}
        onClick={() => void run()}
      >
        {running ? t('test.running') : t('test.button')}
      </Button>
      {steps ? (
        <ul aria-label={t('test.heading')} className="space-y-1 text-sm">
          {steps.map((s) => (
            <li key={s.key} className="flex items-center gap-2">
              {s.ok ? (
                <CircleCheckGlassIcon
                  className="size-4 text-tone-success"
                  aria-hidden="true"
                />
              ) : (
                <CircleXGlassIcon
                  className="size-4 text-tone-danger"
                  aria-hidden="true"
                />
              )}
              <span>{t(`test.${s.key}`)}</span>
              <span className="text-muted-foreground">
                {s.detail ?? (s.ok ? t('test.ok') : t('test.failed'))}
              </span>
            </li>
          ))}
        </ul>
      ) : null}
    </div>
  )
}

export function ProviderSheet({
  id,
  state,
  zones,
  refetchZones,
  onOpenZone,
  onClose,
  onTestResult,
}: {
  id: ProviderId | null
  state: ProviderState | null
  zones: DnsZone[]
  refetchZones: () => Promise<DnsZone[] | undefined>
  onOpenZone: (zone: DnsZone) => void
  onClose: () => void
  onTestResult: (id: ProviderId, error: string | null) => void
}) {
  const { t } = useTranslation('dnsProviders')
  const [busy, setBusy] = useState(false)
  const [confirm, setConfirm] = useState(false)
  const [zoneQuery, setZoneQuery] = useState('')
  const disconnectCf = useDisconnectCloudflareDns()
  const disconnectR53 = useDisconnectRoute53Dns()
  const disconnectGcp = useDisconnectGoogleCloudDns()
  const label = id ? t(`providers.${id}`) : ''
  const mine = zones.filter(
    (z) =>
      z.provider === id &&
      z.name.toLowerCase().includes(zoneQuery.trim().toLowerCase()),
  )
  const connected =
    state?.status === 'connected' || state?.status === 'attention'
  const mutation =
    id === 'cloudflare'
      ? disconnectCf
      : id === 'route53'
        ? disconnectR53
        : disconnectGcp

  return (
    <>
      <Sheet open={id !== null} onOpenChange={(o) => !o && onClose()}>
        <SheetContent className="overflow-y-auto p-4 sm:max-w-lg">
          {id && state ? (
            <>
              <SheetHeader className="p-0">
                <SheetTitle className="flex items-center gap-2">
                  <DnsHostTile
                    provider={id === 'googleclouddns' ? 'googlecloud' : id}
                  />
                  {label}
                </SheetTitle>
                <SheetDescription>{t(`perm.${id}`)}</SheetDescription>
              </SheetHeader>
              {state.status === 'attention' ? (
                <p role="alert" className="text-sm text-tone-danger">
                  {state.error
                    ? t('attention.testFailed', { error: state.error })
                    : t('attention.noCredential')}
                </p>
              ) : null}
              <ProviderForm
                id={id}
                onSaved={() => undefined}
                busy={busy}
                setBusy={setBusy}
              />
              {connected ? (
                <>
                  <TestPanel
                    id={id}
                    state={state}
                    zones={zones}
                    refetchZones={refetchZones}
                    onResult={(e) => onTestResult(id, e)}
                  />
                  <section className="space-y-2">
                    <h3 className="text-sm font-medium">
                      {t('zones.inProvider')}
                    </h3>
                    <div className="relative">
                      <SearchGlassIcon
                        className="pointer-events-none absolute top-2.5 left-2.5 size-4 text-muted-foreground"
                        aria-hidden="true"
                      />
                      <Input
                        aria-label={t('zones.searchInSheet')}
                        placeholder={t('zones.searchInSheet')}
                        className="pl-8"
                        value={zoneQuery}
                        onChange={(e) => setZoneQuery(e.target.value)}
                      />
                    </div>
                    <ul className="divide-y divide-border rounded-md border border-border">
                      {mine.map((z) => (
                        <li key={z.id}>
                          <button
                            type="button"
                            className="flex w-full items-center justify-between gap-2 px-3 py-2 text-left text-sm hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
                            onClick={() => onOpenZone(z)}
                          >
                            <span className="truncate font-mono">{z.name}</span>
                            <Badge variant="muted">{z.record_count ?? 0}</Badge>
                          </button>
                        </li>
                      ))}
                      {mine.length === 0 ? (
                        <li className="px-3 py-2 text-sm text-muted-foreground">
                          {t('zones.none')}
                        </li>
                      ) : null}
                    </ul>
                  </section>
                  <Button
                    size="sm"
                    variant="outline"
                    className="self-start text-tone-danger"
                    onClick={() => setConfirm(true)}
                  >
                    {t('sheet.disconnect')}
                  </Button>
                </>
              ) : null}
            </>
          ) : null}
        </SheetContent>
      </Sheet>
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        tone="destructive"
        title={t('sheet.disconnectTitle', { provider: label })}
        description={t('sheet.disconnectBody')}
        confirmLabel={t('sheet.disconnectConfirm')}
        pendingLabel={t('sheet.disconnecting')}
        requireTyped={label}
        pending={mutation.isPending}
        onConfirm={() =>
          mutation.mutate(undefined, {
            onSuccess: () => {
              setConfirm(false)
              toast.add({
                title: t('sheet.disconnected', { provider: label }),
                type: 'success',
              })
            },
          })
        }
      />
    </>
  )
}
