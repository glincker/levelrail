import { createFileRoute } from '@tanstack/react-router'
import { useTranslation } from 'react-i18next'
import { Share2GlassIcon } from '@thesvg/ui-react'
import { PageTemplate } from '@/components/shell/PageTemplate'
import { TrafficExportCard } from '../../components/trafficExport/TrafficExportCard'

export const Route = createFileRoute('/settings/traffic-export')({
  component: TrafficExportPage,
})

function TrafficExportPage() {
  const { t } = useTranslation('trafficExport')
  return (
    <PageTemplate
      icon={<Share2GlassIcon size="1.4em" />}
      title={t('title')}
      description={t('description')}
    >
      <TrafficExportCard />
    </PageTemplate>
  )
}
