import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import {
  EyeGlassIcon,
  EyeOffGlassIcon,
  KeyGlassIcon,
  RefreshCwGlassIcon,
} from '@thesvg/ui-react'
import { randomSecret } from '../../lib/templateForm'
import type { TemplateEnvPrompt } from '../../types/compose'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldHint, FieldLabel } from '@/components/ui/field'

/** One generated secret: empty means the server generates it, Reveal or Regenerate pins a visible value. */
export function TemplateGeneratedField({
  prompt,
  value,
  onChange,
}: {
  prompt: TemplateEnvPrompt
  value: string
  onChange: (value: string) => void
}) {
  const { t } = useTranslation('templates')
  const [shown, setShown] = useState(false)
  const id = `template-env-${prompt.key}`
  const pinned = value !== ''

  const reveal = () => {
    if (!pinned) onChange(randomSecret(prompt.length ?? 24))
    setShown(true)
  }

  return (
    <Field>
      <FieldLabel htmlFor={id}>
        <KeyGlassIcon className="size-4" aria-hidden="true" />
        {prompt.label ?? prompt.key}
      </FieldLabel>
      <div className="flex flex-wrap items-center gap-2">
        <Input
          id={id}
          readOnly
          type={shown ? 'text' : 'password'}
          className="min-w-0 flex-1 basis-48 font-mono"
          value={pinned ? value : ''}
          placeholder={t('form.generated.placeholder')}
        />
        <Button
          type="button"
          size="sm"
          variant="outline"
          aria-pressed={shown}
          onClick={() => {
            if (shown) setShown(false)
            else reveal()
          }}
        >
          {shown ? (
            <EyeOffGlassIcon size="1.2em" aria-hidden="true" />
          ) : (
            <EyeGlassIcon size="1.2em" aria-hidden="true" />
          )}
          {shown ? t('form.generated.hide') : t('form.generated.reveal')}
        </Button>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            onChange(randomSecret(prompt.length ?? 24))
          }}
        >
          <RefreshCwGlassIcon size="1.2em" aria-hidden="true" />
          {t('form.generated.regenerate')}
        </Button>
      </div>
      <FieldHint>{prompt.description ?? t('form.generated.hint')}</FieldHint>
    </Field>
  )
}
