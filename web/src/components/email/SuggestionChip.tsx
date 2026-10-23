import { useTranslation } from 'react-i18next'
import { SparkleIcon } from '@phosphor-icons/react/dist/ssr'
import { PRESS } from './emailStyles'

interface Props {
  value: string | null
  current: string
  onUse: (value: string) => void
}

/** SuggestionChip offers a derived value; using it only fills the field, nothing is saved until the form is. */
export function SuggestionChip({ value, current, onUse }: Readonly<Props>) {
  const { t } = useTranslation('settings')
  if (!value || value === current.trim()) return null
  return (
    <button
      type="button"
      onClick={() => {
        onUse(value)
      }}
      aria-label={t('emailSender.useSuggestionLabel', { value })}
      className={`inline-flex max-w-full items-center gap-1 self-start rounded-full border border-dashed border-border px-2 py-0.5 text-xs text-muted-foreground transition-colors hover:border-primary hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none ${PRESS}`}
    >
      <SparkleIcon className="size-3 shrink-0" aria-hidden="true" />
      <span className="truncate">
        {t('emailSender.useSuggestion', { value })}
      </span>
    </button>
  )
}
