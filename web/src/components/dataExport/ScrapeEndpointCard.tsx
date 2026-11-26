import { useTranslation } from 'react-i18next'
import { CopyField } from '@/components/traffic/CopyField'

/** ScrapeEndpointCard shows the Prometheus scrape path as a small copyable line. */
export function ScrapeEndpointCard() {
  const { t } = useTranslation('dataExport')
  return (
    <section className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border px-3 py-2 text-sm">
      <span className="font-medium">{t('scrape.title')}</span>
      <CopyField value="/metrics" label={t('scrape.copy')} />
      <span className="text-xs text-muted-foreground">
        {t('scrape.description')}
      </span>
    </section>
  )
}
