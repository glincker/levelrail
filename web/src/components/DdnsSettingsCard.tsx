import { useState } from 'react'
import { useForm } from 'react-hook-form'
import { useTranslation } from 'react-i18next'
import { ChevronDownGlassIcon, GlobeGlassIcon } from '@thesvg/ui-react'
import type { DdnsProvider, DdnsSettings, DdnsStatus } from '../queries/ddns'
import {
  useDdnsSettings,
  useDdnsStatus,
  useUpdateDdnsSettings,
} from '../queries/ddns'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { NativeSelect } from '@/components/ui/native-select'
import { SecretInput } from '@/components/ui/secret-input'
import { Switch } from '@/components/ui/switch'
import { Textarea } from '@/components/ui/textarea'
import { toast } from '@/components/ui/toast'

interface FormValues {
  enabled: boolean
  provider: DdnsProvider
  domains: string
  target: string
  interval: string
  timeout: string
  minGap: string
  reassert: string
  failAlertAfter: string
  quorum: string
  probeUrls: string
  allowPrivate: boolean
  allowInsecureHttp: boolean
}

function toForm(s: DdnsSettings): FormValues {
  return {
    enabled: s.enabled,
    provider: s.provider,
    domains: s.domains ?? '',
    target: '',
    interval: String(s.interval_seconds),
    timeout: String(s.timeout_seconds),
    minGap: String(s.min_update_gap_seconds),
    reassert: String(s.reassert_seconds),
    failAlertAfter: String(s.fail_alert_after),
    quorum: String(s.probe_quorum),
    probeUrls: (s.probe_urls ?? []).join('\n'),
    allowPrivate: s.allow_private,
    allowInsecureHttp: s.allow_insecure_http,
  }
}

const isWholeNumber = (v: string) => /^\d+$/.test(v.trim()) && Number(v) >= 1

type ChipKind = 'running' | 'failing' | 'off'

function chipKind(status: DdnsStatus | undefined): ChipKind {
  if (!status?.running) return 'off'
  if (status.last_result === 'failing' || status.alert_active) return 'failing'
  return 'running'
}

const CHIP_VARIANT = {
  running: 'success',
  failing: 'destructive',
  off: 'muted',
} as const

function when(iso: string | undefined): string | null {
  if (!iso || iso.startsWith('0001-')) return null
  return new Date(iso).toLocaleString()
}

function LastCheckLine({ status }: { status: DdnsStatus | undefined }) {
  const { t } = useTranslation('ddns')
  if (!status) return null
  const checked = when(status.last_check)
  const changed = when(status.last_change)
  if (!checked) {
    return <p className="text-xs text-muted-foreground">{t('last.never')}</p>
  }
  const parts = [
    status.last_ip ? t('last.ip', { ip: status.last_ip }) : null,
    t('last.result', { result: t(`result.${status.last_result}`) }),
    t('last.check', { when: checked }),
    changed ? t('last.change', { when: changed }) : null,
    status.consecutive_failures > 0
      ? t('last.failures', { count: status.consecutive_failures })
      : null,
    status.alert_active ? t('last.alert') : null,
  ].filter((p): p is string => p !== null)
  return (
    <div className="space-y-1" aria-live="polite">
      <p className="text-xs text-muted-foreground">
        <span className="font-medium text-foreground">{t('last.title')}:</span>{' '}
        {parts.join(' · ')}
      </p>
      {status.last_result === 'failing' && status.last_error ? (
        <p className="text-xs break-words text-destructive">
          {status.last_error}
        </p>
      ) : null}
    </div>
  )
}

export function DdnsSettingsCard() {
  const settingsQuery = useDdnsSettings()
  const statusQuery = useDdnsStatus()
  // 501 means the updater is not wired on this instance: hide the card.
  if (settingsQuery.isError || !settingsQuery.data) return null
  return <DdnsForm settings={settingsQuery.data} status={statusQuery.data} />
}

