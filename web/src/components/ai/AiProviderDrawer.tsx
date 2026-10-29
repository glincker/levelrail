import { useId, useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  CircleCheckGlassIcon,
  ExternalLinkGlassIcon,
  EyeGlassIcon,
  EyeOffGlassIcon,
  LockGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { Alert, AlertDescription, AlertTitle } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { ConfirmDialog } from '@/components/ui/confirm-dialog'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
} from '@/components/ui/sheet'
import { toast } from '@/components/ui/toast'
import { BrandTile } from '../BrandLogoBadge'
import { aiProviderBrand } from '../../lib/providerBrands'
import {
  AiProviderError,
  useAiModels,
  useDisconnectAiProvider,
  useSaveAiProvider,
  useTestAiProvider,
  useUpdateAiRouting,
  type AiProvider,
  type AiTestResult,
} from '../../queries/aiProviders'
import type { AiAssistantConfig } from '../../queries/aiAssistantSettings'
import { AiModelPicker } from './AiModelPicker'
import { ProviderPill } from './AiProviderCard'
import { addFallback, checkDraft } from './aiProviderState'

interface DrawerProps {
  provider: AiProvider | null
  onOpenChange: (open: boolean) => void
  canWrite: boolean
  config: AiAssistantConfig | undefined
}

export function AiProviderDrawer({
  provider,
  onOpenChange,
  ...rest
}: DrawerProps) {
  return (
    <Sheet open={provider !== null} onOpenChange={onOpenChange}>
      <SheetContent className="w-full overflow-y-auto data-[side=right]:w-full data-[side=right]:sm:max-w-md">
        {provider ? (
          <DrawerBody
            key={provider.id}
            provider={provider}
            onClose={() => {
              onOpenChange(false)
            }}
            {...rest}
          />
        ) : null}
      </SheetContent>
    </Sheet>
  )
}

