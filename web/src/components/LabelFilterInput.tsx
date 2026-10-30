import { useTranslation } from 'react-i18next'
import { Input } from '@/components/ui/input'

export function LabelFilterInput({
  value,
  onChange,
  invalid,
}: {
  value: string
  onChange: (next: string) => void
  invalid?: boolean
}) {
  const { t } = useTranslation('common')
  return (
    <div className="space-y-1">
      <Input
        value={value}
        onChange={(e) => onChange(e.target.value)}
        placeholder={t('resourceLabels.filterPlaceholder')}
        aria-label={t('resourceLabels.filterLabel')}
        aria-invalid={invalid ? true : undefined}
        className="max-w-xs font-mono text-xs"
      />
      {invalid ? (
        <p className="text-xs text-destructive">
          {t('resourceLabels.filterInvalid')}
        </p>
      ) : null}
    </div>
  )
}