function DdnsForm({
  settings,
  status,
}: {
  settings: DdnsSettings
  status: DdnsStatus | undefined
}) {
  const { t } = useTranslation('ddns')
  const update = useUpdateDdnsSettings()
  const [advancedOpen, setAdvancedOpen] = useState(false)
  const { register, handleSubmit, watch, setValue, reset, formState } =
    useForm<FormValues>({ values: toForm(settings) })
  const provider = watch('provider')
  const enabled = watch('enabled')
  const kind = chipKind(status)
  const numberKeys = [
    'interval',
    'timeout',
    'minGap',
    'reassert',
    'failAlertAfter',
    'quorum',
  ] as const
  const numbers = watch(numberKeys)
  const numbersValid = numbers.every(isWholeNumber)

  const onSubmit = handleSubmit((v) => {
    const probeUrls = v.probeUrls
      .split('\n')
      .map((u) => u.trim())
      .filter(Boolean)
    update.mutate(
      {
        enabled: v.enabled,
        provider: v.provider,
        domains: v.domains.trim(),
        interval_seconds: Number(v.interval),
        timeout_seconds: Number(v.timeout),
        min_update_gap_seconds: Number(v.minGap),
        reassert_seconds: Number(v.reassert),
        fail_alert_after: Number(v.failAlertAfter),
        probe_quorum: Number(v.quorum),
        probe_urls: probeUrls,
        allow_private: v.allowPrivate,
        allow_insecure_http: v.allowInsecureHttp,
        // Omitted when empty so a save never touches the stored credential.
        ...(v.target.trim() ? { target: v.target.trim() } : {}),
      },
      {
        onSuccess: () => {
          reset({ ...v, target: '' })
          toast.add({ title: t('saved'), type: 'success' })
        },
      },
    )
  })

  function clearTarget() {
    update.mutate(
      { target: '' },
      {
        onSuccess: () => {
          toast.add({ title: t('target.cleared'), type: 'success' })
        },
      },
    )
  }

  const targetLabel =
    provider === 'duckdns' ? t('target.labelDuckdns') : t('target.labelGeneric')
  const targetHint = settings.target_configured
    ? settings.host
      ? t('target.configured', { host: settings.host })
      : t('target.configuredNoHost')
    : t('target.notConfigured')

  const numberField = (key: (typeof numberKeys)[number], label: string) => (
    <Field key={key}>
      <FieldLabel htmlFor={`ddns-${key}`}>{label}</FieldLabel>
      <Input
        id={`ddns-${key}`}
        type="number"
        min={1}
        inputMode="numeric"
        {...register(key)}
      />
    </Field>
  )

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex flex-wrap items-center gap-2">
          <GlobeGlassIcon className="size-5" />
          {t('title')}
          <Badge variant={CHIP_VARIANT[kind]} aria-label={t('status.label')}>
            {t(`status.${kind}`)}
          </Badge>
          <span className="ml-auto text-xs font-normal text-muted-foreground">
            {t(`source.${settings.source}`)}
          </span>
        </CardTitle>
        <CardDescription>{t('description')}</CardDescription>
      </CardHeader>
      <CardContent>
        <form
          onSubmit={(e) => {
            void onSubmit(e)
          }}
          className="space-y-3"
        >
          <div className="grid items-end gap-3 sm:grid-cols-[auto_12rem_1fr]">
            <Field orientation="horizontal">
              <Switch
                id="ddns-enabled"
                checked={enabled}
                onCheckedChange={(c) => {
                  setValue('enabled', c, { shouldDirty: true })
                }}
              />
              <FieldLabel htmlFor="ddns-enabled">{t('enabled')}</FieldLabel>
            </Field>
            <Field>
              <FieldLabel htmlFor="ddns-provider">
                {t('provider.label')}
              </FieldLabel>
              <NativeSelect id="ddns-provider" {...register('provider')}>
                <option value="generic">{t('provider.generic')}</option>
                <option value="duckdns">{t('provider.duckdns')}</option>
              </NativeSelect>
            </Field>
            {provider === 'duckdns' ? (
              <Field>
                <FieldLabel htmlFor="ddns-domains">
                  {t('domains.label')}
                </FieldLabel>
                <Input
                  id="ddns-domains"
                  placeholder={t('domains.placeholder')}
                  {...register('domains')}
                />
              </Field>
            ) : (
              <TargetField
                id="ddns-target"
                label={targetLabel}
                hint={targetHint}
                configured={settings.target_configured}
                pending={update.isPending}
                onClear={clearTarget}
                registration={register('target')}
              />
            )}
          </div>
          {provider === 'duckdns' ? (
            <TargetField
              id="ddns-target"
              label={targetLabel}
              hint={targetHint}
              configured={settings.target_configured}
              pending={update.isPending}
              onClear={clearTarget}
              registration={register('target')}
            />
          ) : null}

          <Collapsible open={advancedOpen} onOpenChange={setAdvancedOpen}>
            <CollapsibleTrigger className="flex w-auto items-center gap-1 text-sm text-muted-foreground hover:text-foreground">
              <ChevronDownGlassIcon
                className={
                  advancedOpen
                    ? 'size-4 transition-transform motion-reduce:transition-none'
                    : '-rotate-90 size-4 transition-transform motion-reduce:transition-none'
                }
              />
              {t('advanced.toggle')}
            </CollapsibleTrigger>
            <CollapsiblePanel>
              <div className="space-y-3 pt-3">
                <div className="grid gap-3 sm:grid-cols-3">
                  {numberField('interval', t('advanced.interval'))}
                  {numberField('timeout', t('advanced.timeout'))}
                  {numberField('minGap', t('advanced.minGap'))}
                  {numberField('reassert', t('advanced.reassert'))}
                  {numberField('failAlertAfter', t('advanced.failAlertAfter'))}
                  {numberField('quorum', t('advanced.quorum'))}
                </div>
                <Field>
                  <FieldLabel htmlFor="ddns-probe-urls">
                    {t('advanced.probeUrls')}
                  </FieldLabel>
                  <Textarea
                    id="ddns-probe-urls"
                    rows={3}
                    className="font-mono text-xs"
                    {...register('probeUrls')}
                  />
                </Field>
                <div className="flex flex-wrap gap-x-6 gap-y-2">
                  <Field orientation="horizontal">
                    <Switch
                      id="ddns-allow-private"
                      checked={watch('allowPrivate')}
                      onCheckedChange={(c) => {
                        setValue('allowPrivate', c, { shouldDirty: true })
                      }}
                    />
                    <FieldLabel htmlFor="ddns-allow-private">
                      {t('advanced.allowPrivate')}
                    </FieldLabel>
                  </Field>
                  <Field orientation="horizontal">
                    <Switch
                      id="ddns-allow-http"
                      checked={watch('allowInsecureHttp')}
                      onCheckedChange={(c) => {
                        setValue('allowInsecureHttp', c, { shouldDirty: true })
                      }}
                    />
                    <FieldLabel htmlFor="ddns-allow-http">
                      {t('advanced.allowInsecureHttp')}
                    </FieldLabel>
                  </Field>
                </div>
                {numbersValid ? null : (
                  <p className="text-xs text-destructive" role="alert">
                    {t('error.number')}
                  </p>
                )}
              </div>
            </CollapsiblePanel>
          </Collapsible>

          <div className="flex flex-wrap items-center justify-between gap-3">
            <LastCheckLine status={status} />
            <Button
              type="submit"
              size="sm"
              disabled={update.isPending || !numbersValid || !formState.isDirty}
            >
              {update.isPending ? t('saving') : t('save')}
            </Button>
          </div>
          {update.isError ? (
            <Alert variant="destructive">
              <AlertDescription>{update.error.message}</AlertDescription>
            </Alert>
          ) : null}
        </form>
      </CardContent>
    </Card>
  )
}

function TargetField({
  id,
  label,
  hint,
  configured,
  pending,
  onClear,
  registration,
}: {
  id: string
  label: string
  hint: string
  configured: boolean
  pending: boolean
  onClear: () => void
  registration: ReturnType<ReturnType<typeof useForm<FormValues>>['register']>
}) {
  const { t } = useTranslation('ddns')
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <div className="flex items-center gap-2">
        <SecretInput
          id={id}
          hasExistingValue={configured}
          placeholder={t('target.placeholder')}
          {...registration}
        />
        {configured ? (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={pending}
            onClick={onClear}
          >
            {t('target.clear')}
          </Button>
        ) : null}
      </div>
      <FieldDescription>{hint}</FieldDescription>
    </Field>
  )
}
