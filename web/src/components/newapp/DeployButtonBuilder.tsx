import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'
import { Input } from '@/components/ui/input'
import { Textarea } from '@/components/ui/textarea'
import {
  buildDeployButtonUrl,
  deployButtonMarkdown,
} from '../../lib/deployLink'

/** Builds a "Deploy to Levelrail" README snippet from form fields. */
export default function DeployButtonBuilder() {
  const { t } = useTranslation('newApp')
  const [image, setImage] = useState('')
  const [name, setName] = useState('')
  const [port, setPort] = useState('')
  const [env, setEnv] = useState('')
  const [health, setHealth] = useState('')
  const [copied, setCopied] = useState(false)

  const touched =
    image !== '' || name !== '' || port !== '' || env !== '' || health !== ''
  const built = buildDeployButtonUrl({ image, name, port, env, health })
  const markdown = built.url === null ? '' : deployButtonMarkdown(built.url)
  const bad = (k: string) => touched && built.invalid.includes(k)

  const copy = () => {
    void navigator.clipboard.writeText(markdown).then(() => {
      setCopied(true)
      window.setTimeout(() => {
        setCopied(false)
      }, 2000)
    })
  }

  return (
    <section className="mx-auto w-full max-w-3xl space-y-4 rounded-lg border p-4">
      <div>
        <h2 className="text-base font-medium">{t('deployButton.title')}</h2>
        <p className="text-sm text-muted-foreground">
          {t('deployButton.description')}
        </p>
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <Field>
          <FieldLabel htmlFor="db-image">{t('deployButton.image')}</FieldLabel>
          <Input
            id="db-image"
            className="font-mono"
            placeholder="ghcr.io/you/app:latest"
            value={image}
            onChange={(e) => {
              setImage(e.target.value)
            }}
          />
          {bad('image') ? (
            <FieldError>{t('deployLink.errors.image')}</FieldError>
          ) : null}
        </Field>
        <Field>
          <FieldLabel htmlFor="db-name">{t('deployButton.name')}</FieldLabel>
          <Input
            id="db-name"
            placeholder={t('deployButton.namePlaceholder')}
            value={name}
            onChange={(e) => {
              setName(e.target.value)
            }}
          />
          {bad('name') ? (
            <FieldError>{t('deployLink.errors.name')}</FieldError>
          ) : null}
        </Field>
        <Field>
          <FieldLabel htmlFor="db-port">{t('deployButton.port')}</FieldLabel>
          <Input
            id="db-port"
            inputMode="numeric"
            placeholder="3000"
            value={port}
            onChange={(e) => {
              setPort(e.target.value)
            }}
          />
          {bad('port') ? (
            <FieldError>{t('deployLink.errors.port')}</FieldError>
          ) : null}
        </Field>
        <Field>
          <FieldLabel htmlFor="db-health">
            {t('deployButton.health')}
          </FieldLabel>
          <Input
            id="db-health"
            className="font-mono"
            placeholder="/healthz"
            value={health}
            onChange={(e) => {
              setHealth(e.target.value)
            }}
          />
          {bad('health') ? (
            <FieldError>{t('deployLink.errors.health')}</FieldError>
          ) : null}
        </Field>
      </div>
      <Field>
        <FieldLabel htmlFor="db-env">{t('deployButton.env')}</FieldLabel>
        <Textarea
          id="db-env"
          className="font-mono"
          rows={2}
          placeholder="DATABASE_URL, SESSION_KEY"
          value={env}
          onChange={(e) => {
            setEnv(e.target.value)
          }}
        />
        <FieldHint>{t('deployButton.envHint')}</FieldHint>
        {bad('env') ? (
          <FieldError>{t('deployButton.envError')}</FieldError>
        ) : null}
      </Field>
      {markdown !== '' ? (
        <div className="space-y-2">
          <pre
            className="overflow-x-auto rounded-md bg-muted p-3 font-mono text-xs break-all whitespace-pre-wrap"
            aria-label={t('deployButton.snippet')}
          >
            {markdown}
          </pre>
          <Button type="button" variant="outline" size="sm" onClick={copy}>
            {copied ? (
              <CheckGlassIcon size="1.2em" aria-hidden />
            ) : (
              <CopyGlassIcon size="1.2em" aria-hidden />
            )}
            {copied ? t('deployButton.copied') : t('deployButton.copy')}
          </Button>
        </div>
      ) : null}
    </section>
  )
}
