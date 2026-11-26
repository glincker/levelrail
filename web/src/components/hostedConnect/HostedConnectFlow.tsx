import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { ArrowLeftGlassIcon } from '@thesvg/ui-react'
import {
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { toast } from '@/components/ui/toast'
import { ApiError } from '@/lib/apiError'
import {
  GENERIC_PROVIDER,
  applyParsed,
  formFromProvider,
  hostedRequest,
  probeReached,
  suggestHostedName,
  type HostedForm,
} from '@/lib/hostedConnect'
import {
  HostedProbeBlockedError,
  useConnectHostedDatabase,
  useExternalProviders,
  useParseExternalUrl,
  useTestExternalDatabase,
} from '../../queries/externalDatabases'
import type { ExternalProbeResult } from '../../types/externalDatabase'
import { DEFAULT_PORT } from '../connectDatabaseForm'
import { HostedDetailsStep, type ParseState } from './HostedDetailsStep'
import { HostedProviderGallery } from './HostedProviderGallery'
import { HostedTestStep } from './HostedTestStep'

type Step = 'provider' | 'details' | 'test'

const STEP_NUMBER: Record<Step, number> = { provider: 1, details: 2, test: 3 }

// Three steps: provider, details, test then save. A failed probe stores
// nothing unless the owner ticks save anyway.
export function HostedConnectFlow({
  onClose,
}: Readonly<{ onClose: () => void }>) {
  const { t } = useTranslation('databases')
  const navigate = useNavigate()
  const providers = useExternalProviders(true)
  const parse = useParseExternalUrl()
  const test = useTestExternalDatabase()
  const connect = useConnectHostedDatabase()

  const [step, setStep] = useState<Step>('provider')
  const [providerId, setProviderId] = useState(GENERIC_PROVIDER)
  const [form, setForm] = useState<HostedForm>(() => formFromProvider(null))
  const [url, setUrl] = useState('')
  const [lastParsed, setLastParsed] = useState('')
  const [nameTouched, setNameTouched] = useState(false)
  const [parseState, setParseState] = useState<ParseState>('idle')
  const [parseError, setParseError] = useState('')
  const [detected, setDetected] = useState<string | null>(null)
  const [probe, setProbe] = useState<ExternalProbeResult | null>(null)
  const [blocked, setBlocked] = useState(false)
  const [saveAnyway, setSaveAnyway] = useState(false)
  const [error, setError] = useState<string | null>(null)

  const list = providers.data ?? []
  const provider = list.find((p) => p.id === providerId) ?? null
  const providerLabel = provider?.label ?? t('external.hosted.providerOther')
  const failed = blocked || (probe !== null && !probeReached(probe))
  const busy = test.isPending || connect.isPending

  function clearResult() {
    setProbe(null)
    setBlocked(false)
    setSaveAnyway(false)
    setError(null)
  }

  function patch(p: Partial<HostedForm>) {
    setForm((f) => ({ ...f, ...p }))
    clearResult()
  }

  function pick(id: string) {
    const next = list.find((p) => p.id === id) ?? null
    setProviderId(id)
    setForm(formFromProvider(next))
    setNameTouched(false)
    setUrl('')
    setLastParsed('')
    setParseState('idle')
    setDetected(null)
    clearResult()
    setStep('details')
  }

  function onEngine(engine: HostedForm['engine']) {
    patch({
      engine,
      port: String(DEFAULT_PORT[engine]),
    })
  }

  function onHost(host: string) {
    patch({
      host,
      ...(nameTouched ? {} : { name: suggestHostedName(providerId, host) }),
    })
  }

  function commitUrl(raw: string) {
    const value = raw.trim()
    if (value === '' || value === lastParsed) return
    setLastParsed(value)
    setUrl(value)
    setParseState('parsing')
    parse.mutate(value, {
      onSuccess: (parsed) => {
        setForm((f) => applyParsed(f, parsed, nameTouched))
        if (
          providerId === GENERIC_PROVIDER &&
          parsed.provider !== GENERIC_PROVIDER
        ) {
          setProviderId(parsed.provider)
          setDetected(list.find((p) => p.id === parsed.provider)?.label ?? null)
        }
        setParseState('parsed')
        clearResult()
      },
      onError: (e) => {
        setParseError(e.message)
        setParseState('failed')
      },
    })
  }

  function describe(e: Error): string {
    if (e instanceof ApiError && e.status === 429) {
      return t('external.hosted.busy')
    }
    if (e instanceof ApiError && e.status === 403) {
      return e.message || t('external.hosted.forbidden')
    }
    return e.message
  }

  function runTest() {
    clearResult()
    test.mutate(hostedRequest(form, providerId, url, false), {
      onSuccess: setProbe,
      onError: (e) => {
        setError(describe(e))
      },
    })
  }

  function save() {
    setError(null)
    connect.mutate(hostedRequest(form, providerId, url, saveAnyway), {
      onSuccess: (created) => {
        onClose()
        toast.add({
          title: t('external.hosted.toast', { name: created.name }),
          type: 'success',
        })
        void navigate({
          to: '/databases/$name/overview',
          params: { name: created.name },
        })
      },
      onError: (e) => {
        if (e instanceof HostedProbeBlockedError) {
          setBlocked(true)
          if (e.probe) setProbe(e.probe)
          return
        }
        setError(describe(e))
      },
    })
  }

  const canContinue =
    form.name !== '' && (form.host !== '' || url.trim() !== '')
  const canSave = !busy && (!failed || saveAnyway)

  return (
    <>
      <DialogHeader>
        <DialogTitle>{t('external.hosted.title')}</DialogTitle>
        <DialogDescription>
          {t('external.hosted.description')}{' '}
          <span className="text-xs">
            {t('external.hosted.step', {
              current: STEP_NUMBER[step],
              total: 3,
            })}
          </span>
        </DialogDescription>
      </DialogHeader>

      <div className="min-h-0 flex-1 overflow-y-auto px-px py-1">
        {step === 'provider' ? (
          <HostedProviderGallery
            providers={list}
            loading={providers.isLoading}
            failed={providers.isError}
            onPick={pick}
          />
        ) : null}
        {step === 'details' ? (
          <HostedDetailsStep
            provider={provider}
            providerLabel={providerLabel}
            detected={detected}
            form={form}
            url={url}
            parseState={parseState}
            parseError={parseError}
            patch={patch}
            onEngine={onEngine}
            onHost={onHost}
            onUrl={(v) => {
              setUrl(v)
              clearResult()
            }}
            onUrlCommit={commitUrl}
            onNameEdit={() => {
              setNameTouched(true)
            }}
          />
        ) : null}
        {step === 'test' ? (
          <HostedTestStep
            form={form}
            probe={probe}
            testing={test.isPending}
            failed={failed}
            error={error}
            saveAnyway={saveAnyway}
            onSaveAnyway={setSaveAnyway}
            onTest={runTest}
          />
        ) : null}
      </div>

      <DialogFooter>
        {step === 'provider' ? null : (
          <Button
            type="button"
            variant="ghost"
            onClick={() => {
              setStep(step === 'test' ? 'details' : 'provider')
            }}
          >
            <ArrowLeftGlassIcon className="size-5" aria-hidden="true" />
            {t('external.hosted.back')}
          </Button>
        )}
        <Button type="button" variant="outline" onClick={onClose}>
          {t('external.hosted.cancel')}
        </Button>
        {step === 'details' ? (
          <Button
            type="button"
            disabled={!canContinue || parseState === 'parsing'}
            onClick={() => {
              setStep('test')
            }}
          >
            {t('external.hosted.next')}
          </Button>
        ) : null}
        {step === 'test' ? (
          <Button type="button" disabled={!canSave} onClick={save}>
            {connect.isPending
              ? t('external.hosted.saving')
              : t('external.hosted.save')}
          </Button>
        ) : null}
      </DialogFooter>
    </>
  )
}
