import { useState } from 'react'
import { useNavigate } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import {
  KeyGlassIcon,
  RocketGlassIcon,
  TriangleAlertGlassIcon,
} from '@thesvg/ui-react'
import { useInstallTemplate } from '../queries/composeStack'
import type { TemplateManifest } from '../types/compose'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'
import { toast } from '@/components/ui/toast'

const NAME_PATTERN = /^[a-z][a-z0-9-]{0,39}$/

// Install form for a manifest template: one field per env prompt (generated
// ones are shown, not asked), one optional domain per offered service.
export function TemplateInstallForm({
  manifest,
}: {
  manifest: TemplateManifest
}) {
  const { t } = useTranslation('compose')
  const navigate = useNavigate()
  const install = useInstallTemplate(manifest.id)
  const [name, setName] = useState('')
  const [env, setEnv] = useState<Record<string, string>>({})
  const [domains, setDomains] = useState<Record<string, string>>({})
  const [error, setError] = useState<string | null>(null)

  const prompts = manifest.env ?? []
  const asked = prompts.filter((p) => !p.generate)
  const generated = prompts.filter((p) => p.generate)

  const submit = () => {
    if (name && !NAME_PATTERN.test(name)) {
      setError(t('install.nameHint'))
      return
    }
    const missing = asked.find((p) => p.required && !p.default && !env[p.key])
    if (missing) {
      setError(
        t('install.requiredMissing', { field: missing.label ?? missing.key }),
      )
      return
    }
    setError(null)
    const pick = (m: Record<string, string>) =>
      Object.fromEntries(Object.entries(m).filter(([, v]) => v.trim() !== ''))
    install.mutate(
      { name: name || undefined, env: pick(env), domains: pick(domains) },
      {
        onSuccess: (res) => {
          toast.add({
            title: t('install.success', { app: res.app_id }),
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
      },
    )
  }

  return (
    <form
      className="space-y-4 rounded-lg border border-border bg-card p-4"
      onSubmit={(e) => {
        e.preventDefault()
        submit()
      }}
    >
      <div className="space-y-1">
        <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight) text-foreground">
          {t('install.title')}
        </h2>
        <p className="text-sm text-muted-foreground">
          {t('install.description')}
        </p>
      </div>
      <Field>
        <FieldLabel htmlFor="template-install-name">
          {t('install.nameLabel')}
        </FieldLabel>
        <Input
          id="template-install-name"
          placeholder={t('install.namePlaceholder')}
          value={name}
          onChange={(e) => {
            setName(e.target.value)
          }}
        />
        <FieldHint>{t('install.nameHint')}</FieldHint>
      </Field>
      {asked.map((p) => (
        <Field key={p.key}>
          <FieldLabel htmlFor={`template-env-${p.key}`}>
            {p.label ?? p.key}
          </FieldLabel>
          <Input
            id={`template-env-${p.key}`}
            type={p.secret ? 'password' : 'text'}
            placeholder={p.default}
            aria-required={p.required && !p.default}
            value={env[p.key] ?? ''}
            onChange={(e) => {
              setEnv({ ...env, [p.key]: e.target.value })
            }}
          />
          {p.description ? <FieldHint>{p.description}</FieldHint> : null}
        </Field>
      ))}
      {generated.length ? (
        <ul className="space-y-1">
          {generated.map((p) => (
            <li key={p.key} className="flex items-start gap-2 text-sm">
              <KeyGlassIcon
                className="mt-0.5 size-5 shrink-0 text-muted-foreground"
                aria-hidden="true"
              />
              <span>
                <span className="font-medium text-foreground">
                  {p.label ?? p.key}
                </span>
                <span className="text-muted-foreground">
                  , {t('install.generatedHint')}
                </span>
              </span>
            </li>
          ))}
        </ul>
      ) : null}
      {(manifest.domains ?? []).map((d) => (
        <Field key={d.service}>
          <FieldLabel htmlFor={`template-domain-${d.service}`}>
            {t('install.domainLabel', { service: d.service, port: d.port })}
          </FieldLabel>
          <Input
            id={`template-domain-${d.service}`}
            placeholder={t('install.domainPlaceholder')}
            value={domains[d.service] ?? ''}
            onChange={(e) => {
              setDomains({
                ...domains,
                [d.service]: e.target.value.trim().toLowerCase(),
              })
            }}
          />
          <FieldHint>{d.description ?? t('install.domainHint')}</FieldHint>
        </Field>
      ))}
      {error || install.isError ? (
        <Alert variant="destructive">
          <TriangleAlertGlassIcon className="text-tone-warning" size="1.7em" />
          <AlertDescription>{error ?? install.error?.message}</AlertDescription>
        </Alert>
      ) : null}
      <div className="flex justify-end">
        <Button type="submit" disabled={install.isPending}>
          <RocketGlassIcon size="1.4em" />
          {install.isPending ? t('install.working') : t('install.submit')}
        </Button>
      </div>
    </form>
  )
}
