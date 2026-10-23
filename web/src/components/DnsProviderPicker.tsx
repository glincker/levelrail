import { useState } from 'react'
import { CommandBlock } from '@/components/ui/code-block'
import { useTranslation } from 'react-i18next'
import { CheckCircleIcon, WarningIcon } from '@phosphor-icons/react/dist/ssr'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import {
  Collapsible,
  CollapsiblePanel,
  CollapsibleTrigger,
} from '@/components/ui/collapsible'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { toast } from '@/components/ui/toast'
import {
  useDnsProvider,
  useSetDnsProvider,
  useTestDnsProvider,
  type DnsProviderFix,
  type DnsProviderRequest,
} from '../queries/dnsProvider'
import { BetaBadge } from './BetaBadge'

const readApiError = (err: unknown): string =>
  err instanceof Error ? err.message : ''

const INHERIT = 'inherit'
const CUSTOM = 'custom'

/**
 * DnsProviderPicker chooses the wildcard-DNS provider behind automatic hostnames:
 * the instance default when app is omitted, one app's override otherwise.
 */
export function DnsProviderPicker({ app }: { app?: string }) {
  const { t } = useTranslation('https')
  const { data: state } = useDnsProvider(app)
  const save = useSetDnsProvider(app)
  const test = useTestDnsProvider(app)
  const [choice, setChoice] = useState<string | null>(null)
  const [base, setBase] = useState<string | null>(null)
  const [confirming, setConfirming] = useState(false)
  if (!state) return null

  const savedChoice =
    app && state.effective.inherited ? INHERIT : state.effective.provider
  const provider = choice ?? savedChoice
  const baseDomain = base ?? state.effective.custom_base_domain ?? ''
  const isFree = provider !== CUSTOM && provider !== INHERIT
  const info = state.providers.find((p) => p.id === provider)
  const dirty =
    provider !== savedChoice ||
    (provider === CUSTOM &&
      baseDomain !== (state.effective.custom_base_domain ?? ''))
  const body: DnsProviderRequest =
    provider === INHERIT
      ? { provider: '' }
      : {
          provider,
          custom_base_domain: provider === CUSTOM ? baseDomain : undefined,
        }

  const labelFor = (id: string) => {
    if (id === INHERIT) {
      return t('dnsProvider.inherit', { provider: state.default.provider })
    }
    if (id === CUSTOM) return t('dnsProvider.custom')
    return state.providers.find((p) => p.id === id)?.name ?? id
  }

  const apply = (next: DnsProviderRequest) =>
    save.mutate(next, {
      onSuccess: () => {
        setChoice(null)
        setBase(null)
        setConfirming(false)
        test.reset()
        toast.add({ title: t('dnsProvider.saved'), type: 'success' })
      },
      onError: (err) =>
        toast.add({
          title: t('dnsProvider.saveFailed'),
          description: readApiError(err),
          type: 'error',
        }),
    })

  const applyFix = (fix: DnsProviderFix) => {
    if (fix.kind === 'switch_provider' && fix.provider) {
      setChoice(fix.provider)
      test.reset()
    } else {
      setChoice(CUSTOM)
    }
  }

  const result = test.data
  return (
    <div className="space-y-3 rounded-md border border-border px-3 py-3">
      <div className="space-y-1.5">
        <Label htmlFor={`dns-provider-${app ?? 'default'}`}>
          {app ? t('dnsProvider.appLabel') : t('dnsProvider.defaultLabel')}
        </Label>
        <div className="flex flex-wrap items-center gap-2">
          <Select
            value={provider}
            onValueChange={(v) => {
              if (v) {
                setChoice(v)
                test.reset()
              }
            }}
          >
            <SelectTrigger
              id={`dns-provider-${app ?? 'default'}`}
              className="min-w-48"
            >
              <SelectValue>{labelFor(provider)}</SelectValue>
            </SelectTrigger>
            <SelectContent>
              {app ? (
                <SelectItem value={INHERIT}>{labelFor(INHERIT)}</SelectItem>
              ) : null}
              {state.providers.map((p) => (
                <SelectItem key={p.id} value={p.id}>
                  {p.name}
                </SelectItem>
              ))}
              <SelectItem value={CUSTOM}>{labelFor(CUSTOM)}</SelectItem>
            </SelectContent>
          </Select>
          {isFree ? <BetaBadge hint={t('dnsProvider.betaHint')} /> : null}
        </div>
        <p className="text-xs text-muted-foreground">
          {provider === CUSTOM
            ? t('dnsProvider.customNote')
            : provider === INHERIT
              ? t('dnsProvider.inheritNote')
              : info?.note}
        </p>
      </div>

      {provider === CUSTOM ? (
        <div className="space-y-1.5">
          <Label htmlFor={`dns-base-${app ?? 'default'}`}>
            {t('dnsProvider.baseLabel')}
          </Label>
          <Input
            id={`dns-base-${app ?? 'default'}`}
            className="font-mono"
            placeholder={t('dnsProvider.basePlaceholder')}
            value={baseDomain}
            onChange={(e) => setBase(e.target.value)}
          />
          <p className="text-xs text-muted-foreground">
            {t('dnsProvider.baseHint', {
              domain: baseDomain || 'apps.example.com',
            })}
          </p>
        </div>
      ) : null}

      <div className="flex flex-wrap items-center gap-2">
        <Button
          size="sm"
          disabled={
            !dirty ||
            save.isPending ||
            (provider === CUSTOM && !baseDomain.trim())
          }
          onClick={() => (app ? setConfirming(true) : apply(body))}
        >
          {t('dnsProvider.save')}
        </Button>
        <Button
          size="sm"
          variant="outline"
          disabled={test.isPending}
          onClick={() => test.mutate(body)}
        >
          {test.isPending ? t('dnsProvider.testing') : t('dnsProvider.test')}
        </Button>
      </div>

      {test.isError ? (
        <Alert variant="destructive">
          <WarningIcon />
          <AlertDescription>{readApiError(test.error)}</AlertDescription>
        </Alert>
      ) : null}
      {result ? (
        <Alert variant={result.status === 'ok' ? 'default' : 'destructive'}>
          {result.status === 'ok' ? <CheckCircleIcon /> : <WarningIcon />}
          <AlertDescription>
            <p>{t(`dnsProvider.status.${result.status}`)}</p>
            <p className="text-xs">{result.message}</p>
            {result.fixes && result.fixes.length > 0 ? (
              <div className="mt-2 flex flex-wrap items-center gap-2">
                <span className="text-xs">{t('dnsProvider.fixHeading')}</span>
                {result.fixes.map((fix) => (
                  <Button
                    key={`${fix.kind}-${fix.provider ?? ''}`}
                    size="xs"
                    variant="outline"
                    onClick={() => applyFix(fix)}
                  >
                    {fix.kind === 'switch_provider'
                      ? t('dnsProvider.fixSwitch', {
                          provider: labelFor(fix.provider ?? ''),
                        })
                      : t('dnsProvider.fixCustom')}
                  </Button>
                ))}
              </div>
            ) : null}
            {result.dig_command ? (
              <Collapsible className="mt-2">
                <CollapsibleTrigger className="text-xs text-muted-foreground hover:text-foreground">
                  {t('dnsProvider.digToggle')}
                </CollapsibleTrigger>
                <CollapsiblePanel>
                  <CommandBlock
                    className="mt-1"
                    code={result.dig_command}
                    label={t('dnsProvider.digToggle')}
                  />
                </CollapsiblePanel>
              </Collapsible>
            ) : null}
          </AlertDescription>
        </Alert>
      ) : null}

      <ConfirmDialog
        open={confirming}
        onOpenChange={setConfirming}
        title={t('dnsProvider.confirmTitle')}
        description={t('dnsProvider.confirmBody', {
          host: state.hostname ?? '',
        })}
        consequences={[
          t('dnsProvider.confirmOldStops'),
          t('dnsProvider.confirmNoDowntime'),
        ]}
        confirmLabel={t('dnsProvider.confirmAction')}
        pending={save.isPending}
        onConfirm={() => apply(body)}
      />
    </div>
  )
}
