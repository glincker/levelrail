import { useState, type ReactNode } from 'react'
import { useTranslation } from 'react-i18next'
import { CheckGlassIcon, CopyGlassIcon } from '@thesvg/ui-react'
import { Button } from '@/components/ui/button'
import { Field, FieldDescription, FieldLabel } from '@/components/ui/field'

// Read-only value with a copy button, shared by the webhook URL and the
// one-time webhook secret so the copied-state logic lives in one place.
export function GitSourceCopyField({
  id,
  label,
  value,
  description,
}: {
  id: string
  label: string
  value: string
  description?: ReactNode
}) {
  const { t } = useTranslation('gitSource')
  const [copied, setCopied] = useState(false)
  return (
    <Field>
      <FieldLabel htmlFor={id}>{label}</FieldLabel>
      <div className="flex items-center gap-2 rounded-lg border border-input bg-muted/50 p-2">
        <code
          id={id}
          className="min-w-0 flex-1 overflow-x-auto text-xs break-all"
        >
          {value}
        </code>
        <Button
          type="button"
          size="sm"
          variant="outline"
          onClick={() => {
            void navigator.clipboard.writeText(value).then(() => {
              setCopied(true)
            })
          }}
        >
          {copied ? (
            <CheckGlassIcon size="1.4em" />
          ) : (
            <CopyGlassIcon size="1.4em" />
          )}
          {copied ? t('copy.copied') : t('copy.copy')}
        </Button>
      </div>
      {description ? <FieldDescription>{description}</FieldDescription> : null}
    </Field>
  )
}
