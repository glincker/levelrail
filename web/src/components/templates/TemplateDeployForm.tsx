import { useState } from 'react'
import { DraftBanner } from '../DraftBanner'
import { useDraft } from '../../hooks/useDraft'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  ChevronDownGlassIcon,
  ChevronRightGlassIcon,
  RocketGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { useInstallTemplate } from '../../queries/composeStack'
import {
  buildInstallPayload,
  groupEnvPrompts,
  validateTemplateForm,
  type FormErrors,
  type FormValues,
} from '../../lib/templateForm'
import { appSlug } from '../../lib/templateSummary'
import type { TemplateEnvPrompt, TemplateManifest } from '../../types/compose'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { Switch } from '@/components/ui/switch'
import { toast } from '@/components/ui/toast'
import { TemplateGeneratedField } from './TemplateGeneratedField'

function EnvField({
  prompt,
  value,
  error,
  onChange,
}: {
  prompt: TemplateEnvPrompt
  value: string
  error?: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation('templates')
  const id = `template-env-${prompt.key}`
  const required = !!error || (prompt.required && !prompt.default)
  return (
    <Field>
      <FieldLabel htmlFor={id}>
        {prompt.label ?? prompt.key}
        {required ? (
          <span className="text-destructive" aria-hidden="true">
            *
          </span>
        ) : null}
      </FieldLabel>
      <Input
        id={id}
        type={prompt.secret ? 'password' : 'text'}
        placeholder={prompt.default}
        aria-required={required}
        aria-invalid={!!error}
        aria-describedby={error ? `${id}-error` : undefined}
        value={value}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      />
      {error ? (
        <FieldError id={`${id}-error`}>
          {t('form.errors.required', { field: prompt.label ?? prompt.key })}
        </FieldError>
      ) : prompt.description ? (
        <FieldHint>{prompt.description}</FieldHint>
      ) : null}
    </Field>
  )
}

/** Inline install form for a manifest template: required first, generated secrets, optional collapsed, one Deploy button. */
export function TemplateDeployForm({
  manifest,
}: {
  manifest: TemplateManifest
}) {
  const { t } = useTranslation('templates')
  const navigate = useNavigate()
  const install = useInstallTemplate(manifest.id)
  const prompts = manifest.env ?? []
  const groups = groupEnvPrompts(prompts)
  const draft = useDraft<FormValues>(
    `template-deploy:${manifest.id}`,
    { name: '', env: {}, customDomain: false, domains: {} },
    { omit: prompts.filter((p) => p.secret).map((p) => p.key) },
  )
  const values = draft.value
  const setValues = draft.setValue
  const [errors, setErrors] = useState<FormErrors>({})
  const [optionalOpen, setOptionalOpen] = useState(false)
  const [attempted, setAttempted] = useState(false)

  const update = (next: FormValues) => {
    setValues(next)
    if (attempted) setErrors(validateTemplateForm(prompts, next))
  }
  const setEnv = (key: string, value: string) => {
    update({ ...values, env: { ...values.env, [key]: value } })
  }

  const submit = () => {
    const found = validateTemplateForm(prompts, values)
    setAttempted(true)
    setErrors(found)
    if (Object.keys(found).length > 0) {
      if (found.name !== undefined) setOptionalOpen(true)
      return
    }
    install.mutate(buildInstallPayload(values), {
      onSuccess: (res) => {
        draft.clearOnSuccess()
        toast.add({
          title: t('form.success', { app: res.app_id }),
          type: 'success',
        })
        const first = res.services[0]
        if (first) {
          void navigate({
            to: '/apps/$name/services',
            params: { name: first.name },
          })
        }
      },
    })
  }

  const domains = manifest.domains ?? []
  const optionalCount = groups.optional.length + 1
  const nameError = errors.name !== undefined
  const customHosts = Object.values(values.domains).filter((h) => h)
  const summary = t('form.summary', {
    name: manifest.name,
    app: values.name || appSlug(manifest.id, manifest.name),
    address:
      values.customDomain && customHosts.length > 0
        ? customHosts.join(', ')
        : t('form.summaryAuto'),
    generated: groups.generated.length,
  })

  return (
    <form
      noValidate
      className="space-y-5 rounded-xl border border-border bg-card p-4 sm:p-5"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <DraftBanner draft={draft} />
      <div className="space-y-1">
        <h2 className="text-base font-semibold text-foreground">
          {t('form.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {groups.required.length > 0
            ? t('form.descriptionRequired')
            : t('form.descriptionNone')}
        </p>
      </div>

      {groups.required.map((p) => (
        <EnvField
          key={p.key}
          prompt={p}
          value={values.env[p.key] ?? ''}
          error={errors[`env:${p.key}`]}
          onChange={(v) => {
            setEnv(p.key, v)
          }}
        />
      ))}

      {groups.generated.length > 0 ? (
        <section className="space-y-3" aria-labelledby="template-generated">
          <h3
            id="template-generated"
            className="text-sm font-medium text-foreground"
          >
            {t('form.generated.title')}
          </h3>
          {groups.generated.map((p) => (
            <TemplateGeneratedField
              key={p.key}
              prompt={p}
              value={values.env[p.key] ?? ''}
              onChange={(v) => {
                setEnv(p.key, v)
              }}
            />
          ))}
        </section>
      ) : null}

      <section className="space-y-3" aria-labelledby="template-domain">
        <h3
          id="template-domain"
          className="text-sm font-medium text-foreground"
        >
          {t('form.domain.title')}
        </h3>
        <div className="flex items-start gap-3">
          <Switch
            id="template-custom-domain"
            checked={values.customDomain}
            disabled={domains.length === 0}
            onCheckedChange={(checked) => {
              update({ ...values, customDomain: checked })
            }}
            aria-label={t('form.domain.custom')}
          />
          <label
            htmlFor="template-custom-domain"
            className="min-w-0 text-sm text-muted-foreground"
          >
            <span className="block font-medium text-foreground">
              {values.customDomain
                ? t('form.domain.custom')
                : t('form.domain.auto')}
            </span>
            {domains.length === 0
              ? t('form.domain.none')
              : values.customDomain
                ? t('form.domain.customHint')
                : t('form.domain.autoHint')}
          </label>
        </div>
        {values.customDomain
          ? domains.map((d) => {
              const id = `template-domain-${d.service}`
              const error = errors[`domain:${d.service}`]
              return (
                <Field key={d.service}>
                  <FieldLabel htmlFor={id}>
                    {t('form.domain.label', {
                      service: d.service,
                      port: d.port,
                    })}
                  </FieldLabel>
                  <Input
                    id={id}
                    placeholder={t('form.domain.placeholder')}
                    aria-invalid={!!error}
                    value={values.domains[d.service] ?? ''}
                    onChange={(e) => {
                      update({
                        ...values,
                        domains: {
                          ...values.domains,
                          [d.service]: e.target.value.trim().toLowerCase(),
                        },
                      })
                    }}
                  />
                  {error ? (
                    <FieldError>{t('form.errors.domain')}</FieldError>
                  ) : (
                    <FieldHint>
                      {d.description ?? t('form.domain.hint')}
                    </FieldHint>
                  )}
                </Field>
              )
            })
          : null}
      </section>

      <section className="space-y-3">
        <button
          type="button"
          aria-expanded={optionalOpen}
          aria-controls="template-optional"
          className="flex w-full items-center gap-1.5 rounded-md text-left text-sm font-medium text-foreground focus-visible:ring-3 focus-visible:ring-ring/50 focus-visible:outline-none"
          onClick={() => {
            setOptionalOpen(!optionalOpen)
          }}
        >
          {optionalOpen ? (
            <ChevronDownGlassIcon className="size-5" aria-hidden="true" />
          ) : (
            <ChevronRightGlassIcon className="size-5" aria-hidden="true" />
          )}
          {t('form.optional.title')}
          <span className="font-normal text-muted-foreground">
            {t('form.optional.count', { count: optionalCount })}
          </span>
        </button>
        <div
          id="template-optional"
          hidden={!optionalOpen}
          className="min-w-0 space-y-4"
        >
          <Field>
            <FieldLabel htmlFor="template-install-name">
              {t('form.name.label')}
            </FieldLabel>
            <Input
              id="template-install-name"
              placeholder={appSlug(manifest.id, manifest.name)}
              aria-invalid={nameError}
              value={values.name}
              onChange={(e) => {
                update({ ...values, name: e.target.value })
              }}
            />
            {nameError ? (
              <FieldError>{t('form.errors.name')}</FieldError>
            ) : (
              <FieldHint>{t('form.name.hint')}</FieldHint>
            )}
          </Field>
          {groups.optional.map((p) => (
            <EnvField
              key={p.key}
              prompt={p}
              value={values.env[p.key] ?? ''}
              onChange={(v) => {
                setEnv(p.key, v)
              }}
            />
          ))}
        </div>
      </section>

      {install.isError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.4em" />
          <AlertDescription>{install.error.message}</AlertDescription>
        </Alert>
      ) : null}
      {attempted && Object.keys(errors).length > 0 ? (
        <p role="alert" className="text-sm text-destructive">
          {t('form.errors.summary', { count: Object.keys(errors).length })}
        </p>
      ) : null}

      <div className="flex flex-wrap items-center gap-3 border-t border-border pt-4">
        <p
          className="min-w-0 flex-1 basis-56 text-sm text-muted-foreground"
          aria-live="polite"
        >
          {summary}
        </p>
        <Button type="submit" disabled={install.isPending}>
          <RocketGlassIcon size="1.4em" aria-hidden="true" />
          {install.isPending ? t('form.deploying') : t('form.deploy')}
        </Button>
      </div>
    </form>
  )
}
