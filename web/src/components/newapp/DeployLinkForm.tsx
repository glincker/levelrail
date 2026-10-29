import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { useNavigate } from '@tanstack/react-router'
import { LinkGlassIcon } from '@thesvg/ui-react'
import { Alert, AlertDescription } from '@/components/ui/alert'
import { Banner } from '@/components/ui/banner'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { toast } from '@/components/ui/toast'
import { useCreateApp } from '../../queries/apps'
import { healthCheckFrom } from '../../lib/healthCheckDefaults'
import {
  isValidAppName,
  isValidHealthPath,
  isValidImageRef,
  isValidPort,
  type DeployLinkPrefill,
} from '../../lib/deployLink'

/** Notice shown above any form filled from a link, naming the source host. */
export function DeployLinkNotice({ prefill }: { prefill: DeployLinkPrefill }) {
  const { t } = useTranslation('newApp')
  return (
    <div className="space-y-3">
      <Banner
        variant="info"
        icon={<LinkGlassIcon size="1.2em" />}
        title={t('deployLink.title')}
      >
        {prefill.sourceHost
          ? t('deployLink.fromSource', { host: prefill.sourceHost })
          : t('deployLink.fromUnknown')}
      </Banner>
      {prefill.ignored.length > 0 ? (
        <Banner variant="warning" title={t('deployLink.ignoredTitle')}>
          {t('deployLink.ignoredBody', { params: prefill.ignored.join(', ') })}
        </Banner>
      ) : null}
    </div>
  )
}

/**
 * Create-app form prefilled from a deploy link. It never submits on its own,
 * and env values are always typed here, never read from the link.
 */
export function DeployLinkForm({ prefill }: { prefill: DeployLinkPrefill }) {
  const { t } = useTranslation('newApp')
  const navigate = useNavigate()
  const createApp = useCreateApp()
  const [name, setName] = useState(prefill.name ?? '')
  const [image, setImage] = useState(prefill.image ?? '')
  const [port, setPort] = useState(
    prefill.port !== undefined ? String(prefill.port) : '',
  )
  const [health, setHealth] = useState(prefill.health ?? '')
  const [secrets, setSecrets] = useState<Record<string, string>>({})
  const [touched, setTouched] = useState(false)

  const errors = {
    name: isValidAppName(name.trim()) ? null : t('deployLink.errors.name'),
    image: isValidImageRef(image.trim()) ? null : t('deployLink.errors.image'),
    port: isValidPort(port.trim()) ? null : t('deployLink.errors.port'),
    health:
      health.trim() === '' || isValidHealthPath(health.trim())
        ? null
        : t('deployLink.errors.health'),
  }
  const invalid = Object.values(errors).some((e) => e !== null)

  function submit(e: React.FormEvent) {
    e.preventDefault()
    setTouched(true)
    if (invalid) return
    const filled = prefill.env.filter((k) => (secrets[k] ?? '') !== '')
    createApp.mutate(
      {
        name: name.trim(),
        image: image.trim(),
        port: Number(port.trim()),
        health: healthCheckFrom(health.trim() !== '', health),
        ...(filled.length > 0
          ? {
              secret_env: filled,
              secrets: Object.fromEntries(
                filled.map((k) => [k, secrets[k] ?? '']),
              ),
            }
          : {}),
      },
      {
        onSuccess: (created) => {
          toast.add({
            title: t('toast.created', { name: created.name }),
            type: 'success',
          })
          void navigate({ to: '/apps/$name', params: { name: created.name } })
        },
      },
    )
  }

  return (
    <form onSubmit={submit} className="mx-auto w-full max-w-3xl space-y-5">
      <DeployLinkNotice prefill={prefill} />
      <div className="space-y-4">
        <Field>
          <FieldLabel htmlFor="dl-name">{t('deployLink.name')}</FieldLabel>
          <Input
            id="dl-name"
            value={name}
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
          {touched ? <FieldError>{errors.name}</FieldError> : null}
        </Field>
        <Field>
          <FieldLabel htmlFor="dl-image">{t('deployLink.image')}</FieldLabel>
          <Input
            id="dl-image"
            className="font-mono"
            value={image}
            onChange={(e) => {
              setImage(e.target.value)
            }}
          />
          {touched ? <FieldError>{errors.image}</FieldError> : null}
        </Field>
        <Field>
          <FieldLabel htmlFor="dl-port">{t('deployLink.port')}</FieldLabel>
          <Input
            id="dl-port"
            type="number"
            min="1"
            max="65535"
            value={port}
            onChange={(e) => {
              setPort(e.target.value)
            }}
          />
          <FieldHint>{t('deployLink.portHint')}</FieldHint>
          {touched ? <FieldError>{errors.port}</FieldError> : null}
        </Field>
        <Field>
          <FieldLabel htmlFor="dl-health">{t('deployLink.health')}</FieldLabel>
          <Input
            id="dl-health"
            className="font-mono"
            placeholder="/healthz"
            value={health}
            onChange={(e) => {
              setHealth(e.target.value)
            }}
          />
          <FieldHint>{t('deployLink.healthHint')}</FieldHint>
          {touched ? <FieldError>{errors.health}</FieldError> : null}
        </Field>
      </div>

      {prefill.env.length > 0 ? (
        <div className="space-y-3">
          <h2 className="text-(length:--glinr-type-size-section) leading-(--glinr-type-leading-section) font-(weight:--glinr-type-heading-weight)">
            {t('deployLink.envTitle')}
          </h2>
          <p className="text-sm text-muted-foreground">
            {t('deployLink.envHint')}
          </p>
          {prefill.env.map((key) => (
            <Field key={key}>
              <FieldLabel htmlFor={`dl-env-${key}`} className="font-mono">
                {key}
              </FieldLabel>
              <Input
                id={`dl-env-${key}`}
                type="password"
                autoComplete="off"
                aria-label={t('deployLink.envValue', { key })}
                value={secrets[key] ?? ''}
                onChange={(e) => {
                  setSecrets((prev) => ({ ...prev, [key]: e.target.value }))
                }}
              />
            </Field>
          ))}
        </div>
      ) : null}

      {createApp.isError ? (
        <Alert variant="destructive">
          <AlertDescription>{createApp.error.message}</AlertDescription>
        </Alert>
      ) : null}

      <Button type="submit" disabled={createApp.isPending}>
        {createApp.isPending
          ? t('deployLink.deploying')
          : t('deployLink.deploy')}
      </Button>
    </form>
  )
}