function DrawerBody({
  provider,
  canWrite,
  config,
  onClose,
}: Omit<DrawerProps, 'provider' | 'onOpenChange'> & {
  provider: AiProvider
  onClose: () => void
}) {
  const { t } = useTranslation('settings')
  const ids = useId()
  const [apiKey, setApiKey] = useState('')
  const [showKey, setShowKey] = useState(false)
  const [baseUrl, setBaseUrl] = useState(
    provider.base_url ?? provider.default_base_url,
  )
  const [model, setModel] = useState(provider.default_model)
  const [testResult, setTestResult] = useState<AiTestResult | null>(null)
  const [saved, setSaved] = useState(false)
  const [confirmOpen, setConfirmOpen] = useState(false)

  const save = useSaveAiProvider()
  const test = useTestAiProvider()
  const disconnect = useDisconnectAiProvider()
  const routing = useUpdateAiRouting()
  const models = useAiModels(provider.id, provider.has_key)

  const check = checkDraft(provider, {
    apiKey,
    baseUrl: provider.base_url_editable ? baseUrl : '',
    model,
  })
  const saveError = save.error instanceof AiProviderError ? save.error : null
  const fallback = config?.fallback ?? []
  const trimmedModel = model.trim() || provider.default_model

  function onSave() {
    setSaved(false)
    save.mutate(
      {
        id: provider.id,
        update: {
          ...(apiKey.trim() ? { api_key: apiKey.trim() } : {}),
          ...(provider.base_url_editable ? { base_url: baseUrl.trim() } : {}),
          default_model: model.trim(),
        },
      },
      {
        onSuccess: () => {
          setApiKey('')
          setShowKey(false)
          setSaved(true)
          setTestResult(null)
          toast.add({
            title: t('aiAssistantPage.drawer.saved', { name: provider.label }),
            type: 'success',
          })
        },
      },
    )
  }

  function onTest() {
    test.mutate(provider.id, {
      onSuccess: setTestResult,
      onError: (e) => {
        setTestResult({
          ok: false,
          status: 'unreachable',
          status_detail: e.message,
        })
      },
    })
  }

  function onDisconnect() {
    disconnect.mutate(provider.id, {
      onSuccess: () => {
        setConfirmOpen(false)
        toast.add({
          title: t('aiAssistantPage.drawer.disconnected', {
            name: provider.label,
          }),
          type: 'success',
        })
        onClose()
      },
    })
  }

  const keyHelpId = `${ids}-key-help`
  const testOk = testResult?.ok === true

  return (
    <>
      <SheetHeader className="pr-12">
        <div className="flex items-center gap-3">
          <BrandTile
            name={aiProviderBrand(provider.brand)}
            label={provider.label}
            size="md"
          />
          <div className="min-w-0 space-y-1">
            <SheetTitle>{provider.label}</SheetTitle>
            <ProviderPill provider={provider} />
          </div>
        </div>
        <SheetDescription>
          {t('aiAssistantPage.drawer.description')}
        </SheetDescription>
      </SheetHeader>

      <div className="space-y-4 px-4">
        {!canWrite ? (
          <Alert>
            <LockGlassIcon size="1.2em" aria-hidden="true" />
            <AlertDescription>{t('aiAssistantPage.rootOnly')}</AlertDescription>
          </Alert>
        ) : null}

        {provider.status_detail && provider.has_key ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
              aria-hidden="true"
            />
            <AlertDescription>{provider.status_detail}</AlertDescription>
          </Alert>
        ) : null}

        <Field>
          <FieldLabel htmlFor={`${ids}-key`}>
            {t('aiAssistantPage.drawer.apiKey')}
          </FieldLabel>
          <div className="flex gap-2">
            <Input
              id={`${ids}-key`}
              type={showKey ? 'text' : 'password'}
              autoComplete="new-password"
              spellCheck={false}
              disabled={!canWrite}
              value={apiKey}
              aria-describedby={keyHelpId}
              placeholder={
                provider.has_key
                  ? t('aiAssistantPage.drawer.keySavedPlaceholder')
                  : t('aiAssistantPage.drawer.keyPlaceholder')
              }
              onChange={(e) => {
                setApiKey(e.target.value)
              }}
            />
            <Button
              variant="outline"
              size="icon"
              aria-pressed={showKey}
              aria-label={
                showKey
                  ? t('aiAssistantPage.drawer.hideKey')
                  : t('aiAssistantPage.drawer.showKey')
              }
              onClick={() => {
                setShowKey((v) => !v)
              }}
            >
              {showKey ? (
                <EyeOffGlassIcon size="1.2em" />
              ) : (
                <EyeGlassIcon size="1.2em" />
              )}
            </Button>
          </div>
          <FieldDescription id={keyHelpId}>
            {provider.has_key
              ? t('aiAssistantPage.drawer.keyKept')
              : t('aiAssistantPage.drawer.keyNever')}
            {provider.key_url ? (
              <>
                {' '}
                <a
                  href={provider.key_url}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="inline-flex items-center gap-1 text-brand underline-offset-4 hover:underline"
                >
                  {t('aiAssistantPage.drawer.getKey')}
                  <ExternalLinkGlassIcon
                    className="size-3.5"
                    aria-hidden="true"
                  />
                </a>
              </>
            ) : null}
          </FieldDescription>
        </Field>

        {provider.base_url_editable ? (
          <Field>
            <FieldLabel htmlFor={`${ids}-base`}>
              {t('aiAssistantPage.drawer.baseUrl')}
            </FieldLabel>
            <Input
              id={`${ids}-base`}
              inputMode="url"
              disabled={!canWrite}
              value={baseUrl}
              aria-invalid={check.baseUrlInvalid}
              placeholder={provider.default_base_url}
              onChange={(e) => {
                setBaseUrl(e.target.value)
              }}
            />
            <FieldDescription>
              {check.baseUrlInvalid
                ? t('aiAssistantPage.drawer.baseUrlInvalid')
                : t('aiAssistantPage.drawer.baseUrlNote')}
            </FieldDescription>
          </Field>
        ) : null}

        <Field>
          <FieldLabel htmlFor={`${ids}-model`}>
            {t('aiAssistantPage.drawer.model')}
          </FieldLabel>
          <AiModelPicker
            id={`${ids}-model`}
            value={model}
            onChange={setModel}
            models={models.data ?? []}
            loading={models.isFetching}
            disabled={!canWrite}
            placeholder={provider.default_model}
          />
          <FieldDescription>
            {models.isError
              ? t('aiAssistantPage.drawer.modelFree')
              : t('aiAssistantPage.drawer.modelHelp')}
          </FieldDescription>
        </Field>

        {saveError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
              aria-hidden="true"
            />
            <AlertTitle>{saveError.message}</AlertTitle>
            {saveError.fix ? (
              <AlertDescription>{saveError.fix}</AlertDescription>
            ) : null}
          </Alert>
        ) : save.isError ? (
          <Alert variant="destructive">
            <TriangleAlertGlassIcon
              className="text-tone-warning"
              size="1.2em"
              aria-hidden="true"
            />
            <AlertTitle>{save.error.message}</AlertTitle>
          </Alert>
        ) : null}

        <div className="flex flex-wrap items-center gap-3">
          <Button
            variant="outline"
            disabled={
              !canWrite ||
              !provider.has_key ||
              apiKey.trim() !== '' ||
              test.isPending
            }
            onClick={onTest}
          >
            {test.isPending
              ? t('aiAssistantPage.drawer.testing')
              : t('aiAssistantPage.drawer.test')}
          </Button>
          <p
            role="status"
            aria-live="polite"
            className="flex min-w-0 items-center gap-1.5 text-sm"
          >
            {testResult ? (
              <>
                {testOk ? (
                  <CircleCheckGlassIcon
                    className="size-4.5 shrink-0 text-tone-success transition-[transform,opacity] duration-300 motion-reduce:transition-none starting:scale-50 starting:opacity-0"
                    aria-hidden="true"
                  />
                ) : (
                  <TriangleAlertGlassIcon
                    className="size-4.5 shrink-0 text-tone-danger"
                    aria-hidden="true"
                  />
                )}
                <span>
                  {t(`aiAssistantPage.test.${testResult.status}`)}
                  {testResult.latency_ms !== undefined && testOk
                    ? ` (${t('aiAssistantPage.test.latency', { ms: testResult.latency_ms })})`
                    : ''}
                  {testResult.status_detail
                    ? `: ${testResult.status_detail}`
                    : ''}
                </span>
              </>
            ) : null}
          </p>
        </div>

        {provider.has_key ? (
          <div className="flex flex-wrap gap-2 border-t border-border pt-4">
            <Button
              variant="secondary"
              disabled={!canWrite || routing.isPending || !trimmedModel}
              onClick={() => {
                routing.mutate(
                  { active: { provider: provider.id, model: trimmedModel } },
                  {
                    onSuccess: () => {
                      toast.add({
                        title: t('aiAssistantPage.drawer.nowActive', {
                          name: provider.label,
                        }),
                        type: 'success',
                      })
                    },
                  },
                )
              }}
            >
              {t('aiAssistantPage.drawer.useModel')}
            </Button>
            <Button
              variant="outline"
              disabled={!canWrite || routing.isPending || !trimmedModel}
              onClick={() => {
                routing.mutate({
                  fallback: addFallback(fallback, config?.active, {
                    provider: provider.id,
                    model: trimmedModel,
                  }),
                })
              }}
            >
              {t('aiAssistantPage.drawer.addFallback')}
            </Button>
            {routing.isError ? (
              <p role="alert" className="w-full text-sm text-destructive">
                {routing.error.message}
              </p>
            ) : null}
          </div>
        ) : null}
      </div>

      <SheetFooter className="flex-col sm:flex-row sm:justify-between">
        <Button
          disabled={!canWrite || !check.valid || save.isPending}
          onClick={onSave}
        >
          {save.isPending
            ? t('aiAssistantPage.drawer.saving')
            : saved
              ? t('aiAssistantPage.drawer.savedShort')
              : t('aiAssistantPage.drawer.save')}
        </Button>
        {provider.has_key ? (
          <Button
            variant="destructive"
            disabled={!canWrite}
            onClick={() => {
              setConfirmOpen(true)
            }}
          >
            {t('aiAssistantPage.drawer.disconnect')}
          </Button>
        ) : null}
      </SheetFooter>

      <ConfirmDialog
        open={confirmOpen}
        onOpenChange={setConfirmOpen}
        tone="destructive"
        title={t('aiAssistantPage.drawer.disconnectTitle', {
          name: provider.label,
        })}
        description={t('aiAssistantPage.drawer.disconnectBody')}
        confirmLabel={t('aiAssistantPage.drawer.disconnect')}
        pending={disconnect.isPending}
        error={disconnect.isError ? disconnect.error.message : null}
        onConfirm={onDisconnect}
      />
    </>
  )
}
