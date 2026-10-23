import { lazy, Suspense, useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { SpinnerIcon, SwapIcon } from '@phosphor-icons/react/dist/ssr'
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
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { toast } from '@/components/ui/toast'
import { useIsRoot } from '@/hooks/useIsRoot'
import { ApiError } from '../lib/apiError'
import {
  useProxyIntegration,
  useSetupProxyIntegration,
  useUpdateProxySettings,
  type ProxyIntegration,
  type ProxyProvider,
  type ProxyProviderKind,
  type ProxySettings,
  type ProxySetupResult,
} from '../queries/proxyIntegration'
import { ProxyDetectedFacts } from './proxy/ProxyDetectedFacts'
import {
  ProxyProviderResult,
  type ProxyFailure,
} from './proxy/ProxyProviderResult'
import { ProxyProviderPicker } from './proxy/ProxyProviderPicker'
import { ProxySteps } from './proxy/ProxySteps'
import { isAbsoluteDir, manualDefaults } from '../lib/proxyManualFields'

const ProxyMissingState = lazy(() => import('./proxy/ProxyMissingState'))

const FALLBACK_PROVIDERS: ProxyProvider[] = [
  { kind: 'embedded', mode: 'builtin', detected: false, selected: true },
  { kind: 'traefik', mode: 'managed', detected: false, selected: false },
  { kind: 'nginx', mode: 'managed', detected: false, selected: false },
  { kind: 'caddy', mode: 'manual', detected: false, selected: false },
  { kind: 'haproxy', mode: 'manual', detected: false, selected: false },
]

// The saved provider wins; while none is saved, the detected proxy is the
// useful starting point.
function defaultChoice(data: ProxyIntegration): ProxyProviderKind {
  if (data.provider && data.provider !== 'embedded') return data.provider
  const found = (data.providers ?? []).find((p) => p.detected)
  return found?.kind ?? 'embedded'
}

function QuietNote({ children }: { children: React.ReactNode }) {
  return <p className="px-1 text-xs text-muted-foreground">{children}</p>
}

function DirField({
  label,
  hint,
  error,
  value,
  onChange,
}: {
  label: string
  hint: string
  error?: string
  value: string
  onChange: (v: string) => void
}) {
  const id = useId()
  return (
    <div className="space-y-1">
      <Label htmlFor={id}>{label}</Label>
      <Input
        id={id}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        aria-invalid={error ? true : undefined}
        className="font-mono"
      />
      <p
        className={
          error ? 'text-xs text-destructive' : 'text-xs text-muted-foreground'
        }
      >
        {error ?? hint}
      </p>
    </div>
  )
}

// Network > Traffic card: pick the proxy provider, preview what it would
// change, apply it and verify every domain.
export function ProxyProviderCard() {
  const { t } = useTranslation('domains')
  const { data, isLoading, isError, isFetching, refetch, restartPolling } =
    useProxyIntegration()
  const isRoot = useIsRoot()
  const setup = useSetupProxyIntegration()
  const update = useUpdateProxySettings()
  const [choice, setChoice] = useState<ProxyProviderKind | null>(null)
  const [dir, setDir] = useState<string | null>(null)
  const [certDir, setCertDir] = useState('')
  const [result, setResult] = useState<ProxySetupResult | null>(null)
  const [failure, setFailure] = useState<ProxyFailure | null>(null)

  if (isLoading) return null
  if (isError) {
    return (
      <div className="flex items-center gap-3 px-1 text-xs text-muted-foreground">
        <span>{t('proxyPicker.loadFailed')}</span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          onClick={() => void refetch()}
        >
          {t('proxyPicker.retry')}
        </Button>
      </div>
    )
  }
  if (data === null || data === undefined) {
    return <QuietNote>{t('proxySetup.unavailable')}</QuietNote>
  }

  const providers = data.providers ?? FALLBACK_PROVIDERS
  const selected = choice ?? defaultChoice(data)
  const current = providers.find((p) => p.kind === selected)
  const managed = current?.mode === 'managed'
  const manual = current?.mode === 'manual'
  const embedded = selected === 'embedded'
  const missingProxy = !embedded && data.detected.kind !== selected && managed
  const pending = setup.isPending || update.isPending
  const showDir = managed && !data.detected.dynamic_dir && !missingProxy
  const dirValue = dir ?? manualDefaults(selected).dynamic_dir
  const showCertDir = selected === 'nginx' && !data.detected.cert_dir

  const run = (confirm: boolean) => {
    setup.mutate(
      {
        confirm,
        provider: selected,
        dynamic_dir: (showDir ? dirValue : (dir ?? '')).trim() || undefined,
        cert_dir: certDir.trim() || undefined,
      },
      {
        onSuccess: (res) => {
          setResult(res)
          setFailure(null)
          if (confirm && !res.manual) {
            restartPolling()
            toast.add({ title: t('proxyPicker.applyDone'), type: 'success' })
          }
        },
        onError: (error) => {
          setResult(null)
          setFailure({
            message: error.message,
            status: error instanceof ApiError ? error.status : 0,
            applying: confirm,
          })
          toast.add({
            title: confirm
              ? t('proxyPicker.applyFailed')
              : t('proxyPicker.dryRunFailed'),
            type: 'error',
          })
        },
      },
    )
  }

  const switchToEmbedded = () => {
    update.mutate(
      { ...data.settings, integration: 'off' },
      {
        onSuccess: () => {
          setChoice('embedded')
          restartPolling()
          toast.add({ title: t('proxyPicker.embeddedDone'), type: 'success' })
        },
        onError: (error) => {
          toast.add({
            title: t('proxyPicker.embeddedFailed'),
            description: error.message,
            type: 'error',
          })
        },
      },
    )
  }

  const saveManual = (settings: ProxySettings) => {
    update.mutate(settings, {
      onSuccess: () => {
        void refetch()
        restartPolling()
        toast.add({ title: t('proxyMissing.manual.saved'), type: 'success' })
      },
      onError: (error) => {
        toast.add({
          title: t('proxyMissing.manual.saveFailed'),
          description: error.message,
          type: 'error',
        })
      },
    })
  }

  const recheck = () => {
    void refetch()
    restartPolling()
  }

  const choose = (kind: ProxyProviderKind) => {
    setChoice(kind)
    setResult(null)
    setFailure(null)
  }

  const proxyName = t(`proxyPicker.provider.${selected}`)
  return (
    <Card className="min-w-0">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <SwapIcon className="size-4" aria-hidden="true" />
          {t('proxyPicker.title')}
        </CardTitle>
        <CardDescription>{t('proxyPicker.description')}</CardDescription>
      </CardHeader>
      <CardContent className="min-w-0 space-y-5">
        <ProxyProviderPicker
          providers={providers}
          value={selected}
          onChange={choose}
        />
        <p className="text-sm text-muted-foreground">
          {embedded
            ? t('proxyPicker.embeddedBody')
            : manual
              ? t('proxyPicker.manualBody', { proxy: proxyName })
              : t('proxyPicker.managedBody', { proxy: proxyName })}
        </p>
        {embedded ? null : (
          <div className="grid min-w-0 gap-6 lg:grid-cols-2">
            <ProxySteps steps={data.steps} />
            <ProxyDetectedFacts
              detected={data.detected}
              quietMissing={missingProxy}
            />
          </div>
        )}
        {missingProxy ? (
          <Suspense fallback={null}>
            <ProxyMissingState
              kind={selected}
              proxyName={proxyName}
              data={data}
              canChange={isRoot}
              busy={pending}
              checking={isFetching}
              onBackToBuiltIn={switchToEmbedded}
              onInstalled={(plan) => {
                setDir(plan.route_dir)
                recheck()
              }}
              onSaveManual={saveManual}
              onRecheck={recheck}
            />
          </Suspense>
        ) : null}
        {showDir ? (
          <DirField
            label={t('proxyPicker.inputs.dir')}
            hint={t('proxyPicker.inputs.dirHint')}
            error={
              isAbsoluteDir(dirValue)
                ? undefined
                : t('proxyMissing.manual.errors.absolute')
            }
            value={dirValue}
            onChange={setDir}
          />
        ) : null}
        {showCertDir ? (
          <DirField
            label={t('proxyPicker.inputs.certDir')}
            hint={t('proxyPicker.inputs.certDirHint')}
            value={certDir}
            onChange={setCertDir}
          />
        ) : null}
        <ProxyProviderResult result={result} failure={failure} />
        <div className="flex flex-wrap items-center gap-2">
          {embedded ? (
            <Button
              type="button"
              disabled={!isRoot || pending || data.provider === 'embedded'}
              onClick={switchToEmbedded}
            >
              {data.provider === 'embedded'
                ? t('proxyPicker.usingEmbedded')
                : t('proxyPicker.useEmbedded')}
            </Button>
          ) : (
            <>
              <Button
                type="button"
                variant="outline"
                disabled={!isRoot || pending || missingProxy}
                onClick={() => run(false)}
              >
                {setup.isPending && !setup.variables?.confirm
                  ? t('proxyPicker.dryRunning')
                  : t('proxyPicker.dryRun')}
              </Button>
              {manual ? null : (
                <Button
                  type="button"
                  disabled={
                    !isRoot ||
                    pending ||
                    missingProxy ||
                    (showDir && !isAbsoluteDir(dirValue))
                  }
                  aria-describedby={
                    missingProxy ? 'proxy-apply-why' : undefined
                  }
                  onClick={() => run(true)}
                >
                  {setup.isPending && setup.variables?.confirm ? (
                    <SpinnerIcon className="animate-spin" aria-hidden="true" />
                  ) : null}
                  {setup.isPending && setup.variables?.confirm
                    ? t('proxyPicker.applying')
                    : t('proxyPicker.apply')}
                </Button>
              )}
            </>
          )}
          <Button
            type="button"
            variant="ghost"
            disabled={isFetching}
            onClick={recheck}
          >
            {t('proxyPicker.recheck')}
          </Button>
        </div>
        {missingProxy && !embedded ? (
          <p id="proxy-apply-why" className="text-xs text-muted-foreground">
            {t('proxyPicker.applyWaiting', { proxy: proxyName })}
          </p>
        ) : null}
        {isRoot ? null : (
          <p className="text-xs text-muted-foreground">
            {t('proxyPicker.rootOnly')}
          </p>
        )}
        {embedded || manual ? null : (
          <Collapsible>
            <CollapsibleTrigger className="text-xs text-muted-foreground underline">
              {t('proxyPicker.fixViaCli')}
            </CollapsibleTrigger>
            <CollapsiblePanel>
              <code className="mt-1 block text-xs">
                {missingProxy
                  ? `proxy install --provider ${selected} --confirm`
                  : `proxy setup --provider ${selected} --confirm`}
              </code>
            </CollapsiblePanel>
          </Collapsible>
        )}
      </CardContent>
    </Card>
  )
}
