import { useState } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckIcon, CopyIcon } from '@phosphor-icons/react/dist/ssr'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'

// A labelled, copyable command or token.
export function CommandBlock({
  id,
  label,
  hint,
  value,
}: {
  id: string
  label: string
  hint?: string
  value: string
}) {
  const { t } = useTranslation('nodes')
  const [copied, setCopied] = useState(false)

  function copy() {
    void navigator.clipboard.writeText(value).then(() => {
      setCopied(true)
    })
  }

  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <div className="flex items-start gap-2 rounded-lg border border-input bg-muted/50 p-2">
        <code
          id={id}
          className="min-w-0 flex-1 overflow-x-auto text-xs break-all whitespace-pre-wrap"
        >
          {value}
        </code>
        <Button type="button" size="sm" variant="outline" onClick={copy}>
          {copied ? <CheckIcon /> : <CopyIcon />}
          {copied ? t('join.copied') : t('join.copy')}
        </Button>
      </div>
      {hint ? <FieldDescription>{hint}</FieldDescription> : null}
    </Field>
  )
}
