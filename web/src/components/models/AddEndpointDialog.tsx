import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { PlugGlassIcon } from '@thesvg/ui-react'
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from '@/components/ui/dialog'
import { Button } from '@/components/ui/button'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import {
  endpointFormErrors,
  inferApiType,
  suggestEndpointName,
} from '../../lib/modelEndpointForm'
import { useAddEndpoint, useProbeEndpoint } from '../../queries/modelEndpoints'
import type { CreateModelResponse } from '../../types/models'
import type { EndpointApiType } from '../../types/modelEndpoints'
import { EndpointFix } from './EndpointFix'
import { EndpointHealthBadge } from './EndpointHealthBadge'

const API_TYPES: EndpointApiType[] = ['ollama', 'openai']

export function AddEndpointDialog({
  onAdded,
}: {
  onAdded: (created: CreateModelResponse) => void
}) {
  const { t } = useTranslation('models')
  const [open, setOpen] = useState(false)
  const [url, setUrl] = useState('')
  const [name, setName] = useState('')
  const [apiType, setApiType] = useState<EndpointApiType | null>(null)
  const [credential, setCredential] = useState('')
  const [submitted, setSubmitted] = useState(false)
  const add = useAddEndpoint()
  const probe = useProbeEndpoint()
  const effectiveApi = apiType ?? inferApiType(url)
  const effectiveName = name.trim() || suggestEndpointName(url)
  const errors = endpointFormErrors({ name, url })
  const shown = submitted ? errors : {}

  function reset() {
    setUrl('')
    setName('')
    setApiType(null)
    setCredential('')
    setSubmitted(false)
    add.reset()
    probe.reset()
  }

  function test() {
    setSubmitted(true)
    if (errors.url) return
    probe.mutate({ baseUrl: url.trim(), apiType: effectiveApi, credential })
  }

  function submit() {
    setSubmitted(true)
    if (errors.url || errors.name) return
    add.mutate(
      {
        name: effectiveName,
        baseUrl: url.trim(),
        apiType: effectiveApi,
        credential,
      },
      {
        onSuccess: (created) => {
          toast.add({
            title: t('endpoint.toast.added', { name: created.name }),
            type: 'success',
          })
          setOpen(false)
          reset()
          onAdded(created)
        },
      },
    )
  }

  const result = probe.data
  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next)
        if (!next) reset()
      }}
    >
      <DialogTrigger render={<Button size="sm" variant="outline" />}>
        <PlugGlassIcon size="1.4em" aria-hidden="true" />
        {t('endpoint.add.trigger')}
      </DialogTrigger>
      <DialogContent className="sm:max-w-xl">
        <DialogHeader>
          <DialogTitle>{t('endpoint.add.title')}</DialogTitle>
          <DialogDescription>{t('endpoint.add.description')}</DialogDescription>
        </DialogHeader>
        <form
          noValidate
          className="space-y-4"
          onSubmit={(e) => {
            e.preventDefault()
            submit()
          }}
        >
          <Field>
            <FieldLabel htmlFor="endpoint-url">
              {t('endpoint.fields.url')}
            </FieldLabel>
            <Input
              id="endpoint-url"
              value={url}
              onChange={(e) => {
                setUrl(e.target.value)
              }}
              placeholder={t('endpoint.fields.urlPlaceholder')}
              autoComplete="off"
              className="font-mono"
              aria-invalid={shown.url === true}
            />
            {shown.url ? (
              <p role="alert" className="text-xs text-destructive">
                {t('endpoint.errors.url')}
              </p>
            ) : (
              <FieldHint>{t('endpoint.fields.urlHint')}</FieldHint>
            )}
          </Field>
          <fieldset className="min-w-0">
            <legend className="mb-1.5 text-sm font-medium text-foreground">
              {t('endpoint.fields.apiType')}
            </legend>
            <div className="grid gap-2 sm:grid-cols-2">
              {API_TYPES.map((type) => (
                <label
                  key={type}
                  className="flex cursor-pointer items-start gap-2 rounded-lg border border-border bg-card p-3 text-sm has-checked:border-primary has-checked:bg-primary/5 has-focus-visible:ring-3 has-focus-visible:ring-ring/50"
                >
                  <input
                    type="radio"
                    name="endpoint-api-type"
                    value={type}
                    checked={effectiveApi === type}
                    onChange={() => {
                      setApiType(type)
                    }}
                    className="sr-only"
                  />
                  <span>
                    <span className="block font-medium text-foreground">
                      {t(`endpoint.apiType.${type}.label`)}
                    </span>
                    <span className="block text-xs text-muted-foreground">
                      {t(`endpoint.apiType.${type}.hint`)}
                    </span>
                  </span>
                </label>
              ))}
            </div>
          </fieldset>
          <div className="grid gap-3 sm:grid-cols-2">
            <Field>
              <FieldLabel htmlFor="endpoint-name">
                {t('endpoint.fields.name')}
              </FieldLabel>
              <Input
                id="endpoint-name"
                value={name}
                onChange={(e) => {
                  setName(e.target.value)
                }}
                placeholder={
                  suggestEndpointName(url) ||
                  t('endpoint.fields.namePlaceholder')
                }
                autoComplete="off"
                aria-invalid={shown.name === true}
              />
              {shown.name ? (
                <p role="alert" className="text-xs text-destructive">
                  {t('endpoint.errors.name')}
                </p>
              ) : (
                <FieldHint>{t('endpoint.fields.nameHint')}</FieldHint>
              )}
            </Field>
            <Field>
              <FieldLabel htmlFor="endpoint-credential">
                {t('endpoint.fields.credential')}
              </FieldLabel>
              <Input
                id="endpoint-credential"
                type="password"
                value={credential}
                onChange={(e) => {
                  setCredential(e.target.value)
                }}
                autoComplete="off"
                placeholder={t('endpoint.fields.credentialPlaceholder')}
              />
              <FieldHint>{t('endpoint.fields.credentialHint')}</FieldHint>
            </Field>
          </div>
          {result ? (
            <section
              aria-label={t('endpoint.testResult')}
              aria-live="polite"
              className="space-y-2"
            >
              <div className="flex flex-wrap items-center gap-2">
                <EndpointHealthBadge
                  state={result.state}
                  latencyMs={result.latency_ms}
                />
                {result.models.length > 0 ? (
                  <span className="text-xs text-muted-foreground">
                    {t('endpoint.modelsFound', { count: result.models.length })}
                  </span>
                ) : null}
              </div>
              {result.models.length > 0 ? (
                <p className="truncate font-mono text-xs text-muted-foreground">
                  {result.models.join(', ')}
                </p>
              ) : null}
              {result.diagnosis ? (
                <EndpointFix
                  diagnosis={result.diagnosis}
                  apiType={effectiveApi}
                  onRecheck={test}
                  rechecking={probe.isPending}
                />
              ) : null}
              {result.diagnosis ? (
                <p className="text-xs text-muted-foreground">
                  {t('endpoint.add.registerAnyway')}
                </p>
              ) : null}
            </section>
          ) : null}
          {probe.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {probe.error.message}
            </p>
          ) : null}
          {add.isError ? (
            <p role="alert" className="text-sm text-destructive">
              {add.error.message}
            </p>
          ) : null}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              disabled={probe.isPending}
              onClick={test}
            >
              {probe.isPending
                ? t('endpoint.testing')
                : t('endpoint.add.testConnection')}
            </Button>
            <Button type="submit" disabled={add.isPending}>
              {add.isPending
                ? t('endpoint.add.adding')
                : t('endpoint.add.submit')}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
