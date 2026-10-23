import { useTranslation } from 'react-i18next'
import type { FlowValues, UrlPreview } from './logic'
import { useSummaryText } from './useSummaryText'

export function DeploySummary({
  values,
  preview,
}: Readonly<{ values: FlowValues; preview: UrlPreview }>) {
  const { t } = useTranslation('functions')
  const text = useSummaryText(values, preview)
  return (
    <section
      aria-labelledby="fn-summary-title"
      className="space-y-1 rounded-lg border border-input bg-muted/40 p-3"
    >
      <h4 id="fn-summary-title" className="text-sm font-medium">
        {t('deploy.summary.title')}
      </h4>
      <p className="text-sm text-muted-foreground">{text}</p>
    </section>
  )
}
