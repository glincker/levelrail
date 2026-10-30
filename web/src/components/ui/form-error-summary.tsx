import { useTranslation } from 'react-i18next'
import type { FieldErrors, FieldValues, UseFormReturn } from 'react-hook-form'

import { cn } from '@/lib/utils'
import { focusAndReveal } from '@/lib/focusInvalid'

export interface SummaryField {
  label: string
  // Control id; falls back to the control whose name attribute is the key.
  id?: string
}

function flatten(errors: FieldErrors, prefix = ''): string[] {
  return Object.entries(errors).flatMap(([key, value]) => {
    if (!value || typeof value !== 'object') return []
    const path = prefix ? `${prefix}.${key}` : key
    if ('message' in value || 'type' in value) return [path]
    return flatten(value as FieldErrors, path)
  })
}

function findControl(name: string, field?: SummaryField): HTMLElement | null {
  if (field?.id) return document.getElementById(field.id)
  return document.querySelector<HTMLElement>(`[name="${CSS.escape(name)}"]`)
}

// Polite live region listing the invalid fields of a form after a failed
// submit; each entry is a link that moves focus to its control.
export function FormErrorSummary<T extends FieldValues>({
  form,
  fields = {},
  className,
}: {
  form: UseFormReturn<T, unknown, FieldValues>
  fields?: Record<string, SummaryField>
  className?: string
}) {
  const { t } = useTranslation('common')
  const { errors, submitCount } = form.formState
  const names = submitCount > 0 ? flatten(errors) : []
  return (
    <div
      data-slot="form-error-summary"
      role="status"
      aria-live="polite"
      className={className}
    >
      {names.length > 0 ? (
        <div
          className={cn(
            'rounded-lg border border-destructive/40 bg-destructive/5 p-3 text-sm',
          )}
        >
          <p className="font-medium text-destructive">
            {t('formErrorSummary.title', { count: names.length })}
          </p>
          <ul className="mt-1 list-disc pl-5">
            {names.map((name) => {
              const field = fields[name]
              return (
                <li key={name}>
                  <a
                    href={`#${field?.id ?? name}`}
                    className="underline underline-offset-2"
                    onClick={(e) => {
                      e.preventDefault()
                      const el = findControl(name, field)
                      if (el) focusAndReveal(el)
                    }}
                  >
                    {field?.label ?? name}
                  </a>
                </li>
              )
            })}
          </ul>
        </div>
      ) : null}
    </div>
  )
}
