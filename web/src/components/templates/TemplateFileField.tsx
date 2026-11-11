import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { FileTextGlassIcon, LockGlassIcon } from '@thesvg/ui-react'
import type { TemplateEnvPrompt } from '../../types/compose'
import { Badge } from '@/components/ui/badge'
import { Textarea } from '@/components/ui/textarea'
import { Field, FieldError, FieldHint, FieldLabel } from '@/components/ui/field'

/** A config or secret file prompt: the default body is prefilled, only an edit is sent. */
export function TemplateFileField({
  prompt,
  value,
  error,
  sizeError,
  onChange,
}: {
  prompt: TemplateEnvPrompt
  value: string
  error?: string
  sizeError?: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation('templates')
  const [focused, setFocused] = useState(false)
  const id = `template-env-${prompt.key}`
  const label = prompt.label ?? prompt.key
  const masked = prompt.secret === true && !focused
  const invalid = !!error || !!sizeError
  const message =
    sizeError ?? (error ? t('form.errors.required', { field: label }) : '')

  return (
    <Field>
      <FieldLabel htmlFor={id}>
        {prompt.secret ? (
          <LockGlassIcon className="size-5" aria-hidden="true" />
        ) : (
          <FileTextGlassIcon className="size-5" aria-hidden="true" />
        )}
        {label}
        {prompt.secret ? (
          <Badge variant="warning">{t('form.file.secret')}</Badge>
        ) : null}
      </FieldLabel>
      <Textarea
        id={id}
        spellCheck={false}
        autoComplete="off"
        rows={6}
        className={
          masked
            ? 'min-h-24 font-mono text-xs [-webkit-text-security:disc]'
            : 'min-h-24 font-mono text-xs'
        }
        aria-required={prompt.required && !prompt.default}
        aria-invalid={invalid}
        aria-describedby={`${id}-hint`}
        value={value === '' ? (prompt.default ?? '') : value}
        onFocus={() => {
          setFocused(true)
        }}
        onBlur={() => {
          setFocused(false)
        }}
        onChange={(e) => {
          onChange(e.target.value)
        }}
      />
      <p
        id={`${id}-hint`}
        className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground"
      >
        <span>
          {t('form.file.mount', {
            service: prompt.service ?? '',
            path: prompt.path ?? '',
          })}
        </span>
        {prompt.mode ? (
          <span>{t('form.file.mode', { mode: prompt.mode })}</span>
        ) : null}
      </p>
      {invalid ? (
        <FieldError>{message}</FieldError>
      ) : prompt.description ? (
        <FieldHint>{prompt.description}</FieldHint>
      ) : null}
    </Field>
  )
}
